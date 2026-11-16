package router

import (
	"time"

	"github.com/ifnodoraemon/airoute/internal/telemetry"
)

// Breaker state transitions and the three state handlers (closed/open/half-open),
// extracted from circuit_breaker.go to keep the breaker core under the size red line.

func (b *BreakerStatus) toClosed(now time.Time) {
	b.State = StateClosed
	b.handler = &closedState{}
	b.ConsecutiveFailures = 0
	b.ConsecutiveSuccesses = 0
	b.CanaryInFlight = 0
	b.Generation++
	b.LastStateChange = now
	telemetry.ScopedLogger("breaker").Info("circuit breaker recovered to closed state", "provider", b.name)
}

func (b *BreakerStatus) toOpen(now time.Time, cooldown time.Duration) {
	b.State = StateOpen
	b.handler = &openState{}
	b.ConsecutiveSuccesses = 0
	b.CanaryInFlight = 0
	b.Generation++
	b.LastStateChange = now
	telemetry.ScopedLogger("breaker").Warn("circuit breaker TRIPPED to OPEN state, bypassing provider",
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
	b.Generation++
	b.LastStateChange = now
	telemetry.ScopedLogger("breaker").Info("circuit breaker entered half-open state, allowing canary probe", "provider", b.name)
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

// RecordSuccess ignores outcomes reported while the breaker is OPEN.
//
// Those results belong to requests that were admitted *before* the trip: an
// upstream that rejects with 429 still completes earlier in-flight calls
// successfully. Closing on the first such success reset the cooldown timer to
// zero, so a busy channel produced TRIPPED -> recovered cycles a few
// milliseconds apart (15 flips in under a second), hammering the rate-limited
// upstream instead of backing off. Recovery is owned by the cooldown +
// half-open canary path.
func (s *openState) RecordSuccess(b *BreakerStatus, cb *CircuitBreaker, now time.Time) {}

// RecordFailure likewise does not extend the cooldown. It used to reset
// LastStateChange on every late failure, so a continuous trickle of in-flight
// failures could keep the breaker OPEN forever.
func (s *openState) RecordFailure(b *BreakerStatus, cb *CircuitBreaker, now time.Time) {}

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