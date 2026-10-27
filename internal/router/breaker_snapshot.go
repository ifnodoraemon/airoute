package router

import "time"

// BreakerSnapshot is a read-only observation, never an admission or a probe.
type BreakerSnapshot struct {
	State               string `json:"state"`
	Failures            int    `json:"consecutive_failures"`
	CanaryInFlight      int    `json:"canary_in_flight"`
	CanExecute          bool   `json:"can_execute"`
	RecoveryProbe       bool   `json:"recovery_probe"`
	CooldownRemainingMs int64  `json:"cooldown_remaining_ms"`
}

func (cb *CircuitBreaker) Snapshot(name string) BreakerSnapshot {
	cb.mu.RLock()
	defer cb.mu.RUnlock()
	snapshot := BreakerSnapshot{State: StateClosed.String(), CanExecute: true}
	status := cb.breakers[name]
	if status == nil {
		return snapshot
	}
	snapshot.State = status.State.String()
	snapshot.Failures = status.ConsecutiveFailures
	snapshot.CanaryInFlight = status.CanaryInFlight
	switch status.State {
	case StateOpen:
		remaining := cb.cooldownDuration - time.Since(status.LastStateChange)
		snapshot.CooldownRemainingMs = max(0, remaining.Milliseconds())
		snapshot.CanExecute = remaining <= 0
		snapshot.RecoveryProbe = snapshot.CanExecute
	case StateHalfOpen:
		snapshot.RecoveryProbe = true
		snapshot.CanExecute = status.CanaryInFlight < cb.maxCanaryProbes || time.Since(status.LastStateChange) > 2*cb.cooldownDuration
	}
	if cb.disabled {
		// Advisory mode: the recorded state is informational only. Nothing is ever
		// rejected, so the snapshot must not claim the channel is unusable.
		snapshot.CanExecute = true
		snapshot.RecoveryProbe = false
		snapshot.CooldownRemainingMs = 0
	}
	return snapshot
}
