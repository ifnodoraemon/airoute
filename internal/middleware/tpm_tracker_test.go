package middleware

import (
	"testing"
	"time"
)

func TestTokenQuotaEnforcesPerMinuteBudget(t *testing.T) {
	tr := &TokenQuotaTracker{buckets: make(map[string]*tokenQuotaBucket)}
	key := "sk-test"

	if !tr.Allow(key, 100) {
		t.Fatal("initial request should be allowed")
	}
	tr.Consume(key, 100, 60)
	if tr.Remaining(key, 100) != 40 {
		t.Fatalf("remaining = %d, want 40", tr.Remaining(key, 100))
	}
	if !tr.Allow(key, 100) {
		t.Fatal("request within budget should be allowed")
	}

	tr.Consume(key, 100, 50)
	if tr.Remaining(key, 100) != 0 {
		t.Fatalf("remaining = %d, want 0", tr.Remaining(key, 100))
	}
	if tr.Allow(key, 100) {
		t.Fatal("request over budget must be rejected")
	}
}

func TestTokenQuotaUnlimitedWhenZero(t *testing.T) {
	tr := &TokenQuotaTracker{buckets: make(map[string]*tokenQuotaBucket)}
	if !tr.Allow("k", 0) {
		t.Fatal("TPM=0 means unlimited and must always allow")
	}
	tr.Consume("k", 0, 1_000_000)
	if !tr.Allow("k", 0) {
		t.Fatal("unlimited keys must stay allowed")
	}
}

func TestTokenQuotaWindowRollsOver(t *testing.T) {
	tr := &TokenQuotaTracker{buckets: make(map[string]*tokenQuotaBucket)}
	key := "sk-window"
	tr.Consume(key, 100, 100)
	if tr.Remaining(key, 100) != 0 {
		t.Fatalf("expected exhausted budget, got %d", tr.Remaining(key, 100))
	}
	// Force the window backwards, then a call should roll it forward and reset.
	tr.mu.Lock()
	tr.buckets[key].windowFrom = time.Now().Add(-2 * time.Minute)
	tr.mu.Unlock()
	if tr.Remaining(key, 100) != 100 {
		t.Fatalf("expected window reset, got %d", tr.Remaining(key, 100))
	}
}
