package router

import (
	"sync"
	"time"

	"github.com/ifnodoraemon/airoute/internal/telemetry"
)

// CircuitState represents the current state of a circuit breaker.
type CircuitState int

const (
	StateClosed   CircuitState = iota // Normal operation, all traffic allowed
	StateOpen                         // Tripped, failing fast without hitting dead upstream
	StateHalfOpen                     // Cooldown passed, testing recovery with canary request
)

func (s CircuitState) String() string {
	switch s {
	case StateClosed:
		return "CLOSED"
	case StateOpen:
		return "OPEN"
	case StateHalfOpen:
		return "HALF-OPEN"
	default:
		return "UNKNOWN"
	}
}

// circuitStateHandler defines the state behavior interface under the State Pattern.
type circuitStateHandler interface {
	CanExecute(b *BreakerStatus, cb *CircuitBreaker, now time.Time) bool
	RecordSuccess(b *BreakerStatus, cb *CircuitBreaker, now time.Time)
	RecordFailure(b *BreakerStatus, cb *CircuitBreaker, now time.Time)
	State() CircuitState
}

// BreakerStatus tracks state for a single upstream channel.
type BreakerStatus struct {
	State                CircuitState
	ConsecutiveFailures  int
	ConsecutiveSuccesses int
	CanaryInFlight       int
	LastStateChange      time.Time
	// Generation increments on every state transition so a result admitted in an
	// earlier era can be recognised as stale and excluded from canary accounting.
	Generation uint64
	name       string
	handler    circuitStateHandler
}

// CircuitBreaker manages failover and auto-recovery for all upstream providers.
type CircuitBreaker struct {
	mu               sync.RWMutex
	breakers         map[string]*BreakerStatus
	failureThreshold int
	successThreshold int
	maxCanaryProbes  int
	cooldownDuration time.Duration
	// disabled keeps the breaker advisory: state is still recorded for operators,
	// but it never rejects traffic. With a single upstream channel there is nothing
	// to fail over to, so opening the circuit only converts upstream errors into a
	// self-inflicted blackout.
	disabled bool
}

// SetDisabled turns the breaker into an advisory-only recorder.
func (cb *CircuitBreaker) SetDisabled(disabled bool) {
	cb.mu.Lock()
	cb.disabled = disabled
	cb.mu.Unlock()
}

// Disabled reports whether the breaker may reject traffic.
func (cb *CircuitBreaker) Disabled() bool {
	cb.mu.RLock()
	defer cb.mu.RUnlock()
	return cb.disabled
}

// NewCircuitBreaker creates a circuit breaker instance with tunable thresholds.
func NewCircuitBreaker(failureThreshold int, cooldown time.Duration) *CircuitBreaker {
	if failureThreshold <= 0 {
		failureThreshold = 3
	}
	if cooldown <= 0 {
		cooldown = 30 * time.Second
	}
	return &CircuitBreaker{
		breakers:         make(map[string]*BreakerStatus),
		failureThreshold: failureThreshold,
		successThreshold: 1,
		maxCanaryProbes:  1,
		cooldownDuration: cooldown,
	}
}

func (cb *CircuitBreaker) getOrCreateStatusLocked(name string) *BreakerStatus {
	status, exists := cb.breakers[name]
	if !exists {
		status = &BreakerStatus{
			State:           StateClosed,
			LastStateChange: time.Now(),
			name:            name,
			handler:         &closedState{},
		}
		cb.breakers[name] = status
	} else if status.handler == nil {
		switch status.State {
		case StateOpen:
			status.handler = &openState{}
		case StateHalfOpen:
			status.handler = &halfOpenState{}
		default:
			status.handler = &closedState{}
		}
	}
	return status
}

// CanExecute checks if an upstream provider is healthy or allowed to test recovery.
//
// WARNING: this is a *mutating* admission check. In half-open state it consumes
// a canary slot, and it may flip an OPEN breaker to HALF-OPEN. Never call it from
// read-only paths (routing introspection, /api/v1/public/status, load balancing);
// use Healthy for those instead.
func (cb *CircuitBreaker) CanExecute(name string) bool {
	ok, _ := cb.CanExecuteGen(name)
	return ok
}

// CanExecuteGen admits a call and returns the breaker generation at admission.
// Callers that record an outcome later pass that generation back so a result
// from before a state transition is not counted as a half-open canary outcome.
func (cb *CircuitBreaker) CanExecuteGen(name string) (bool, uint64) {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	status := cb.getOrCreateStatusLocked(name)
	if cb.disabled {
		return true, status.Generation
	}
	return status.handler.CanExecute(status, cb, time.Now()), status.Generation
}

// Healthy reports whether a channel is currently considered usable, without
// mutating breaker state: it neither consumes a canary slot nor promotes an
// OPEN breaker to HALF-OPEN.
func (cb *CircuitBreaker) Healthy(name string) bool {
	cb.mu.RLock()
	defer cb.mu.RUnlock()

	// Advisory mode never rejects traffic, so a channel recorded as failing must
	// still be treated as usable by routing decisions (session affinity etc.).
	// Its state is for operators, not for admission control.
	if cb.disabled {
		return true
	}
	status, exists := cb.breakers[name]
	if !exists {
		return true
	}
	switch status.State {
	case StateOpen:
		return time.Since(status.LastStateChange) >= cb.cooldownDuration
	case StateHalfOpen:
		return status.CanaryInFlight < cb.maxCanaryProbes
	default:
		return true
	}
}

// RecordSuccess marks a successful execution, resetting breaker to healthy Closed state.
func (cb *CircuitBreaker) RecordSuccess(name string) {
	cb.mu.Lock()
	var gen uint64
	if s, ok := cb.breakers[name]; ok {
		gen = s.Generation
	}
	cb.mu.Unlock()
	cb.RecordSuccessGen(name, gen)
}

// RecordSuccessGen records a success tagged with the generation captured at
// admission. A stale result (generation mismatch) is ignored while half-open so
// a pre-trip success cannot close the breaker without a real canary probe.
func (cb *CircuitBreaker) RecordSuccessGen(name string, gen uint64) {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	status, exists := cb.breakers[name]
	if !exists {
		return
	}
	if status.State == StateHalfOpen && gen != status.Generation {
		return
	}
	if cb.disabled {
		// Advisory mode: clear the recorded failure state so recovery is visible
		// to operators (nothing was ever blocked).
		status.ConsecutiveFailures = 0
		status.ConsecutiveSuccesses = 0
		if status.State != StateClosed {
			status.toClosed(time.Now())
		}
		return
	}
	if status.handler == nil {
		status.handler = &closedState{}
	}
	status.handler.RecordSuccess(status, cb, time.Now())
}

// RecordFailure marks a failure, potentially tripping the breaker to Open state.
func (cb *CircuitBreaker) RecordFailure(name string) {
	cb.mu.Lock()
	var gen uint64
	if s, ok := cb.breakers[name]; ok {
		gen = s.Generation
	}
	cb.mu.Unlock()
	cb.RecordFailureGen(name, gen)
}

// RecordFailureGen records a failure tagged with the admission generation; a
// stale result is ignored while half-open so a pre-trip failure cannot reopen
// the breaker and reset its cooldown without a real canary probe.
func (cb *CircuitBreaker) RecordFailureGen(name string, gen uint64) {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	status := cb.getOrCreateStatusLocked(name)
	if status.State == StateHalfOpen && gen != status.Generation {
		return
	}
	if cb.disabled {
		// Advisory mode (GATEWAY_BREAKER_ENABLED=false): the outcome is recorded for
		// operators, but the breaker never rejects traffic.
		cb.recordAdvisoryFailureLocked(status, time.Now())
		return
	}
	status.handler.RecordFailure(status, cb, time.Now())
}

// recordAdvisoryFailureLocked records a failure without ever blocking traffic.
// It mirrors closedState.RecordFailure's threshold logic so the console shows the
// same TRIPPED state a protective breaker would have shown.
func (cb *CircuitBreaker) recordAdvisoryFailureLocked(status *BreakerStatus, now time.Time) {
	status.ConsecutiveFailures++
	if status.State == StateClosed && status.ConsecutiveFailures >= cb.failureThreshold {
		status.State = StateOpen
		status.handler = &openState{}
		status.CanaryInFlight = 0
		status.LastStateChange = now
		telemetry.ScopedLogger("breaker").Warn("circuit breaker advisory: channel is failing (traffic is STILL allowed because GATEWAY_BREAKER_ENABLED=false)",
			"provider", status.name,
			"consecutive_failures", status.ConsecutiveFailures,
			"cooldown_sec", cb.cooldownDuration.Seconds(),
		)
	}
}

// AbortCanary decrements the canary in-flight count if a canary probe was aborted
// (e.g. client cancellation) during half-open recovery, preventing canary lockouts.
func (cb *CircuitBreaker) AbortCanary(name string) {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	status, exists := cb.breakers[name]
	if !exists {
		return
	}
	if status.State == StateHalfOpen && status.CanaryInFlight > 0 {
		status.CanaryInFlight--
	}
}

// GetStatus returns the current state of a provider's circuit breaker.
func (cb *CircuitBreaker) GetStatus(name string) CircuitState {
	cb.mu.RLock()
	defer cb.mu.RUnlock()

	if status, exists := cb.breakers[name]; exists {
		return status.State
	}
	return StateClosed
}
