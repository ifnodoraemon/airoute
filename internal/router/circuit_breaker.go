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

// BreakerStatus tracks state for a single upstream channel.
type BreakerStatus struct {
	State               CircuitState
	ConsecutiveFailures int
	LastStateChange     time.Time
}

// CircuitBreaker manages failover and auto-recovery for all upstream providers.
type CircuitBreaker struct {
	mu                   sync.RWMutex
	breakers             map[string]*BreakerStatus
	failureThreshold     int
	cooldownDuration     time.Duration
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
		cooldownDuration: cooldown,
	}
}

// CanExecute checks if an upstream provider is healthy or allowed to test recovery.
func (cb *CircuitBreaker) CanExecute(name string) bool {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	status, exists := cb.breakers[name]
	if !exists {
		status = &BreakerStatus{
			State:           StateClosed,
			LastStateChange: time.Now(),
		}
		cb.breakers[name] = status
		return true
	}

	now := time.Now()

	switch status.State {
	case StateClosed:
		return true

	case StateOpen:
		// Check if cooldown has elapsed
		if now.Sub(status.LastStateChange) >= cb.cooldownDuration {
			status.State = StateHalfOpen
			status.LastStateChange = now
			telemetry.Logger.Info("circuit breaker entered half-open state, allowing canary probe", "provider", name)
			return true
		}
		// Still in open state, fail fast!
		return false

	case StateHalfOpen:
		// In half-open state, allow canary request
		return true
	}

	return true
}

// RecordSuccess marks a successful execution, resetting breaker to healthy Closed state.
func (cb *CircuitBreaker) RecordSuccess(name string) {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	status, exists := cb.breakers[name]
	if !exists {
		return
	}

	if status.State != StateClosed {
		telemetry.Logger.Info("circuit breaker recovered to closed state", "provider", name)
	}

	status.State = StateClosed
	status.ConsecutiveFailures = 0
	status.LastStateChange = time.Now()
}

// RecordFailure marks a failure, potentially tripping the breaker to Open state.
func (cb *CircuitBreaker) RecordFailure(name string) {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	status, exists := cb.breakers[name]
	if !exists {
		status = &BreakerStatus{
			State:           StateClosed,
			LastStateChange: time.Now(),
		}
		cb.breakers[name] = status
	}

	status.ConsecutiveFailures++
	now := time.Now()

	if status.State == StateHalfOpen || status.ConsecutiveFailures >= cb.failureThreshold {
		if status.State != StateOpen {
			telemetry.Logger.Warn("circuit breaker TRIPPED to OPEN state, bypassing provider",
				"provider", name,
				"consecutive_failures", status.ConsecutiveFailures,
				"cooldown_sec", cb.cooldownDuration.Seconds(),
			)
		}
		status.State = StateOpen
		status.LastStateChange = now
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
