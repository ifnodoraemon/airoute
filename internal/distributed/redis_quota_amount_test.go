package distributed

import (
	"context"
	"math"
	"strings"
	"testing"
)

func TestQuotaAmountUnitsConversion(t *testing.T) {
	cases := []struct {
		name        string
		amountCNY   float64
		wantBalance int64 // rounds to the nearest sub-unit, like the ledger
		wantReserve int64 // rounds up, so a reservation never under-covers
	}{
		{"default reserve", 0.05, 50_000_000, 50_000_000},
		{"nine decimals", 1.234567891, 1_234_567_891, 1_234_567_891},
		{"sub-unit balance rounds down", 0.0000000004, 0, 1},
		{"sub-unit balance rounds up", 0.0000000006, 1, 1},
		{"zero", 0, 0, 0},
		{"negative", -1.5, 0, 0},
		{"NaN", math.NaN(), 0, 0},
		{"infinity", math.Inf(1), 0, 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := quotaBalanceUnits(tc.amountCNY); got != tc.wantBalance {
				t.Fatalf("quotaBalanceUnits(%v) = %d, want %d", tc.amountCNY, got, tc.wantBalance)
			}
			if got := quotaReserveUnits(tc.amountCNY); got != tc.wantReserve {
				t.Fatalf("quotaReserveUnits(%v) = %d, want %d", tc.amountCNY, got, tc.wantReserve)
			}
		})
	}
}

func TestInflightQuotaAmountKeys(t *testing.T) {
	amountKey, countKey := inflightQuotaAmountKeys(42)
	if amountKey != "airoute:inflight:amt:42" {
		t.Fatalf("amount key = %q", amountKey)
	}
	if countKey != "airoute:inflight:cnt:42" {
		t.Fatalf("count key = %q", countKey)
	}
	// The two counters must never be the same key: a crash-expiry on one must not
	// silently release the other.
	if amountKey == countKey {
		t.Fatalf("amount and count keys collide: %q", amountKey)
	}
}

// TestQuotaAmountScriptsKeepBothGates pins the two conditions the Lua gate exists
// for. The scripts cannot be executed without a Redis server, but a regression
// that drops the amount ceiling or the concurrency cap would silently restore the
// pre-fix behaviour, so the source is checked directly.
func TestQuotaAmountScriptsKeepBothGates(t *testing.T) {
	for name, fragment := range map[string]string{
		"amount ceiling":    "balance - reserved - need < 0",
		"concurrency cap":   "inflight + 1 > maxInflight",
		"books amount":      "INCRBY",
		"books slot":        "INCR",
		"expires both keys": "EXPIRE",
	} {
		if !strings.Contains(quotaAmountReserveLuaSrc, fragment) {
			t.Fatalf("reserve script lost %q:\n%s", name, quotaAmountReserveLuaSrc)
		}
	}
	for name, fragment := range map[string]string{
		"clamped amount decrement": "DECRBY",
		"deletes empty amount key": "DEL",
		"clamped count decrement":  "DECR",
	} {
		if !strings.Contains(quotaAmountReleaseLuaSrc, fragment) {
			t.Fatalf("release script lost %q:\n%s", name, quotaAmountReleaseLuaSrc)
		}
	}
	// The release must never subtract more than was booked: the clamp is the
	// guard against a settled cost larger than the reservation draining another
	// request's reservation.
	if !strings.Contains(quotaAmountReleaseLuaSrc, "units > reserved") {
		t.Fatalf("release script does not clamp to the booked amount:\n%s", quotaAmountReleaseLuaSrc)
	}
}

// TestQuotaAmountInactiveClientFailsOpen covers the single-node / Redis-down
// contract: the distributed gate reports "admitted" and the caller's local
// tracker owns the reservation instead.
func TestQuotaAmountInactiveClientFailsOpen(t *testing.T) {
	ctx := context.Background()

	var nilClient *Client
	for name, client := range map[string]*Client{
		"nil client":    nilClient,
		"uninitialised": {},
	} {
		t.Run(name, func(t *testing.T) {
			allowed, err := client.TryReserveQuotaAmount(ctx, 7, 1.0, 0.05, 64)
			if err != nil || !allowed {
				t.Fatalf("TryReserveQuotaAmount = (%v, %v), want (true, nil)", allowed, err)
			}
			if err := client.ReleaseQuotaAmount(ctx, 7, 50_000_000); err != nil {
				t.Fatalf("ReleaseQuotaAmount = %v, want nil", err)
			}
		})
	}
}

func TestQuotaAmountRejectsInvalidUser(t *testing.T) {
	client := &Client{}
	ctx := context.Background()

	allowed, err := client.TryReserveQuotaAmount(ctx, 0, 1.0, 0.05, 64)
	if err != nil || !allowed {
		t.Fatalf("userID 0: TryReserveQuotaAmount = (%v, %v), want (true, nil)", allowed, err)
	}
	if err := client.ReleaseQuotaAmount(ctx, 0, 1); err != nil {
		t.Fatalf("userID 0: ReleaseQuotaAmount = %v, want nil", err)
	}
	if err := client.ReleaseQuotaAmount(ctx, 7, 0); err != nil {
		t.Fatalf("zero units: ReleaseQuotaAmount = %v, want nil", err)
	}
}
