package distributed

import (
	"context"
	"sync"
	"testing"
	"time"
)

// B-01 regression suite (in-flight reservation lifetime).
//
// The reservation booked at admission expires after inflightQuotaTTL, which is
// deliberately short so a crashed node cannot block a wallet forever. A live
// request therefore refreshes its keys through KeepInflightQuotaAlive /
// keepAliveLoop; without the heartbeat a request that outlives the TTL loses its
// reservation, overdrafts the wallet and steals another request's ledger lines on
// release.
//
// The tests below exercise the real heartbeat loop and the real interval/plumbing
// helpers against a model of the two Redis keys (the CI environment has no Redis
// server). quota_heartbeat_integration_test.go asserts the same three properties
// against a real server.

// fakeQuotaKeys models the two reservation keys with Redis PEXPIRE semantics:
// refreshing an existing key extends its deadline, refreshing a missing key does
// nothing (it must never be re-created), and a key is gone once the deadline
// passes.
type fakeQuotaKeys struct {
	mu       sync.Mutex
	expireAt map[string]time.Time
}

func newFakeQuotaKeys(names ...string) *fakeQuotaKeys {
	f := &fakeQuotaKeys{expireAt: make(map[string]time.Time)}
	for _, name := range names {
		f.expireAt[name] = time.Time{}
	}
	return f
}

// set books a key with the given TTL (used for reserve/release in the model).
func (f *fakeQuotaKeys) set(name string, ttl time.Duration) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.expireAt[name] = time.Now().Add(ttl)
}

// pexpire is the model of the Lua keep-alive script: only existing keys move.
func (f *fakeQuotaKeys) pexpire(ttl time.Duration) {
	now := time.Now()
	f.mu.Lock()
	defer f.mu.Unlock()
	for name, deadline := range f.expireAt {
		if deadline.IsZero() || now.After(deadline) {
			// Already expired: leave it gone instead of re-creating it.
			delete(f.expireAt, name)
			continue
		}
		f.expireAt[name] = now.Add(ttl)
	}
}

func (f *fakeQuotaKeys) alive(name string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	deadline, ok := f.expireAt[name]
	if !ok || deadline.IsZero() {
		return false
	}
	if time.Now().After(deadline) {
		delete(f.expireAt, name)
		return false
	}
	return true
}

func (f *fakeQuotaKeys) drop() {
	f.mu.Lock()
	defer f.mu.Unlock()
	for name := range f.expireAt {
		delete(f.expireAt, name)
	}
}

// withInflightTTL shortens the reservation TTL for the duration of one test and
// restores it afterwards. The heartbeat interval is derived from the TTL, so the
// whole mechanism runs at millisecond scale.
func withInflightTTL(t *testing.T, ttl time.Duration) {
	t.Helper()
	previous := inflightQuotaTTL
	inflightQuotaTTL = ttl
	t.Cleanup(func() { inflightQuotaTTL = previous })
}

func TestInflightQuotaTTLPlumbingIsShared(t *testing.T) {
	withInflightTTL(t, 90*time.Millisecond)

	if got := inflightQuotaTTLMillis(); got < 1 {
		t.Fatalf("millisecond TTL = %d, want >= 1", got)
	}
	// The heartbeat must fire well before the keys can expire, otherwise a single
	// delayed tick loses the reservation.
	interval := inflightQuotaHeartbeatInterval()
	if interval <= 0 || interval >= inflightQuotaTTL {
		t.Fatalf("heartbeat interval %s is not inside (0, TTL=%s)", interval, inflightQuotaTTL)
	}
	if inflightQuotaTTL/interval < 2 {
		t.Fatalf("heartbeat interval %s gives fewer than two refreshes per TTL %s", interval, inflightQuotaTTL)
	}
}

// TestInflightQuotaHeartbeatKeepsReservationAlive is property ①: a live request's
// reservation must not disappear, no matter how much longer than the TTL it runs.
func TestInflightQuotaHeartbeatKeepsReservationAlive(t *testing.T) {
	withInflightTTL(t, 90*time.Millisecond)

	keys := newFakeQuotaKeys("amt", "cnt")
	keys.set("amt", inflightQuotaTTL)
	keys.set("cnt", inflightQuotaTTL)

	stopCh := make(chan struct{})
	done := make(chan struct{})
	go func() {
		defer close(done)
		keepAliveLoop(inflightQuotaHeartbeatInterval(), func(context.Context) error {
			keys.pexpire(inflightQuotaTTL)
			return nil
		}, stopCh)
	}()

	// Run for several TTLs with the heartbeat on.
	time.Sleep(5 * inflightQuotaTTL)
	if !keys.alive("amt") || !keys.alive("cnt") {
		close(stopCh)
		<-done
		t.Fatalf("reservation expired despite the heartbeat (amt alive=%v cnt alive=%v)",
			keys.alive("amt"), keys.alive("cnt"))
	}

	close(stopCh)
	<-done

	// Property ③ (stop half): once the heartbeat is stopped the keys age out again,
	// so a finished request cannot keep a reservation alive by accident.
	time.Sleep(2 * inflightQuotaTTL)
	if keys.alive("amt") || keys.alive("cnt") {
		t.Fatalf("keys survived after the heartbeat was stopped: the heartbeat is not bounded by stop()")
	}
}

// TestInflightQuotaWithoutHeartbeatExpires is property ②: the crash path. A
// reservation with no heartbeat (the process died before releasing) must expire on
// its TTL instead of blocking the wallet forever.
func TestInflightQuotaWithoutHeartbeatExpires(t *testing.T) {
	withInflightTTL(t, 60*time.Millisecond)

	keys := newFakeQuotaKeys("amt", "cnt")
	keys.set("amt", inflightQuotaTTL)
	keys.set("cnt", inflightQuotaTTL)

	if !keys.alive("amt") {
		t.Fatal("model broken: a freshly booked key is not alive")
	}
	time.Sleep(2 * inflightQuotaTTL)
	if keys.alive("amt") || keys.alive("cnt") {
		t.Fatalf("an unrefreshed reservation did not expire after %s; the crashed-node safety net is gone", inflightQuotaTTL)
	}
}

// TestInflightQuotaReleaseAfterHeartbeatStopClearsKeys is property ③: stop the
// heartbeat, release, keys are gone; and a late refresh must not revive them.
func TestInflightQuotaReleaseAfterHeartbeatStopClearsKeys(t *testing.T) {
	withInflightTTL(t, 90*time.Millisecond)

	keys := newFakeQuotaKeys("amt", "cnt")
	keys.set("amt", inflightQuotaTTL)
	keys.set("cnt", inflightQuotaTTL)

	stopCh := make(chan struct{})
	done := make(chan struct{})
	go func() {
		defer close(done)
		keepAliveLoop(inflightQuotaHeartbeatInterval(), func(context.Context) error {
			keys.pexpire(inflightQuotaTTL)
			return nil
		}, stopCh)
	}()

	time.Sleep(2 * inflightQuotaTTL)

	// The documented order: stop the heartbeat synchronously, then release.
	close(stopCh)
	<-done
	keys.drop() // the release script deletes both keys

	if keys.alive("amt") || keys.alive("cnt") {
		t.Fatal("release did not clear the keys")
	}
	time.Sleep(2 * inflightQuotaTTL)
	if keys.alive("amt") || keys.alive("cnt") {
		t.Fatal("a late heartbeat re-armed the released reservation; stop() must run before release")
	}
}

// TestKeepInflightQuotaAliveIsSafeWithoutClient pins the no-Redis deployment: the
// heartbeat is a no-op and its stop function is idempotent. Every caller has to
// hold a reservation for the local backend too, so a nil client must never panic.
func TestKeepInflightQuotaAliveIsSafeWithoutClient(t *testing.T) {
	previous := GetClient()
	defer InitRedis("") // restore local-memory mode
	if previous != nil && previous.IsActive() {
		t.Skip("a Redis server is configured for this process; the no-client path cannot be exercised")
	}

	stop := (*Client)(nil).KeepInflightQuotaAlive(42)
	stop()
	stop() // idempotent

	inactive := &Client{}
	stop = inactive.KeepInflightQuotaAlive(42)
	stop()
	stop()
}

// TestKeepAliveLoopStopWaitsForRunningTick is the ordering guarantee callers rely
// on: stop() must not return while a refresh is in flight, otherwise the release
// that follows could race a refresh that re-arms the keys.
func TestKeepAliveLoopStopWaitsForRunningTick(t *testing.T) {
	var inFlight, maxConcurrent int
	var mu sync.Mutex
	started := make(chan struct{}, 1)

	stopCh := make(chan struct{})
	done := make(chan struct{})
	go func() {
		defer close(done)
		keepAliveLoop(5*time.Millisecond, func(context.Context) error {
			mu.Lock()
			inFlight++
			if inFlight > maxConcurrent {
				maxConcurrent = inFlight
			}
			mu.Unlock()
			select {
			case started <- struct{}{}:
			default:
			}
			time.Sleep(15 * time.Millisecond)
			mu.Lock()
			inFlight--
			mu.Unlock()
			return nil
		}, stopCh)
	}()

	<-started
	close(stopCh)
	<-done

	mu.Lock()
	defer mu.Unlock()
	if inFlight != 0 {
		t.Fatalf("keepAliveLoop returned with %d refreshes still running", inFlight)
	}
	if maxConcurrent != 1 {
		t.Fatalf("refreshes overlapped (max concurrent %d); the loop must be sequential", maxConcurrent)
	}
}
