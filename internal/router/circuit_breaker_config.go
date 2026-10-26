package router

import (
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/ifnodoraemon/airoute/internal/telemetry"
)

const (
	defaultBreakerCooldown = 30 * time.Second
	defaultBreakerFailures = 3
)

// newConfiguredCircuitBreaker builds the process breaker from the environment.
//
// GATEWAY_BREAKER_ENABLED=false makes it advisory-only, which is the pragmatic
// setting while a model is served by a single channel: there is no alternative to
// fail over to, so an open circuit can only replace upstream error responses with
// gateway 502s. GATEWAY_BREAKER_COOLDOWN_SEC tunes how long it stays open, and
// GATEWAY_BREAKER_FAILURES tunes how many consecutive failures are needed.
func newConfiguredCircuitBreaker() *CircuitBreaker {
	breaker := NewCircuitBreaker(breakerFailureThreshold(), breakerCooldown())
	if !breakerEnabled() {
		breaker.SetDisabled(true)
		telemetry.ScopedLogger("router").Warn("circuit breaker runs in advisory mode; it records failures but never rejects traffic",
			"reason", "GATEWAY_BREAKER_ENABLED=false")
	}
	return breaker
}

func breakerEnabled() bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv("GATEWAY_BREAKER_ENABLED"))) {
	case "0", "false", "no", "off":
		return false
	}
	return true
}

func breakerCooldown() time.Duration {
	raw := strings.TrimSpace(os.Getenv("GATEWAY_BREAKER_COOLDOWN_SEC"))
	if raw == "" {
		return defaultBreakerCooldown
	}
	seconds, err := strconv.Atoi(raw)
	if err != nil || seconds <= 0 {
		return defaultBreakerCooldown
	}
	return time.Duration(seconds) * time.Second
}

func breakerFailureThreshold() int {
	raw := strings.TrimSpace(os.Getenv("GATEWAY_BREAKER_FAILURES"))
	if raw == "" {
		return defaultBreakerFailures
	}
	threshold, err := strconv.Atoi(raw)
	if err != nil || threshold <= 0 {
		return defaultBreakerFailures
	}
	return threshold
}
