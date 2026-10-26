package router

import (
	"testing"
	"time"
)

// With a single upstream channel there is nothing to fail over to, so an open
// circuit only converts upstream errors into gateway 502s: advisory mode must keep
// letting traffic through.
func TestDisabledBreakerNeverRejectsTraffic(t *testing.T) {
	breaker := NewCircuitBreaker(3, 30*time.Second)
	breaker.SetDisabled(true)

	for i := 0; i < 20; i++ {
		breaker.RecordFailure("罗总")
	}
	if !breaker.CanExecute("罗总") {
		t.Fatal("advisory breaker rejected traffic")
	}
}

// The enabled breaker still protects a multi-channel deployment.
func TestEnabledBreakerStillTrips(t *testing.T) {
	breaker := NewCircuitBreaker(3, 30*time.Second)
	for i := 0; i < 3; i++ {
		breaker.RecordFailure("ch")
	}
	if breaker.CanExecute("ch") {
		t.Fatal("breaker did not open after reaching the failure threshold")
	}
}

func TestBreakerConfigFromEnv(t *testing.T) {
	t.Setenv("GATEWAY_BREAKER_COOLDOWN_SEC", "7")
	t.Setenv("GATEWAY_BREAKER_FAILURES", "5")
	if got := breakerCooldown(); got != 7*time.Second {
		t.Fatalf("cooldown = %v, want 7s", got)
	}
	if got := breakerFailureThreshold(); got != 5 {
		t.Fatalf("threshold = %d, want 5", got)
	}

	t.Setenv("GATEWAY_BREAKER_ENABLED", "false")
	if breakerEnabled() {
		t.Fatal("GATEWAY_BREAKER_ENABLED=false did not disable the breaker")
	}
	t.Setenv("GATEWAY_BREAKER_ENABLED", "")
	if !breakerEnabled() {
		t.Fatal("breaker should be enabled by default")
	}
}
