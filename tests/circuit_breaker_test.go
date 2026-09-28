package tests

import (
	"testing"
	"time"

	"github.com/ifnodoraemon/airoute/internal/router"
)

func TestCircuitBreaker_StateTransitions(t *testing.T) {
	cooldown := 50 * time.Millisecond
	threshold := 3
	cb := router.NewCircuitBreaker(threshold, cooldown)

	name := "test-upstream-gpu"

	// 1. Initial state should be healthy / closed
	if !cb.CanExecute(name) {
		t.Fatalf("expected CanExecute to be true initially")
	}
	if status := cb.GetStatus(name); status != router.StateClosed {
		t.Fatalf("expected StateClosed, got %v", status)
	}

	// 2. Record 2 failures (below threshold)
	cb.RecordFailure(name)
	cb.RecordFailure(name)
	if !cb.CanExecute(name) {
		t.Fatalf("expected CanExecute to remain true before threshold")
	}

	// 3. Record 3rd failure (trips breaker to OPEN)
	cb.RecordFailure(name)
	if status := cb.GetStatus(name); status != router.StateOpen {
		t.Fatalf("expected StateOpen after %d failures, got %v", threshold, status)
	}

	// 4. CanExecute must fail-fast while OPEN
	if cb.CanExecute(name) {
		t.Fatalf("expected CanExecute to be false while OPEN")
	}

	// 5. Wait for cooldown to enter HALF-OPEN
	time.Sleep(cooldown + 10*time.Millisecond)

	if !cb.CanExecute(name) {
		t.Fatalf("expected CanExecute to be true after cooldown in HALF-OPEN")
	}
	if status := cb.GetStatus(name); status != router.StateHalfOpen {
		t.Fatalf("expected StateHalfOpen, got %v", status)
	}

	// 6. Canary succeeds -> resets to CLOSED
	cb.RecordSuccess(name)
	if status := cb.GetStatus(name); status != router.StateClosed {
		t.Fatalf("expected StateClosed after recovery, got %v", status)
	}
	if !cb.CanExecute(name) {
		t.Fatalf("expected CanExecute to be true after recovery")
	}
}
