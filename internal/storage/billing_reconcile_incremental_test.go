package storage

import (
	"context"
	"fmt"
	"testing"
	"time"
)

// fullAuditLagUnits is the reference implementation of the audit-lag gauge: the
// absolute difference between everything settled and everything the audit trail
// proves, computed with plain full-table SUMs. The incremental path must agree
// with it.
func fullAuditLagUnits(t *testing.T, r *Repository) int64 {
	t.Helper()
	var lag int64
	query := `SELECT COALESCE((SELECT SUM(cost_units) FROM billing_settlements), 0)
		+ COALESCE((SELECT SUM(cost_units) FROM ` + billingLedgerArchivesTable + `), 0)
		- (COALESCE((SELECT SUM(cost_units) FROM usage_logs), 0) + ?)`
	if err := r.db.QueryRow(query, r.UsageAuditPrunedUnits()).Scan(&lag); err != nil {
		t.Fatalf("reference audit lag: %v", err)
	}
	if lag < 0 {
		lag = -lag
	}
	return lag
}

// runAuditLag executes one reconciliation pass of the audit-lag gauge and returns
// the reported (absolute) value.
func runAuditLag(t *testing.T, r *Repository) int64 {
	t.Helper()
	worker := &BillingReconciler{repo: r, stop: make(chan struct{})}
	lag, err := worker.auditLag()
	if err != nil {
		t.Fatalf("audit lag: %v", err)
	}
	return lag
}

// auditLagEvidenceBaseline reads the persisted audit-evidence baseline, i.e. the
// number that proves whether the incremental path or the full recalibration ran.
func auditLagEvidenceBaseline(t *testing.T, r *Repository) int64 {
	t.Helper()
	units, ok := settingInt64(r, billingAuditLagBaselineEvidenceSetting)
	if !ok {
		t.Fatalf("settings key %s is missing", billingAuditLagBaselineEvidenceSetting)
	}
	return units
}

// auditLagTestRepo builds a repo with a key and pins the overlap window to zero so
// a test can exercise the incremental watermark without sleeping for minutes.
// Production keeps the 5-minute window; the algorithm is the same, only the
// trailing slack changes.
func auditLagTestRepo(t *testing.T) (*Repository, *APIKeyRecord, *UserRecord) {
	t.Helper()
	previous := billingAuditLagOverlapWindow
	billingAuditLagOverlapWindow = 0
	t.Cleanup(func() { billingAuditLagOverlapWindow = previous })
	return billingIDTestRepo(t)
}

// TestAuditLagIncrementalMatchesFullRecalculation is the central correctness
// requirement: the rolling baseline must report exactly what a full recomputation
// would, pass after pass, and it must only add the rows created since the
// previous pass.
func TestAuditLagIncrementalMatchesFullRecalculation(t *testing.T) {
	r, key, u := auditLagTestRepo(t)
	ctx := context.Background()

	// Historical, already balanced charges. Their created_at is well before the
	// first watermark, so no pass may re-count them.
	old := time.Now().Add(-2 * time.Hour)
	for i := 0; i < 2; i++ {
		rec := billingRecord(key, u.Username, fmt.Sprintf("tr-inc-old-%d", i), fmt.Sprintf("bill-inc-old-%d", i), 1.25)
		rec.CreatedAt = old.Add(time.Duration(i) * time.Minute)
		if err := r.RecordUsageLogContext(ctx, rec); err != nil {
			t.Fatalf("record old charge %d: %v", i, err)
		}
	}

	// Pass 1 has no baseline, so it calibrates with a full SUM.
	if got := runAuditLag(t, r); got != 0 {
		t.Fatalf("calibration pass lag = %d, want 0", got)
	}
	calibrated := auditLagEvidenceBaseline(t, r)
	if want := ToUnits(2.50); calibrated != want {
		t.Fatalf("calibrated evidence baseline = %d units, want %d", calibrated, want)
	}

	// Pass 2 must take the incremental path: nothing was written, so the evidence
	// baseline is unchanged and the result still matches the full computation.
	if got := runAuditLag(t, r); got != 0 {
		t.Fatalf("incremental pass lag = %d, want 0", got)
	}
	if got := auditLagEvidenceBaseline(t, r); got != calibrated {
		t.Fatalf("incremental pass rewrote the evidence baseline: %d -> %d; it should only add the window", calibrated, got)
	}

	// New money on both sides, all created after the baseline: a settlement with no
	// audit row (0.75) and an audit row with no settlement (0.50). The difference is
	// 0.25 and only the incremental window can see it.
	if err := r.SettleUsageCharge(ctx, billingRecord(key, u.Username,
		"tr-inc-settle-only", "bill-inc-settle-only", 0.75)); err != nil {
		t.Fatalf("settle without audit row: %v", err)
	}
	auditOnly := billingRecord(key, u.Username, "tr-inc-audit-only", "bill-inc-audit-only", 0.50)
	auditOnly.CreatedAt = time.Now()
	if err := r.insertUsageLogsContext(ctx, []*UsageLogRecord{auditOnly}); err != nil {
		t.Fatalf("write audit row without settlement: %v", err)
	}

	if got, want := runAuditLag(t, r), ToUnits(0.25); got != want {
		t.Fatalf("incremental pass lag = %d units, want %d", got, want)
	}
	if got, want := runAuditLag(t, r), fullAuditLagUnits(t, r); got != want {
		t.Fatalf("incremental pass lag = %d units, full recomputation = %d units; they must agree", got, want)
	}
	if got, want := runAuditLag(t, r), ToUnits(0.25); got != want {
		t.Fatalf("repeated incremental pass lag = %d units, want %d (a quiet pass must not change it)", got, want)
	}
}

// TestAuditLagIncrementalDoesNotRescanTheAuditTable proves the audit side really is
// incremental: a row whose created_at predates the watermark is not picked up
// until the daily recalibration, which is the documented trade-off for not running
// a full SUM every pass.
func TestAuditLagIncrementalDoesNotRescanTheAuditTable(t *testing.T) {
	r, key, u := auditLagTestRepo(t)
	ctx := context.Background()

	old := time.Now().Add(-2 * time.Hour)
	record := billingRecord(key, u.Username, "tr-late-base", "bill-late-base", 1.00)
	record.CreatedAt = old
	if err := r.RecordUsageLogContext(ctx, record); err != nil {
		t.Fatal(err)
	}
	if got := runAuditLag(t, r); got != 0 {
		t.Fatalf("calibration lag = %d, want 0", got)
	}
	evidence := auditLagEvidenceBaseline(t, r)

	// A late audit row written with an old created_at (spill replay, a repair that
	// copies the original timestamp). It is behind the watermark by design.
	late := billingRecord(key, u.Username, "tr-late-arrival", "bill-late-arrival", 0.40)
	late.CreatedAt = old
	if err := r.insertUsageLogsContext(ctx, []*UsageLogRecord{late}); err != nil {
		t.Fatal(err)
	}

	if got := runAuditLag(t, r); got != 0 {
		t.Fatalf("incremental lag = %d, want 0: a row behind the watermark must not be counted twice", got)
	}
	if got := auditLagEvidenceBaseline(t, r); got != evidence {
		t.Fatalf("evidence baseline moved for a row behind the watermark: %d -> %d", evidence, got)
	}
	if got, want := fullAuditLagUnits(t, r), ToUnits(0.40); got != want {
		t.Fatalf("full recomputation = %d units, want %d (sanity check on the fixture)", got, want)
	}

	// Forcing the 24h recalibration makes the late row visible and restores the
	// agreement with the full computation.
	stale := time.Now().Add(-25 * time.Hour).UTC().Format(sqliteTimestampLayout)
	if err := r.SetSetting(billingAuditLagBaselineAtSetting, stale); err != nil {
		t.Fatal(err)
	}
	if got, want := runAuditLag(t, r), fullAuditLagUnits(t, r); got != want {
		t.Fatalf("recalibrated lag = %d units, full recomputation = %d units", got, want)
	}
	if got, want := runAuditLag(t, r), ToUnits(0.40); got != want {
		t.Fatalf("recalibrated lag = %d units, want %d", got, want)
	}
}

// TestAuditLagStaysConsistentWhenLedgerRowsAreArchived covers the interaction with
// the ledger lifecycle: pruning audit rows and then retiring the matching ledger
// rows must not move the gauge, and both the incremental pass and a fresh
// recalibration must still agree with the full computation.
func TestAuditLagStaysConsistentWhenLedgerRowsAreArchived(t *testing.T) {
	r, key, u := auditLagTestRepo(t)
	ctx := context.Background()

	old := time.Now().Add(-72 * time.Hour)
	for i := 0; i < 2; i++ {
		record := billingRecord(key, u.Username, fmt.Sprintf("tr-arch-%d", i), fmt.Sprintf("bill-arch-%d", i), 1.50)
		record.CreatedAt = old.Add(time.Duration(i) * time.Minute)
		if err := r.RecordUsageLogContext(ctx, record); err != nil {
			t.Fatalf("record %d: %v", i, err)
		}
	}
	if got := runAuditLag(t, r); got != 0 {
		t.Fatalf("calibration lag = %d, want 0", got)
	}

	// Audit retention removes the evidence and moves the units to the pruned
	// baseline, which is neutral for the gauge.
	if _, err := r.PurgeUsageLogsBefore(time.Now().Add(-24*time.Hour), 100); err != nil {
		t.Fatalf("prune audit rows: %v", err)
	}
	if got := runAuditLag(t, r); got != 0 {
		t.Fatalf("lag after audit pruning = %d, want 0", got)
	}

	// The ledger rows are now retirable: their audit rows are gone and they are
	// older than the retention window.
	retirer := &BillingLedgerRetention{repo: r, retention: 24 * time.Hour}
	deleted, archivedUnits, err := retirer.pruneOnce()
	if err != nil {
		t.Fatalf("retire ledger rows: %v", err)
	}
	if deleted != 2 || archivedUnits != ToUnits(3.00) {
		t.Fatalf("retired %d rows / %d units, want 2 rows / %d units", deleted, archivedUnits, ToUnits(3.00))
	}

	if got, want := runAuditLag(t, r), int64(0); got != want {
		t.Fatalf("incremental lag after archival = %d units, want %d (archived money is still settled money)", got, want)
	}
	if got, want := runAuditLag(t, r), fullAuditLagUnits(t, r); got != want {
		t.Fatalf("incremental lag after archival = %d units, full recomputation = %d units", got, want)
	}

	// A forced recalibration must land on the same number: the archive table is part
	// of the settled total.
	stale := time.Now().Add(-25 * time.Hour).UTC().Format(sqliteTimestampLayout)
	if err := r.SetSetting(billingAuditLagBaselineAtSetting, stale); err != nil {
		t.Fatal(err)
	}
	if got, want := runAuditLag(t, r), int64(0); got != want {
		t.Fatalf("recalibrated lag after archival = %d units, want %d", got, want)
	}
	if got, want := runAuditLag(t, r), fullAuditLagUnits(t, r); got != want {
		t.Fatalf("recalibrated lag after archival = %d units, full recomputation = %d units", got, want)
	}

	// Re-running the retirer is a no-op: nothing retirable is left.
	deleted, archivedUnits, err = retirer.pruneOnce()
	if err != nil {
		t.Fatalf("second retire pass: %v", err)
	}
	if deleted != 0 || archivedUnits != 0 {
		t.Fatalf("second retire pass deleted %d rows / %d units, want a no-op", deleted, archivedUnits)
	}
}

// TestAuditLagRecalibratesOnPrunedRegression covers the guard that makes a restored
// or hand-edited settings table recalibrate instead of computing from a baseline
// that no longer describes the database.
func TestAuditLagRecalibratesOnPrunedRegression(t *testing.T) {
	r, key, u := auditLagTestRepo(t)
	ctx := context.Background()

	old := time.Now().Add(-48 * time.Hour)
	for i := 0; i < 2; i++ {
		record := billingRecord(key, u.Username, fmt.Sprintf("tr-regress-%d", i), fmt.Sprintf("bill-regress-%d", i), 1.00)
		record.CreatedAt = old
		if err := r.RecordUsageLogContext(ctx, record); err != nil {
			t.Fatalf("record %d: %v", i, err)
		}
	}
	if got := runAuditLag(t, r); got != 0 {
		t.Fatalf("calibration lag = %d, want 0", got)
	}

	// Rewrite the persisted pruned snapshot to a value above the actual counter,
	// exactly like a restore from a newer dump would.
	if err := r.SetSetting(billingAuditLagBaselinePrunedSetting, "999999999999"); err != nil {
		t.Fatal(err)
	}
	// A settlement whose audit row is missing makes the two paths disagree unless
	// the recalibration runs.
	if err := r.SettleUsageCharge(ctx, billingRecord(key, u.Username, "tr-regress-gap", "bill-regress-gap", 2.00)); err != nil {
		t.Fatal(err)
	}
	if got, want := runAuditLag(t, r), fullAuditLagUnits(t, r); got != want {
		t.Fatalf("lag after pruned-counter regression = %d units, full recomputation = %d units", got, want)
	}
	// The unbalanced settlement really is a lag of 2.00, and the following
	// incremental pass reproduces the same number.
	if got, want := runAuditLag(t, r), ToUnits(2.00); got != want {
		t.Fatalf("lag = %d units, want %d", got, want)
	}
	if got, want := runAuditLag(t, r), fullAuditLagUnits(t, r); got != want {
		t.Fatalf("incremental lag after recalibration = %d units, full recomputation = %d units", got, want)
	}
}
