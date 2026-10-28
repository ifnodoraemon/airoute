package middleware

import (
	"testing"
	"time"
)

// B-05 regression suite: the TPM gate is a true 60s sliding window and refuses once
// the budget is spent.

func newTestBucket(limit int) *tokenQuotaBucket {
	return &tokenQuotaBucket{limit: limit}
}

// TestTPMWindowDoesNotDoubleBurstAtTheBoundary is the core regression: burning the
// whole budget at the end of a window must not be rewarded with a second budget
// right after the boundary (the fixed-window behaviour gave 2x TPM in ~1s).
func TestTPMWindowDoesNotDoubleBurstAtTheBoundary(t *testing.T) {
	b := newTestBucket(100)
	base := time.Now()

	// 100 tokens consumed at t=59s of what the old implementation would call the
	// first window.
	b.consumeAt(base.Add(59*time.Second), 100)

	// Just past the fixed-window boundary the old counter would have reset; a
	// sliding window still sees the tokens, which are only 1s old.
	if b.allowAt(base.Add(60*time.Second + 100*time.Millisecond)) {
		t.Fatal("sliding window admitted a request 1s after the budget was spent: the window is still a fixed one")
	}
	if got := b.remainingAt(base.Add(60*time.Second + 100*time.Millisecond)); got != 0 {
		t.Fatalf("remaining = %d just past the boundary, want 0", got)
	}

	// Only when the consumption itself is one full window old may the budget return.
	if !b.allowAt(base.Add(119*time.Second + 500*time.Millisecond)) {
		t.Fatal("consumption older than the window still blocks: the window does not slide")
	}
}

// TestTPMRejectsWhenBudgetExactlySpent is the off-by-one fix: with `used == limit`
// the next admission must be refused, not admitted with zero tokens charged.
func TestTPMRejectsWhenBudgetExactlySpent(t *testing.T) {
	b := newTestBucket(10)
	base := time.Now()
	b.consumeAt(base, 10)

	if b.allowAt(base.Add(time.Second)) {
		t.Fatal("an exhausted budget still admits requests (used == limit must refuse)")
	}

	// One token under the limit is still admissible.
	b2 := newTestBucket(10)
	b2.consumeAt(base, 9)
	if !b2.allowAt(base.Add(time.Second)) {
		t.Fatal("a request under the budget was refused")
	}
}

// TestTPMWindowEvictsIncrementally pins the sliding semantics: consumption is
// forgotten entry by entry, not all at once.
func TestTPMWindowEvictsIncrementally(t *testing.T) {
	b := newTestBucket(100)
	base := time.Now()

	b.consumeAt(base.Add(0*time.Second), 60)
	b.consumeAt(base.Add(30*time.Second), 40)

	// At t=61s the 60-token entry has slid out but the 40-token one has not.
	if got := b.remainingAt(base.Add(61 * time.Second)); got != 60 {
		t.Fatalf("remaining at t=61s = %d, want 60 (only the older entry slid out)", got)
	}
	// At t=91s everything is out.
	if got := b.remainingAt(base.Add(91 * time.Second)); got != 100 {
		t.Fatalf("remaining at t=91s = %d, want 100", got)
	}
}

// TestTPMWindowMemoryIsBounded keeps the per-key footprint proportional to the
// window's traffic: evicted entries must actually be dropped, not accumulated.
func TestTPMWindowMemoryIsBounded(t *testing.T) {
	b := newTestBucket(1_000_000)
	base := time.Now()
	for i := 0; i < 1000; i++ {
		b.consumeAt(base.Add(time.Duration(i)*time.Millisecond), 1)
	}
	b.mu.Lock()
	before := len(b.usage)
	b.mu.Unlock()
	if before != 1000 {
		t.Fatalf("usage entries = %d, want 1000 inside the window", before)
	}

	// Everything is now older than the window: the next operation must drop it all.
	b.remainingAt(base.Add(2 * tokenWindowSpan))
	b.mu.Lock()
	after := len(b.usage)
	b.mu.Unlock()
	if after != 0 {
		t.Fatalf("usage entries after the window passed = %d, want 0", after)
	}
}

// TestTokenQuotaHotLimitUpdateAppliesImmediately keeps the documented hot-reload
// behaviour and pins that spent tokens are not forgotten when the limit changes.
func TestTokenQuotaHotLimitUpdateAppliesImmediately(t *testing.T) {
	tr := &TokenQuotaTracker{buckets: make(map[string]*tokenQuotaBucket)}
	const key = "sk-hot"

	tr.Consume(key, 1000, 500)
	if !tr.Allow(key, 1000) {
		t.Fatal("request under the initial limit was refused")
	}

	// Lowering the limit below the spent amount refuses immediately.
	if tr.Allow(key, 100) {
		t.Fatal("a lowered TPM limit did not apply: 500 spent tokens still admitted against a 100 limit")
	}
	if got := tr.Remaining(key, 100); got != 0 {
		t.Fatalf("remaining under the lowered limit = %d, want 0", got)
	}

	// Raising it above the spent amount admits again.
	if !tr.Allow(key, 1000) {
		t.Fatal("a raised TPM limit did not apply")
	}
}

// TestTokenQuotaCleanupDropsIdleKeys keeps the cleanup the rate-limit ticker calls.
func TestTokenQuotaCleanupDropsIdleKeys(t *testing.T) {
	tr := &TokenQuotaTracker{buckets: make(map[string]*tokenQuotaBucket)}
	tr.Consume("k1", 100, 10)
	tr.Consume("k2", 100, 10)

	tr.buckets["k1"].mu.Lock()
	tr.buckets["k1"].lastSeen = time.Now().Add(-time.Hour)
	tr.buckets["k1"].mu.Unlock()

	tr.CleanupIdle(15 * time.Minute)

	tr.mu.Lock()
	defer tr.mu.Unlock()
	if _, ok := tr.buckets["k1"]; ok {
		t.Fatal("an idle bucket was not deleted")
	}
	if _, ok := tr.buckets["k2"]; !ok {
		t.Fatal("a bucket seen recently was deleted")
	}
}
