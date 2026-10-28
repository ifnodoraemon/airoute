package router

import (
	"testing"
	"time"
)

// B-07: auxiliary upstream calls (count_tokens, out-of-band probes) bypass the
// Dispatch family, so they need an exported bridge onto the same CircuitBreaker.
// The wrappers must not change the semantics of the underlying state machine.
func TestBreakerAccessorsMatchCircuitBreakerSemantics(t *testing.T) {
	d := NewDispatcher(nil)
	d.circuitBreaker = NewCircuitBreaker(2, 30*time.Second)

	if !d.ChannelCanExecute("ch") {
		t.Fatal("a closed breaker must admit an auxiliary call")
	}

	// Two failures trip the configured threshold, exactly like dispatch traffic.
	d.NoteChannelFailure("ch")
	d.NoteChannelFailure("ch")
	if d.ChannelCanExecute("ch") {
		t.Fatal("accessor failures did not trip the breaker")
	}
	if got := d.GetBreakerStatus("ch"); got != StateOpen.String() {
		t.Fatalf("breaker status = %s, want OPEN", got)
	}

	// Success on a healthy channel resets the failure streak.
	d.NoteChannelFailure("healthy")
	d.NoteChannelSuccess("healthy")
	if failures := d.circuitBreaker.Snapshot("healthy").Failures; failures != 0 {
		t.Fatalf("success did not clear the failure streak: %d", failures)
	}

	// A nil dispatcher/breaker must be a no-op rather than a panic: auxiliary
	// callers may exist before the router is wired.
	var nilDispatcher *Dispatcher
	if !nilDispatcher.ChannelCanExecute("x") {
		t.Fatal("nil dispatcher must not reject auxiliary traffic")
	}
	nilDispatcher.NoteChannelSuccess("x")
	nilDispatcher.NoteChannelFailure("x")
	nilDispatcher.ReleaseHalfOpenProbe("x")
}

// ReleaseHalfOpenProbe returns the canary slot consumed by ChannelCanExecute when
// the attempt ended for a reason that is not a channel-health signal (client went
// away, payload could not be built). Without it a non-charging canary locks the
// channel out of rotation until the stale-slot reset.
func TestReleaseHalfOpenProbeGivesCanaryBack(t *testing.T) {
	d := NewDispatcher(nil)
	d.circuitBreaker = NewCircuitBreaker(1, 10*time.Millisecond)

	d.NoteChannelFailure("ch") // threshold 1 -> OPEN
	time.Sleep(15 * time.Millisecond)

	if !d.ChannelCanExecute("ch") {
		t.Fatal("cooldown elapsed but the breaker did not admit a canary probe")
	}
	if got := d.circuitBreaker.Snapshot("ch").CanaryInFlight; got != 1 {
		t.Fatalf("canary in flight = %d, want 1 after admission", got)
	}

	d.ReleaseHalfOpenProbe("ch")
	snapshot := d.circuitBreaker.Snapshot("ch")
	if snapshot.CanaryInFlight != 0 {
		t.Fatalf("canary in flight = %d after release, want 0", snapshot.CanaryInFlight)
	}
	if snapshot.State != StateHalfOpen.String() {
		t.Fatalf("state = %s, want HALF-OPEN (releasing a probe must not close the circuit)", snapshot.State)
	}
}

// Advisory mode (GATEWAY_BREAKER_ENABLED=false) records failures for operators but
// must never reject auxiliary traffic.
func TestBreakerAccessorsStayAdvisory(t *testing.T) {
	d := NewDispatcher(nil)
	d.circuitBreaker = NewCircuitBreaker(2, 30*time.Second)
	d.circuitBreaker.SetDisabled(true)

	for i := 0; i < 4; i++ {
		d.NoteChannelFailure("ch")
	}
	if !d.ChannelCanExecute("ch") {
		t.Fatal("advisory breaker rejected auxiliary traffic")
	}
	if failures := d.circuitBreaker.Snapshot("ch").Failures; failures != 4 {
		t.Fatalf("advisory breaker recorded %d failures, want 4 so the outage stays visible", failures)
	}
	d.NoteChannelSuccess("ch")
	if failures := d.circuitBreaker.Snapshot("ch").Failures; failures != 0 {
		t.Fatalf("advisory success did not clear the recorded failures: %d", failures)
	}
}
