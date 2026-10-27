package controlplane

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"
)

// newTestLoginGuard returns a guard with local-only counters and a shared store
// injected by the caller.
func newTestLoginGuard(shared loginAttemptStore) *loginGuard {
	return &loginGuard{local: newLocalLoginStore(), shared: shared}
}

// localOnlyClient is a throttle identity with no attributable address, which is
// what an unconfigured (no trusted proxy) deployment resolves to.
func localOnlyClient(username, ip string) loginClient {
	return loginClient{Username: username, IP: ip}
}

func forwardedClient(username, ip string) loginClient {
	return loginClient{Username: username, IP: ip, IPIsForwarded: true}
}

func TestLoginGuardLocksAccountAfterRepeatedFailures(t *testing.T) {
	g := newTestLoginGuard(nil)
	alice := forwardedClient("alice", "1.2.3.4")

	for i := 0; i < maxLoginFailuresPerAccount; i++ {
		if ok, _ := g.check(context.Background(), alice); !ok {
			t.Fatalf("attempt %d should be allowed before the limit", i+1)
		}
		g.failure(context.Background(), alice)
	}

	ok, retry := g.check(context.Background(), alice)
	if ok {
		t.Fatal("expected the account to be locked after repeated failures")
	}
	if retry <= 0 {
		t.Fatalf("expected a positive retry duration, got %v", retry)
	}

	// A different account from the same IP is still allowed: the address bucket
	// has its own (larger) budget.
	if ok, _ := g.check(context.Background(), forwardedClient("bob", "1.2.3.4")); !ok {
		t.Fatal("a different account from the same address should still be allowed")
	}
	// A different IP with the locked account name stays locked.
	if ok, _ := g.check(context.Background(), forwardedClient("alice", "5.6.7.8")); ok {
		t.Fatal("expected the locked account to stay locked across IPs")
	}
}

func TestLoginGuardIPBucketLocksSprayingAddress(t *testing.T) {
	g := newTestLoginGuard(nil)
	for i := 0; i < maxLoginFailuresPerIP; i++ {
		g.failure(context.Background(), forwardedClient(fmt.Sprintf("victim-%d", i), "9.9.9.9"))
	}
	if ok, _ := g.check(context.Background(), forwardedClient("someone-else", "9.9.9.9")); ok {
		t.Fatal("expected the spraying address to be throttled")
	}
	// A different address is unaffected.
	if ok, _ := g.check(context.Background(), forwardedClient("someone-else", "9.9.9.10")); !ok {
		t.Fatal("expected a different address to stay allowed")
	}
}

// TestLoginGuardSkipsAddressBucketWhenPeerIsUnattributable pins the fail-safe
// for deployments that have not configured a trusted proxy: the resolved
// address is the proxy, not the client, so locking it would lock out everyone.
func TestLoginGuardSkipsAddressBucketWhenPeerIsUnattributable(t *testing.T) {
	g := newTestLoginGuard(nil)
	// The same un-forwarded peer address, many different accounts. Only the
	// per-account buckets may trip; the shared peer address must not.
	for i := 0; i < maxLoginFailuresPerIP*2; i++ {
		g.failure(context.Background(), localOnlyClient(fmt.Sprintf("user-%d", i), "172.18.0.3"))
	}
	if ok, _ := g.check(context.Background(), localOnlyClient("fresh-account", "172.18.0.3")); !ok {
		t.Fatal("an unattributable proxy address must not lock out other accounts")
	}
}

func TestLoginGuardSuccessClearsCounter(t *testing.T) {
	g := newTestLoginGuard(nil)
	client := forwardedClient("carol", "9.9.9.9")
	for i := 0; i < maxLoginFailuresPerAccount-1; i++ {
		g.failure(context.Background(), client)
	}
	g.success(context.Background(), client)
	for i := 0; i < maxLoginFailuresPerAccount; i++ {
		if ok, _ := g.check(context.Background(), client); !ok {
			t.Fatalf("attempt %d should be allowed after a successful login reset", i+1)
		}
		g.failure(context.Background(), client)
	}
	if ok, _ := g.check(context.Background(), client); ok {
		t.Fatal("expected a fresh lock after the reset budget is spent")
	}
}

func TestLoginGuardLockExpires(t *testing.T) {
	g := newTestLoginGuard(nil)
	client := forwardedClient("dave", "8.8.8.8")
	for i := 0; i < maxLoginFailuresPerAccount; i++ {
		g.failure(context.Background(), client)
	}
	if ok, _ := g.check(context.Background(), client); ok {
		t.Fatal("expected lock")
	}
	// Simulate the lockout elapsing.
	g.local.mu.Lock()
	g.local.entries[loginBucketKey(loginBucket{kind: "user", subject: "dave"})].lockedUntil = time.Now().Add(-time.Second)
	g.local.entries[loginBucketKey(loginBucket{kind: "ip", subject: "8.8.8.8"})].lockedUntil = time.Now().Add(-time.Second)
	g.local.mu.Unlock()
	if ok, _ := g.check(context.Background(), client); !ok {
		t.Fatal("expected the lock to expire")
	}
}

// sharedMemoryStore simulates the Redis-backed cluster counters: two guards
// sharing one store must share the brute-force budget, which the per-process
// maps never did.
type sharedMemoryStore struct {
	mu      sync.Mutex
	buckets map[string]*loginAttempt
}

func newSharedMemoryStore() *sharedMemoryStore {
	return &sharedMemoryStore{buckets: make(map[string]*loginAttempt)}
}

func (s *sharedMemoryStore) locked(_ context.Context, bucket loginBucket) (bool, time.Duration, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	attempt := s.buckets[loginBucketKey(bucket)]
	if attempt == nil {
		return false, 0, nil
	}
	if remaining := time.Until(attempt.lockedUntil); remaining > 0 {
		return true, remaining, nil
	}
	return false, 0, nil
}

func (s *sharedMemoryStore) fail(_ context.Context, bucket loginBucket) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now()
	attempt := s.buckets[loginBucketKey(bucket)]
	if attempt == nil {
		attempt = &loginAttempt{}
		s.buckets[loginBucketKey(bucket)] = attempt
	}
	attempt.failures++
	attempt.lastFailure = now
	if attempt.failures >= bucket.limit {
		attempt.lockedUntil = now.Add(loginLockoutWindow)
		attempt.failures = 0
	}
	return nil
}

func (s *sharedMemoryStore) reset(_ context.Context, bucket loginBucket) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.buckets, loginBucketKey(bucket))
	return nil
}

// TestLoginGuardSharedStoreBlocksAcrossReplicas is the cross-replica regression:
// five failures against replica A must lock the account on replica B.
func TestLoginGuardSharedStoreBlocksAcrossReplicas(t *testing.T) {
	store := newSharedMemoryStore()
	replicaA := newTestLoginGuard(store)
	replicaB := newTestLoginGuard(store)

	client := forwardedClient("erin", "203.0.113.7")
	for i := 0; i < maxLoginFailuresPerAccount; i++ {
		if ok, _ := replicaA.check(context.Background(), client); !ok {
			t.Fatalf("replica A attempt %d should be allowed", i+1)
		}
		replicaA.failure(context.Background(), client)
	}

	if ok, retry := replicaB.check(context.Background(), client); ok {
		t.Fatal("replica B must see the lock created by replica A")
	} else if retry <= 0 {
		t.Fatalf("expected a positive retry, got %v", retry)
	}

	// A successful login on either replica clears the shared budget.
	replicaB.success(context.Background(), client)
	if ok, _ := replicaA.check(context.Background(), client); !ok {
		t.Fatal("expected the success on replica B to clear the lock seen by replica A")
	}
}

// failingStore simulates an unavailable shared store.
type failingStore struct{}

func (failingStore) locked(context.Context, loginBucket) (bool, time.Duration, error) {
	return false, 0, context.DeadlineExceeded
}
func (failingStore) fail(context.Context, loginBucket) error  { return context.DeadlineExceeded }
func (failingStore) reset(context.Context, loginBucket) error { return context.DeadlineExceeded }

// TestLoginGuardFallsBackToLocalWhenSharedStoreFails ensures a Redis outage
// degrades to process-local counters instead of disabling the lockout.
func TestLoginGuardFallsBackToLocalWhenSharedStoreFails(t *testing.T) {
	g := newTestLoginGuard(failingStore{})
	client := forwardedClient("frank", "198.51.100.5")
	for i := 0; i < maxLoginFailuresPerAccount; i++ {
		if ok, _ := g.check(context.Background(), client); !ok {
			t.Fatalf("attempt %d should be allowed", i+1)
		}
		g.failure(context.Background(), client)
	}
	if ok, _ := g.check(context.Background(), client); ok {
		t.Fatal("expected the local fallback to enforce the lock after the shared store failed")
	}
}
