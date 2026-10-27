package config

import (
	"testing"
	"time"

	"github.com/ifnodoraemon/airoute/internal/telemetry"
)

// Regression (ST-12): an unparsable or negative trial-balance override used to be
// ignored silently, so every new account received the built-in 5.0 free credit
// while the operator believed a different value was configured.
//
// The behaviour is now deterministic (grant nothing) and reported once.
func TestInvalidTrialBalanceOverrideIsRejectedDeterministically(t *testing.T) {
	telemetry.ResetWarnGate()
	t.Cleanup(telemetry.ResetWarnGate)

	cases := []struct {
		name  string
		value string
	}{
		{"unparsable", "not-a-number"},
		{"negative", "-5"},
		{"partial number", "1.2.3"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			telemetry.ResetWarnGate()
			t.Setenv("INITIAL_USER_BALANCE", tc.value)
			t.Setenv("DEFAULT_TRIAL_BALANCE", "")

			before := countTrialBalanceWarnings()
			if got := GetGlobalConfig().GetInitialUserBalance(); got != 0 {
				t.Fatalf("invalid INITIAL_USER_BALANCE=%q produced %.2f, want 0 (no free credit)", tc.value, got)
			}
			// The ring buffer keeps history across subtests, so compare the delta.
			deadline := time.Now().Add(2 * time.Second)
			for time.Now().Before(deadline) && countTrialBalanceWarnings()-before == 0 {
				time.Sleep(5 * time.Millisecond)
			}
			if delta := countTrialBalanceWarnings() - before; delta != 1 {
				t.Fatalf("expected exactly one configuration warning for this call, got %d", delta)
			}
		})
	}
}

func countTrialBalanceWarnings() int {
	count := 0
	for _, entry := range telemetry.GlobalRingBuffer.GetRecentLogs(1000, "", "", "", "") {
		if entry.Message == "invalid trial balance configuration; new accounts will start with 0 instead of free credit" {
			count++
		}
	}
	return count
}

// The documented behaviour for a *valid* override must be unchanged.
func TestValidTrialBalanceOverrideIsApplied(t *testing.T) {
	t.Setenv("INITIAL_USER_BALANCE", "12.5")
	t.Setenv("DEFAULT_TRIAL_BALANCE", "")
	if got := DefaultConfig().GetInitialUserBalance(); got != 12.5 {
		t.Fatalf("valid override produced %.2f, want 12.5", got)
	}

	t.Setenv("INITIAL_USER_BALANCE", "")
	t.Setenv("DEFAULT_TRIAL_BALANCE", "7")
	if got := DefaultConfig().GetInitialUserBalance(); got != 7 {
		t.Fatalf("DEFAULT_TRIAL_BALANCE produced %.2f, want 7", got)
	}
}

// With no override at all the config value (or the built-in 5.0) still applies.
func TestTrialBalanceFallsBackToConfigWhenUnset(t *testing.T) {
	t.Setenv("INITIAL_USER_BALANCE", "")
	t.Setenv("DEFAULT_TRIAL_BALANCE", "")
	if got := DefaultConfig().GetInitialUserBalance(); got != 5.0 {
		t.Fatalf("default balance = %.2f, want 5.0", got)
	}
}
