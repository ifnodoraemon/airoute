package router

import (
	"testing"
	"time"
)

// A result admitted before a trip belongs to an earlier generation. While the
// breaker is half-open it must not be counted as a canary outcome: a stale
// success would close the breaker without a real probe, and a stale failure
// would reopen it and reset the cooldown.
func TestCircuitBreakerHalfOpenIgnoresStaleGeneration(t *testing.T) {
	cb := NewCircuitBreaker(2, 40*time.Millisecond)

	cb.RecordFailure("ch")
	cb.RecordFailure("ch")
	if cb.GetStatus("ch") != StateOpen {
		t.Fatalf("expected OPEN after threshold failures, got %v", cb.GetStatus("ch"))
	}
	time.Sleep(50 * time.Millisecond)

	ok, staleGen := cb.CanExecuteGen("ch")
	if !ok || cb.GetStatus("ch") != StateHalfOpen {
		t.Fatalf("expected half-open canary admission, got ok=%v state=%v", ok, cb.GetStatus("ch"))
	}
	// The canary fails: the breaker reopens and the generation advances.
	cb.RecordFailureGen("ch", staleGen)
	if cb.GetStatus("ch") != StateOpen {
		t.Fatalf("expected OPEN after canary failure, got %v", cb.GetStatus("ch"))
	}
	time.Sleep(50 * time.Millisecond)

	ok, currentGen := cb.CanExecuteGen("ch")
	if !ok || cb.GetStatus("ch") != StateHalfOpen {
		t.Fatalf("expected a fresh half-open era, got ok=%v state=%v", ok, cb.GetStatus("ch"))
	}
	if currentGen == staleGen {
		t.Fatal("generation must advance across state transitions")
	}

	// A long stream admitted in the old era finishes now: its success must be
	// ignored, leaving the current canary pending.
	cb.RecordSuccessGen("ch", staleGen)
	if cb.GetStatus("ch") != StateHalfOpen {
		t.Fatalf("stale success closed the breaker without a probe: %v", cb.GetStatus("ch"))
	}
	// A stale failure must not reopen the breaker either.
	cb.RecordFailureGen("ch", staleGen)
	if cb.GetStatus("ch") != StateHalfOpen {
		t.Fatalf("stale failure reopened the breaker: %v", cb.GetStatus("ch"))
	}

	// Current-era successes recover the breaker.
	for i := 0; i < 5; i++ {
		cb.RecordSuccessGen("ch", currentGen)
	}
	if cb.GetStatus("ch") != StateClosed {
		t.Fatalf("current-era successes did not recover the breaker: %v", cb.GetStatus("ch"))
	}
}