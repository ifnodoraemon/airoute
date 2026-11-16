package storage

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/ifnodoraemon/airoute/internal/telemetry"
)

// auditLag is the absolute difference between the total settled and the total
// recorded in the audit trail.
//
// Audit rows are allowed to disappear on purpose: retention prunes expired rows,
// and both are charged evidence. Those removals are counted in the pruned-units
// baseline (in the same transaction as the delete), so this gauge keeps meaning
// "a charge was settled but its audit row is missing" instead of reporting the
// entire pruned history as a discrepancy forever.
//
// The original version ran two full-table SUMs on every pass, so it slowed down
// linearly with the audit table. It now keeps a rolling baseline in
// system_settings:
//
//   - Both sides are cut at the same watermark. The watermark trails the wall
//     clock by billingAuditLagOverlapWindow so that an audit row which lands
//     slightly after its charge (async flush, Redis redelivery) still falls
//     inside the next window instead of behind it. Applying that window to the
//     audit side only made the gauge report roughly "the last five minutes of
//     billing" as missing evidence on every incremental pass under traffic,
//     because the settled side was summed to the present. The settled side is
//     therefore summed with the same created_at < watermark cut. It is still a
//     plain SUM (not an incremental delta), so a ledger row that disappears is
//     visible on the next pass; only rows created after the watermark wait for
//     the next pass, which is exactly how the evidence side behaves.
//
//   - The audit side — the table that actually hurts — is incremental. The
//     baseline stores the evidence total SUM(usage_logs) + prunedUnits together
//     with a watermark that only moves forward, and each pass only adds the rows
//     created inside [watermark, cut):
//
//     lag = settledBefore(cut) - (evidenceBaseline + windowDelta)
//
//     then it advances the watermark to cut. Rows written with an older
//     created_at than the current window (for example the ledger-gap repair
//     reusing the audit row's timestamp) are picked up by the next full
//     recalibration.
//
//   - A full recalibration runs when there is no baseline, when the watermark is
//     older than 24h, when the clock moved backwards, or when the pruned counter
//     regressed (the settings row was edited or restored). Without it a
//     months-old baseline, or a database restored from a dump, could drift
//     silently forever. The calibration uses the same cut as an incremental
//     pass, so it does not count charges whose audit rows are still in the
//     pipeline; those are counted by the following pass from the same watermark.
//
// Archived ledger rows do not move this gauge: the ledger retention worker
// retires rows whose audit row is already gone and moves their units into
// billing_ledger_archives, which is part of the settled total and is summed
// without a time cut (an archive row is created at retirement time, but the
// units it carries are older than any watermark). That is also why the baseline
// needs no manual adjustment when rows are deleted — the incremental window and
// the recalibration both see the same totals, so the incremental and the full
// result agree (asserted by billing_reconcile_incremental_test.go).
func (b *BillingReconciler) auditLag() (int64, error) {
	pruned := b.repo.UsageAuditPrunedUnits()
	telemetry.GlobalMetrics.BillingAuditPrunedUnits.Store(pruned)

	now := time.Now().UTC()
	baseline := b.repo.auditLagBaseline()
	// One cut for both sides; see the comment above for why the settled side has
	// to honour the same trailing window as the evidence side.
	cut := now.Add(-billingAuditLagOverlapWindow)
	evidence, watermark, err := b.auditEvidenceUnits(baseline, pruned, cut)
	if err != nil {
		return 0, fmt.Errorf("audit lag: audit evidence: %w", err)
	}
	settled, err := b.settledTotalUnitsBefore(watermark)
	if err != nil {
		return 0, fmt.Errorf("audit lag: settled total: %w", err)
	}
	lag := settled - evidence
	if err := b.repo.storeAuditLagBaseline(lag, watermark, pruned, evidence); err != nil {
		return 0, fmt.Errorf("audit lag: store baseline: %w", err)
	}
	if lag < 0 {
		lag = -lag
	}
	return lag, nil
}

// settledTotalUnitsBefore is SUM(billing_settlements.cost_units) for rows created
// before the watermark, plus the units of the ledger rows the retention worker
// retired.
//
// The archive total is deliberately not time-cut: an archive row is created when
// the old settlement is retired, but the units it carries were settled long
// before, so filtering it by its own created_at would make the ledger look
// smaller than the audit trail right after a retirement pass.
func (b *BillingReconciler) settledTotalUnitsBefore(cut time.Time) (int64, error) {
	var total int64
	query := `SELECT COALESCE((SELECT SUM(cost_units) FROM billing_settlements WHERE created_at < ?), 0)
		+ COALESCE((SELECT SUM(cost_units) FROM ` + billingLedgerArchivesTable + `), 0)`
	if err := b.repo.db.QueryRow(query, b.repo.dialectTimePrecise(cut)).Scan(&total); err != nil {
		return 0, err
	}
	return total, nil
}

// auditEvidenceUnits returns the cumulative charge units the audit trail proves up
// to cut, together with the watermark the next pass may start from. Pruned rows
// stay counted: their units moved to the pruned baseline when retention deleted
// them.
func (b *BillingReconciler) auditEvidenceUnits(baseline auditLagBaseline, pruned int64, cut time.Time) (evidence int64, watermark time.Time, err error) {
	if baseline.needsRecalibration(pruned, cut) {
		var auditTotal int64
		if err := b.repo.db.QueryRow(`SELECT COALESCE(SUM(cost_units), 0) FROM usage_logs WHERE created_at < ?`,
			b.repo.dialectTimePrecise(cut)).Scan(&auditTotal); err != nil {
			return 0, time.Time{}, err
		}
		return auditTotal + pruned, cut, nil
	}

	if cut.Before(baseline.at) {
		// Two passes closer together than the overlap. Keep the watermark where it
		// is: moving it backwards would re-count rows the previous pass already
		// included.
		return baseline.evidenceUnits, baseline.at, nil
	}
	var delta int64
	if err := b.repo.db.QueryRow(
		`SELECT COALESCE(SUM(cost_units), 0) FROM usage_logs WHERE created_at >= ? AND created_at < ?`,
		b.repo.dialectTimePrecise(baseline.at), b.repo.dialectTimePrecise(cut),
	).Scan(&delta); err != nil {
		return 0, time.Time{}, err
	}
	return baseline.evidenceUnits + delta, cut, nil
}

// auditLagBaseline is the persisted state of the rolling audit-lag computation.
type auditLagBaseline struct {
	// lagUnits is the last confirmed lag. It is kept for operators (the settings
	// row is readable from the console) and for the change log; the incremental
	// math below does not need it.
	lagUnits int64
	// prunedUnits and evidenceUnits are the snapshots the watermark refers to.
	prunedUnits   int64
	evidenceUnits int64
	// at is the watermark: every audit row created before it is already included
	// in evidenceUnits.
	at     time.Time
	loaded bool
}

// needsRecalibration reports whether the incremental path cannot be trusted and
// the full-table SUM has to run.
func (bl auditLagBaseline) needsRecalibration(pruned int64, now time.Time) bool {
	if !bl.loaded {
		return true
	}
	if bl.at.After(now) {
		// The clock moved backwards (NTP step, restore from a snapshot): the
		// window would be nonsense.
		return true
	}
	if now.Sub(bl.at) > billingAuditLagRecalibrateAfter {
		return true
	}
	// A regressed pruned counter means system_settings was edited or restored;
	// the baseline no longer describes this database.
	return pruned < bl.prunedUnits
}

// auditLagBaseline reads the persisted baseline. Any missing or unparsable part
// is treated as "no baseline", which forces a recalibration instead of silently
// computing from garbage.
func (r *Repository) auditLagBaseline() auditLagBaseline {
	rawAt := strings.TrimSpace(r.GetSetting(billingAuditLagBaselineAtSetting, ""))
	if rawAt == "" {
		return auditLagBaseline{}
	}
	at, err := time.Parse(sqliteTimestampLayout, rawAt)
	if err != nil {
		return auditLagBaseline{}
	}
	lag, okLag := settingInt64(r, billingAuditLagBaselineUnitsSetting)
	pruned, okPruned := settingInt64(r, billingAuditLagBaselinePrunedSetting)
	evidence, okEvidence := settingInt64(r, billingAuditLagBaselineEvidenceSetting)
	if !okLag || !okPruned || !okEvidence {
		return auditLagBaseline{}
	}
	return auditLagBaseline{
		lagUnits:      lag,
		prunedUnits:   pruned,
		evidenceUnits: evidence,
		at:            at.UTC(),
		loaded:        true,
	}
}

// storeAuditLagBaseline writes the four baseline values in one transaction so a
// crash cannot leave a half-updated watermark that the next pass would trust.
func (r *Repository) storeAuditLagBaseline(lag int64, at time.Time, pruned, evidence int64) error {
	values := []struct{ key, value string }{
		{billingAuditLagBaselineUnitsSetting, strconv.FormatInt(lag, 10)},
		{billingAuditLagBaselineAtSetting, at.UTC().Format(sqliteTimestampLayout)},
		{billingAuditLagBaselinePrunedSetting, strconv.FormatInt(pruned, 10)},
		{billingAuditLagBaselineEvidenceSetting, strconv.FormatInt(evidence, 10)},
	}
	tx, err := r.db.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	for _, kv := range values {
		if _, err := tx.Exec(`INSERT INTO system_settings (key, value, updated_at) VALUES (?, ?, CURRENT_TIMESTAMP)
			ON CONFLICT(key) DO UPDATE SET value = excluded.value, updated_at = CURRENT_TIMESTAMP`, kv.key, kv.value); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// settingInt64 parses an integer setting, reporting whether it was present and
// well formed.
func settingInt64(r *Repository, key string) (int64, bool) {
	raw := strings.TrimSpace(r.GetSetting(key, ""))
	if raw == "" {
		return 0, false
	}
	value, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		return 0, false
	}
	return value, true
}
