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
	name                 string
	handler              circuitStateHandler
}

func (b *BreakerStatus) toClosed(now time.Time) {
	b.State = StateClosed
	b.handler = &closedState{}
	b.ConsecutiveFailures = 0
	b.ConsecutiveSuccesses = 0
	b.CanaryInFlight = 0
	b.LastStateChange = now
	telemetry.Logger.Info("circuit breaker recovered to closed state", "provider", b.name)
}

func (b *BreakerStatus) toOpen(now time.Time, cooldown time.Duration) {
	b.State = StateOpen
	b.handler = &openState{}
	b.ConsecutiveSuccesses = 0
	b.CanaryInFlight = 0
	b.LastStateChange = now
	telemetry.Logger.Warn("circuit breaker TRIPPED to OPEN state, bypassing provider",
		"provider", b.name,
		"consecutive_failures", b.ConsecutiveFailures,
		"cooldown_sec", cooldown.Seconds(),
	)
}

func (b *BreakerStatus) toHalfOpen(now time.Time) {
	b.State = StateHalfOpen
	b.handler = &halfOpenState{}
	b.ConsecutiveSuccesses = 0
	b.CanaryInFlight = 0
	b.LastStateChange = now
	telemetry.Logger.Info("circuit breaker entered half-open state, allowing canary probe", "provider", b.name)
}

// closedState handles requests when circuit is healthy.
type closedState struct{}

func (s *closedState) State() CircuitState { return StateClosed }

func (s *closedState) CanExecute(b *BreakerStatus, cb *CircuitBreaker, now time.Time) bool {
	return true
}

func (s *closedState) RecordSuccess(b *BreakerStatus, cb *CircuitBreaker, now time.Time) {
	b.ConsecutiveFailures = 0
}

func (s *closedState) RecordFailure(b *BreakerStatus, cb *CircuitBreaker, now time.Time) {
	b.ConsecutiveFailures++
	if b.ConsecutiveFailures >= cb.failureThreshold {
		b.toOpen(now, cb.cooldownDuration)
	}
}

// openState handles requests when circuit is tripped.
type openState struct{}

func (s *openState) State() CircuitState { return StateOpen }

func (s *openState) CanExecute(b *BreakerStatus, cb *CircuitBreaker, now time.Time) bool {
	if now.Sub(b.LastStateChange) >= cb.cooldownDuration {
		b.toHalfOpen(now)
		return b.handler.CanExecute(b, cb, now)
	}
	return false
}

func (s *openState) RecordSuccess(b *BreakerStatus, cb *CircuitBreaker, now time.Time) {
	b.toClosed(now)
}

func (s *openState) RecordFailure(b *BreakerStatus, cb *CircuitBreaker, now time.Time) {
	b.LastStateChange = now
}

// halfOpenState handles canary probe testing during recovery.
type halfOpenState struct{}

func (s *halfOpenState) State() CircuitState { return StateHalfOpen }

func (s *halfOpenState) CanExecute(b *BreakerStatus, cb *CircuitBreaker, now time.Time) bool {
	// Limit concurrent canary probes to prevent herd stampede
	if b.CanaryInFlight >= cb.maxCanaryProbes {
		// If canary in-flight was abandoned for too long (> 2x cooldown), reset it
		if now.Sub(b.LastStateChange) > 2*cb.cooldownDuration {
			b.CanaryInFlight = 0
		} else {
			return false
		}
	}
	b.CanaryInFlight++
	return true
}

func (s *halfOpenState) RecordSuccess(b *BreakerStatus, cb *CircuitBreaker, now time.Time) {
	if b.CanaryInFlight > 0 {
		b.CanaryInFlight--
	}
	b.ConsecutiveSuccesses++
	if b.ConsecutiveSuccesses >= cb.successThreshold {
		b.toClosed(now)
	}
}

func (s *halfOpenState) RecordFailure(b *BreakerStatus, cb *CircuitBreaker, now time.Time) {
	b.toOpen(now, cb.cooldownDuration)
}

// CircuitBreaker manages failover and auto-recovery for all upstream providers.
type CircuitBreaker struct {
	mu               sync.RWMutex
	breakers         map[string]*BreakerStatus
	failureThreshold int
	successThreshold int
	maxCanaryProbes  int
	cooldownDuration time.Duration
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
func (cb *CircuitBreaker) CanExecute(name string) bool {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	status := cb.getOrCreateStatusLocked(name)
	return status.handler.CanExecute(status, cb, time.Now())
}

// RecordSuccess marks a successful execution, resetting breaker to healthy Closed state.
func (cb *CircuitBreaker) RecordSuccess(name string) {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	status, exists := cb.breakers[name]
	if !exists {
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
	defer cb.mu.Unlock()

	status := cb.getOrCreateStatusLocked(name)
	status.handler.RecordFailure(status, cb, time.Now())
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

