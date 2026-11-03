package storage

import (
	"context"
	"testing"
	"time"
)

// TestAuditLagIgnoresTheTrailingOverlapOnBothSides is the regression test for the
// phantom lag the live deployment reported: the gauge summed every settlement but
// only counted audit evidence up to now-5min, so on every pass under traffic it
// reported roughly the last five minutes of billing as "missing audit evidence"
// (0.4-0.8 CNY on a quiet gateway, with the ledger itself perfectly balanced).
//
// The fixture keeps the shipped overlap window: a charge inside the trailing
// window is deliberately excluded from both sides, so it must not move the gauge.
// The charge is balanced on both sides (RecordUsageLogContext writes the audit
// row and settles it), which is exactly the healthy steady state.
func TestAuditLagIgnoresTheTrailingOverlapOnBothSides(t *testing.T) {
	// No auditLagTestRepo here on purpose: this test is about the production
	// 5-minute watermark, not about the incremental window.
	r, key, u := billingIDTestRepo(t)
	ctx := context.Background()

	// A balanced charge that is older than the watermark: it is on both sides.
	past := billingRecord(key, u.Username, "tr-overlap-old", "bill-overlap-old", 1.00)
	past.CreatedAt = time.Now().Add(-30 * time.Minute)
	if err := r.RecordUsageLogContext(ctx, past); err != nil {
		t.Fatalf("record historical charge: %v", err)
	}

	// First pass calibrates at the watermark and must see no difference.
	if got := runAuditLag(t, r); got != 0 {
		t.Fatalf("calibration lag = %d units, want 0", got)
	}

	// A balanced charge created inside the trailing window. It is newer than the
	// watermark, so both sums exclude it. Before the fix the settled side counted
	// it while the audit side could not, which is precisely the 2.00 of phantom lag
	// this asserts against.
	fresh := billingRecord(key, u.Username, "tr-overlap-fresh", "bill-overlap-fresh", 2.00)
	fresh.CreatedAt = time.Now()
	if err := r.RecordUsageLogContext(ctx, fresh); err != nil {
		t.Fatalf("record fresh charge: %v", err)
	}
	if got := runAuditLag(t, r); got != 0 {
		t.Fatalf("lag = %d units, want 0: a balanced charge inside the overlap window is not missing evidence", got)
	}

	// Once the watermark moves past the charge (simulated by shrinking the overlap
	// to zero), the same balanced charge still nets out to zero.
	previous := billingAuditLagOverlapWindow
	billingAuditLagOverlapWindow = 0
	t.Cleanup(func() { billingAuditLagOverlapWindow = previous })
	if got := runAuditLag(t, r); got != 0 {
		t.Fatalf("lag after the watermark passed the charge = %d units, want 0", got)
	}
}

// TestAuditLagWarnFloorOnlySuppressesImmaterialNoise pins the WARN gate: the gauge
// keeps the exact value, but a sub-cent difference (flush latency) must not page
// anyone.
func TestAuditLagWarnFloorOnlySuppressesImmaterialNoise(t *testing.T) {
	cases := []struct {
		lag  int64
		want bool
	}{
		{0, false},
		{billingAuditLagWarnFloorUnits - 1, false},
		{billingAuditLagWarnFloorUnits, true},
		{ToUnits(0.82), true},
	}
	for _, c := range cases {
		if got := auditLagIsMaterial(c.lag); got != c.want {
			t.Fatalf("auditLagIsMaterial(%d) = %v, want %v", c.lag, got, c.want)
		}
	}
}
