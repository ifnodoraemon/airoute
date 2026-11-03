package middleware

import (
	"testing"

	"github.com/ifnodoraemon/airoute/internal/distributed"
	"github.com/ifnodoraemon/airoute/internal/telemetry"
)

func TestShouldFailClosedOnUnknownAccount(t *testing.T) {
	cases := []struct {
		name       string
		freeRead   bool
		userStatus string
		userID     int64
		want       bool
	}{
		{"known active user is admitted", false, "active", 7, false},
		{"known locked user is decided elsewhere", false, "locked", 7, false},
		{"unknown status with an owner fails closed", false, "", 7, true},
		{"key without an owner keeps open/dev-mode behaviour", false, "", 0, false},
		{"free read is never blocked by unknown state", true, "", 7, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := shouldFailClosedOnUnknownAccount(tc.freeRead, tc.userStatus, tc.userID); got != tc.want {
				t.Fatalf("shouldFailClosedOnUnknownAccount(%v, %q, %d) = %v, want %v",
					tc.freeRead, tc.userStatus, tc.userID, got, tc.want)
			}
		})
	}
}

// TestBalanceUnknownDenialIsRetryableDocumentedContract pins the response shape:
// a 503 with Retry-After, because a database stall is transient by definition.
func TestBalanceUnknownDenial(t *testing.T) {
	denial := BalanceUnknownDenial()
	if denial == nil {
		t.Fatal("nil denial")
	}
	if denial.StatusCode != 503 {
		t.Fatalf("status = %d, want 503", denial.StatusCode)
	}
	if denial.Code != "account_state_unavailable" {
		t.Fatalf("code = %q", denial.Code)
	}
	if denial.Headers["Retry-After"] == "" {
		t.Fatal("missing Retry-After header: a fail-closed denial must tell the client to retry")
	}
}

// TestTopUpUserQuotaForUserTracksReservedTotal exercises the MCP-facing variant:
// reserve, top up, release, and the reserved-units gauge must balance.
func TestTopUpUserQuotaForUserTracksReservedTotal(t *testing.T) {
	if rClient := distributed.GetClient(); rClient != nil && rClient.IsActive() {
		t.Skip("redis is active: this test pins the process-local fallback accounting")
	}
	const userID int64 = 424242
	before := telemetry.GlobalMetrics.BillingReservedUnits.Load()

	coarse := InflightReserveCNY()
	acquired, backend := ReserveUserQuotaAmount(userID, 100, coarse)
	if !acquired {
		t.Fatal("coarse reservation refused for a funded user")
	}
	if backend != QuotaBackendLocal {
		t.Fatalf("backend = %q, want local fallback", backend)
	}

	// Top up to 0.5 CNY (a long-context estimate). The wallet covers it.
	reserved := TopUpUserQuotaForUser(userID, backend, 100, 0.5, coarse)
	if reserved < 0.5 {
		t.Fatalf("reserved after top-up = %f, want >= 0.5", reserved)
	}

	// A second top-up below the current total is a no-op.
	if again := TopUpUserQuotaForUser(userID, backend, 100, 0.2, reserved); again != reserved {
		t.Fatalf("redundant top-up changed the reservation: %f -> %f", reserved, again)
	}

	ReleaseUserQuotaAmount(userID, backend, reserved)
	if got := telemetry.GlobalMetrics.BillingReservedUnits.Load(); got != before {
		t.Fatalf("reserved gauge = %d after release, want %d (reserve/top-up/release must balance)", got, before)
	}
}

// A top-up can never reserve more than the wallet can cover: the excess is
// clamped, and the returned total reflects only what was actually booked.
func TestTopUpUserQuotaForUserClampsToWallet(t *testing.T) {
	if rClient := distributed.GetClient(); rClient != nil && rClient.IsActive() {
		t.Skip("redis is active: this test pins the process-local fallback accounting")
	}
	const userID int64 = 424243
	before := telemetry.GlobalMetrics.BillingReservedUnits.Load()

	coarse := InflightReserveCNY()
	acquired, backend := ReserveUserQuotaAmount(userID, 1, coarse)
	if !acquired {
		t.Fatal("coarse reservation refused")
	}
	// Ask for far more than the 1 CNY wallet holds.
	reserved := TopUpUserQuotaForUser(userID, backend, 1, 50, coarse)
	if reserved > 1+coarse {
		t.Fatalf("reserved = %f, want clamped to the wallet's ~1 CNY", reserved)
	}
	ReleaseUserQuotaAmount(userID, backend, reserved)
	if got := telemetry.GlobalMetrics.BillingReservedUnits.Load(); got != before {
		t.Fatalf("reserved gauge = %d after release, want %d", got, before)
	}
}
