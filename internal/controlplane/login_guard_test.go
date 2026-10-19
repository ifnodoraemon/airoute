package controlplane

import (
	"testing"
	"time"
)

func TestLoginGuardLocksAfterRepeatedFailures(t *testing.T) {
	g := &loginGuard{byUser: map[string]*loginAttempt{}, byIP: map[string]*loginAttempt{}}

	for i := 0; i < maxLoginFailures; i++ {
		if ok, _ := g.check("alice", "1.2.3.4"); !ok {
			t.Fatalf("attempt %d should be allowed before the limit", i+1)
		}
		g.failure("alice", "1.2.3.4")
	}

	ok, retry := g.check("alice", "1.2.3.4")
	if ok {
		t.Fatal("expected the account to be locked after repeated failures")
	}
	if retry <= 0 {
		t.Fatalf("expected a positive retry duration, got %v", retry)
	}

	// A different account from the same IP is also throttled (IP budget).
	if ok, _ := g.check("bob", "1.2.3.4"); ok {
		t.Fatal("expected the offending IP to be throttled as well")
	}
	// A different IP with the locked account name stays locked.
	if ok, _ := g.check("alice", "5.6.7.8"); ok {
		t.Fatal("expected the locked account to stay locked across IPs")
	}
}

func TestLoginGuardSuccessClearsCounter(t *testing.T) {
	g := &loginGuard{byUser: map[string]*loginAttempt{}, byIP: map[string]*loginAttempt{}}
	for i := 0; i < maxLoginFailures-1; i++ {
		g.failure("carol", "9.9.9.9")
	}
	g.success("carol", "9.9.9.9")
	for i := 0; i < maxLoginFailures; i++ {
		if ok, _ := g.check("carol", "9.9.9.9"); !ok {
			t.Fatalf("attempt %d should be allowed after a successful login reset", i+1)
		}
		g.failure("carol", "9.9.9.9")
	}
	if ok, _ := g.check("carol", "9.9.9.9"); ok {
		t.Fatal("expected a fresh lock after the reset budget is spent")
	}
}

func TestLoginGuardLockExpires(t *testing.T) {
	g := &loginGuard{byUser: map[string]*loginAttempt{}, byIP: map[string]*loginAttempt{}}
	for i := 0; i < maxLoginFailures; i++ {
		g.failure("dave", "8.8.8.8")
	}
	if ok, _ := g.check("dave", "8.8.8.8"); ok {
		t.Fatal("expected lock")
	}
	// Simulate the lockout elapsing.
	g.mu.Lock()
	g.byUser["dave"].lockedUntil = time.Now().Add(-time.Second)
	g.byIP["8.8.8.8"].lockedUntil = time.Now().Add(-time.Second)
	g.mu.Unlock()
	if ok, _ := g.check("dave", "8.8.8.8"); !ok {
		t.Fatal("expected the lock to expire")
	}
}
