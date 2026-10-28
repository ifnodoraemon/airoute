package storage

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/ifnodoraemon/airoute/internal/telemetry"
)

const (
	defaultBillingReconcileHours = 6
	// A rolling deploy leaves a window where the previous binary still writes
	// audit rows without settlement rows (it predates the ledger). Those gaps are
	// repaired by inserting the missing ledger rows, a few hundred per pass.
	billingSettlementRepairBatch = 500
	billingSettlementRepairGrace = 5 * time.Minute
	// Do not compete with migrations and the boot-time hot reload.
	billingReconcileStartupDelay = 3 * time.Minute

	// auditLag keeps a rolling baseline in system_settings instead of running two
	// full-table SUMs on every pass. See auditLag for the algorithm.
	billingAuditLagBaselineUnitsSetting    = "billing_audit_lag_baseline_units"
	billingAuditLagBaselineAtSetting       = "billing_audit_lag_baseline_at"
	billingAuditLagBaselinePrunedSetting   = "billing_audit_lag_baseline_pruned_units"
	billingAuditLagBaselineEvidenceSetting = "billing_audit_lag_baseline_evidence_units"
	// A baseline older than this is recalibrated against the whole table. It
	// bounds how long a late-arriving audit row can stay uncounted.
	billingAuditLagRecalibrateAfter = 24 * time.Hour
)

// billingAuditLagOverlapWindow keeps the incremental watermark a few minutes
// behind the wall clock so an audit row that lands just after its charge (async
// flush, Redis redelivery) is still inside the next window instead of being
// skipped until the daily recalibration.
//
// It is a variable rather than a constant only so the incremental tests can drive
// the watermark without sleeping for five minutes; production never changes it.
var billingAuditLagOverlapWindow = 5 * time.Minute

// ResolveBillingReconcileInterval reads the reconciliation cadence.
// GATEWAY_BILLING_RECONCILE_HOURS=0 disables the worker.
func ResolveBillingReconcileInterval() time.Duration {
	hours := defaultBillingReconcileHours
	if raw := strings.TrimSpace(os.Getenv("GATEWAY_BILLING_RECONCILE_HOURS")); raw != "" {
		if value, err := strconv.Atoi(raw); err == nil && value >= 0 {
			hours = value
		}
	}
	return time.Duration(hours) * time.Hour
}

// BillingReconciler periodically compares the money mirrors and publishes the
// drift on /metrics.
//
// The ledger is written in the same transaction as the wallet update, so any
// drift means a write path was wrong, a legacy row predates the integer mirrors,
// or something modified balances outside the billing path. None of that is
// visible in a dashboard today, which is why the audit called reconciliation
// "impossible": the numbers existed, but nobody was comparing them.
type BillingReconciler struct {
	repo     *Repository
	interval time.Duration
	stop     chan struct{}
	stopOnce sync.Once
	wg       sync.WaitGroup
}

// InitBillingReconcile starts the reconciliation worker. A zero interval or a nil
// repository disables it.
func InitBillingReconcile(repo *Repository) *BillingReconciler {
	worker := &BillingReconciler{
		repo:     repo,
		interval: ResolveBillingReconcileInterval(),
		stop:     make(chan struct{}),
	}
	if repo == nil || worker.interval <= 0 {
		return worker
	}
	worker.wg.Add(1)
	go worker.worker()
	return worker
}

// Stop waits for the worker to exit.
func (b *BillingReconciler) Stop() {
	if b == nil {
		return
	}
	b.stopOnce.Do(func() { close(b.stop) })
	b.wg.Wait()
}

func (b *BillingReconciler) worker() {
	defer b.wg.Done()

	select {
	case <-b.stop:
		return
	case <-time.After(billingReconcileStartupDelay):
	}

	ticker := time.NewTicker(b.interval)
	defer ticker.Stop()
	for {
		b.runOnce()
		select {
		case <-b.stop:
			return
		case <-ticker.C:
		}
	}
}

// runOnce recomputes both drift gauges. It never mutates balances: reconciliation
// reports, a human decides.
func (b *BillingReconciler) runOnce() {
	if repaired, err := b.repairSettlementGap(); err != nil {
		telemetry.ScopedLogger("billing").Warn("settlement gap repair failed", "error", err.Error())
	} else if repaired > 0 {
		telemetry.ScopedLogger("billing").Info("repaired settlement ledger gaps", "records", repaired)
	}

	ledgerDrift, ledgerKeys, err := b.ledgerDrift()
	if err != nil {
		telemetry.ScopedLogger("billing").Error("billing reconciliation failed", "stage", "ledger", "error", err.Error())
		return
	}
	walletDrift, err := b.walletMirrorDrift()
	if err != nil {
		telemetry.ScopedLogger("billing").Error("billing reconciliation failed", "stage", "wallet", "error", err.Error())
		return
	}
	auditLag, err := b.auditLag()
	if err != nil {
		telemetry.ScopedLogger("billing").Error("billing reconciliation failed", "stage", "audit", "error", err.Error())
		return
	}

	unlinkedUnits, unlinkedRows, err := b.ledgerLinkage()
	if err != nil {
		telemetry.ScopedLogger("billing").Error("billing reconciliation failed", "stage", "linkage", "error", err.Error())
		return
	}

	tenantDriftUnits, tenantDriftCount, tenantRows, err := b.tenantLedgerDrift()
	if err != nil {
		telemetry.ScopedLogger("billing").Error("billing reconciliation failed", "stage", "tenant", "error", err.Error())
		return
	}

	telemetry.GlobalMetrics.BillingLedgerDriftUnits.Store(ledgerDrift)
	telemetry.GlobalMetrics.BillingWalletDriftUnits.Store(walletDrift)
	telemetry.GlobalMetrics.BillingAuditLagUnits.Store(auditLag)
	telemetry.GlobalMetrics.BillingLedgerUnlinkedUnits.Store(unlinkedUnits)

	if ledgerDrift != 0 || walletDrift != 0 {
		telemetry.WarnLogger(telemetry.ScopedLogger("billing"), "billing-reconcile-drift", 6*time.Hour,
			"billing ledgers do not agree",
			"ledger_drift_cny", FromUnits(ledgerDrift),
			"wallet_drift_cny", FromUnits(walletDrift),
			"keys_with_drift", ledgerKeys,
		)
	}
	if auditLag != 0 {
		telemetry.WarnLogger(telemetry.ScopedLogger("billing"), "billing-audit-lag", 6*time.Hour,
			"settlement ledger and audit trail disagree",
			"audit_lag_cny", FromUnits(auditLag),
			"audit_pruned_cny", FromUnits(b.repo.UsageAuditPrunedUnits()),
		)
	}
	if unlinkedUnits != 0 {
		telemetry.WarnLogger(telemetry.ScopedLogger("billing"), "billing-ledger-unlinked", 6*time.Hour,
			"settled charges have no matching audit row",
			"unlinked_cny", FromUnits(unlinkedUnits),
			"unlinked_records", unlinkedRows,
		)
	}
	if tenantDriftUnits != 0 {
		telemetry.WarnLogger(telemetry.ScopedLogger("billing"), "billing-tenant-attribution-drift", 6*time.Hour,
			"settlement ledger and audit trail disagree per tenant inside the reconciliation window",
			"tenant_drift_cny", FromUnits(tenantDriftUnits),
			"tenants_with_drift", tenantDriftCount,
			"window_days", int(tenantLedgerWindow.Hours()/24),
		)
	}
	// Per-tenant settled totals: this is the read side of the ledger's tenant_id
	// and total_tokens columns (the wallet ledger UI that used them was cut in
	// v1.1), so operators can still attribute settled spend and tokens.
	if len(tenantRows) > 0 {
		args := make([]any, 0, 2*tenantLedgerTopN)
		for i := 0; i < len(tenantRows) && i < tenantLedgerTopN; i++ {
			args = append(args, tenantRows[i].Tenant, FromUnits(tenantRows[i].SettledUnits), tenantRows[i].SettledTokens)
		}
		telemetry.ScopedLogger("billing").Info("tenant settlement ledger summary",
			append([]any{"window_days", int(tenantLedgerWindow.Hours() / 24), "tenants", len(tenantRows)}, args...)...)
	}
}

// repairSettlementGap inserts settlement rows for audit rows that have none.
//
// A rolling deploy is the normal source: the previous binary writes usage rows
// (and moves wallets) but knows nothing about the settlement ledger, so those
// charges appear as "audit without settlement". Re-recording them is safe and
// idempotent: the ledger row only states that the charge was already applied,
// and the wallet was moved by the process that wrote the audit row. Rows newer
// than the grace window are skipped so in-flight writes are not raced.
// usageLogBillingIDExpr is the settlement-ledger key of an audit row.
//
// Rows written before billing_id existed were recorded in the ledger under the
// deterministic key "legacy-<id>" (migration 23 backfilled them that way) while
// usage_logs.billing_id stayed NULL. Anything that joins the two tables has to
// use this expression instead of the raw column, otherwise every pre-billing_id
// row looks unlinkable and can never be reconciled.
const usageLogBillingIDExpr = `COALESCE(NULLIF(l.billing_id, ''), 'legacy-' || l.id)`

func (b *BillingReconciler) repairSettlementGap() (int, error) {
	cutoff := b.repo.dialectTimePrecise(time.Now().Add(-billingSettlementRepairGrace))
	query := b.repo.db.Rebind(`INSERT INTO billing_settlements (billing_id, api_key, tenant_id, cost_units, total_tokens, created_at)
		SELECT ` + usageLogBillingIDExpr + `, COALESCE(l.api_key, ''), COALESCE(l.tenant_id, ''), l.cost_units, l.total_tokens, l.created_at
		FROM usage_logs l
		WHERE (l.cost_units <> 0 OR l.total_tokens <> 0)
		  AND l.created_at < ?
		  AND NOT EXISTS (SELECT 1 FROM billing_settlements s WHERE s.billing_id = ` + usageLogBillingIDExpr + `)
		LIMIT ?
		ON CONFLICT (billing_id) DO NOTHING`)
	res, err := b.repo.db.Exec(query, cutoff, billingSettlementRepairBatch)
	if err != nil {
		return 0, err
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return 0, nil
	}
	return int(affected), nil
}

// ledgerDrift is the absolute difference, summed over keys, between the key's
// used_cost_units and the charges actually settled for that key.
//
// It compares against the settlement ledger rather than usage_logs because the
// two are now written separately: audit rows may lag a settlement by a retry,
// which is a pipeline symptom, not a money error. The settlement-vs-audit gap is
// reported separately by auditLag.
//
// A per-key comparison of api_keys.used_cost_units against SUM(usage_logs) used to
// exist as Repository.ReconcileMoney and was only ever called from tests. It has
// been removed rather than wired in: usage_logs is pruned at the retention window
// and only a global pruned-units baseline is kept, so after the first prune every
// long-lived key would look like drift. This ledger comparison is the prune-aware
// equivalent (billing_ledger_retention subtracts archived rows).
//
// Retired ledger rows are part of the comparison: the ledger retention worker
// moves their units into billing_ledger_archives (one row per key) instead of
// dropping the amount, so archiving cannot make every long-lived key look
// drifted. The per-key settlement aggregate is what the new
// (api_key, cost_units) index exists for: the GROUP BY becomes an index-only
// scan in key order instead of a full-table scan plus sort.
func (b *BillingReconciler) ledgerDrift() (int64, int, error) {
	query := `SELECT COUNT(*), COALESCE(SUM(CASE WHEN COALESCE(k.used_cost_units, 0) >= COALESCE(s.settled, 0) + COALESCE(a.cost_units, 0)
			THEN COALESCE(k.used_cost_units, 0) - (COALESCE(s.settled, 0) + COALESCE(a.cost_units, 0))
			ELSE (COALESCE(s.settled, 0) + COALESCE(a.cost_units, 0)) - COALESCE(k.used_cost_units, 0) END), 0)
		FROM api_keys k
		LEFT JOIN (SELECT api_key, SUM(cost_units) AS settled FROM billing_settlements GROUP BY api_key) s ON s.api_key = k.key
		LEFT JOIN ` + billingLedgerArchivesTable + ` a ON a.api_key = k.key
		WHERE COALESCE(k.used_cost_units, 0) <> COALESCE(s.settled, 0) + COALESCE(a.cost_units, 0)`
	var keys int
	var total int64
	if err := b.repo.db.QueryRow(query).Scan(&keys, &total); err != nil {
		return 0, 0, fmt.Errorf("ledger drift query: %w", err)
	}
	return total, keys, nil
}

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
//   - The settled side is always summed exactly. billing_settlements is the
//     money-bearing ledger: a row disappearing without the archive moving has to
//     be visible on the next pass (see TestAuditLagSurvivesRetentionPruning),
//     which an incremental window by created_at cannot see. The new
//     (api_key, cost_units) index makes that SUM an index-only scan, and the
//     table is far smaller than usage_logs.
//
//   - The audit side — the table that actually hurts — is incremental. The
//     baseline stores the evidence total SUM(usage_logs) + prunedUnits together
//     with a watermark that only moves forward, and each pass only adds the rows
//     created inside [watermark, now-5min):
//
//     lag = (settled + archived) - (evidenceBaseline + windowDelta)
//
//     then it advances the watermark. The window has both a lower and an upper
//     bound on purpose: "created_at >= watermark" alone would re-count the last
//     few minutes on every pass, so the reported lag would grow by the amount in
//     that overlap each run instead of staying at the real difference. The
//     trailing 5-minute watermark is what tolerates audit rows that land slightly
//     after their charge (async flush, Redis redelivery) without double counting.
//     Rows written with an older created_at than the current window (for example
//     the ledger-gap repair reusing the audit row's timestamp) are picked up by
//     the next full recalibration.
//
//   - A full recalibration runs when there is no baseline, when the watermark is
//     older than 24h, when the clock moved backwards, or when the pruned counter
//     regressed (the settings row was edited or restored). Without it a
//     months-old baseline, or a database restored from a dump, could drift
//     silently forever. After a calibration the watermark is the run time, not
//     now-5min: the calibration already counted the trailing overlap window, and
//     counting it again would be exactly the double count described above.
//
// Archived ledger rows do not move this gauge: the ledger retention worker
// retires rows whose audit row is already gone and moves their units into
// billing_ledger_archives, which is part of the settled total. That is also why
// the baseline needs no manual adjustment when rows are deleted — the
// incremental window and the recalibration both see the same totals, so the
// incremental and the full result agree (asserted by
// billing_reconcile_incremental_test.go).
func (b *BillingReconciler) auditLag() (int64, error) {
	pruned := b.repo.UsageAuditPrunedUnits()
	telemetry.GlobalMetrics.BillingAuditPrunedUnits.Store(pruned)

	now := time.Now().UTC()
	baseline := b.repo.auditLagBaseline()
	settled, err := b.settledTotalUnits()
	if err != nil {
		return 0, fmt.Errorf("audit lag: settled total: %w", err)
	}
	evidence, watermark, err := b.auditEvidenceUnits(baseline, pruned, now)
	if err != nil {
		return 0, fmt.Errorf("audit lag: audit evidence: %w", err)
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

// settledTotalUnits is SUM(billing_settlements.cost_units) plus the units of the
// ledger rows the retention worker retired.
func (b *BillingReconciler) settledTotalUnits() (int64, error) {
	var total int64
	query := `SELECT COALESCE((SELECT SUM(cost_units) FROM billing_settlements), 0)
		+ COALESCE((SELECT SUM(cost_units) FROM ` + billingLedgerArchivesTable + `), 0)`
	if err := b.repo.db.QueryRow(query).Scan(&total); err != nil {
		return 0, err
	}
	return total, nil
}

// auditEvidenceUnits returns the cumulative charge units the audit trail proves,
// together with the watermark the next pass may start from. Pruned rows stay
// counted: their units moved to the pruned baseline when retention deleted them.
func (b *BillingReconciler) auditEvidenceUnits(baseline auditLagBaseline, pruned int64, now time.Time) (evidence int64, watermark time.Time, err error) {
	if baseline.needsRecalibration(pruned, now) {
		var auditTotal int64
		if err := b.repo.db.QueryRow(`SELECT COALESCE(SUM(cost_units), 0) FROM usage_logs`).Scan(&auditTotal); err != nil {
			return 0, time.Time{}, err
		}
		// The calibration counted every row that exists now, including the trailing
		// overlap window, so the watermark becomes the run time.
		return auditTotal + pruned, now, nil
	}

	windowEnd := now.Add(-billingAuditLagOverlapWindow)
	if windowEnd.Before(baseline.at) {
		// Two passes closer together than the overlap. Keep the watermark where it
		// is: moving it backwards would re-count rows the previous pass already
		// included.
		return baseline.evidenceUnits, baseline.at, nil
	}
	var delta int64
	if err := b.repo.db.QueryRow(
		`SELECT COALESCE(SUM(cost_units), 0) FROM usage_logs WHERE created_at >= ? AND created_at < ?`,
		b.repo.dialectTimePrecise(baseline.at), b.repo.dialectTimePrecise(windowEnd),
	).Scan(&delta); err != nil {
		return 0, time.Time{}, err
	}
	return baseline.evidenceUnits + delta, windowEnd, nil
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

// ledgerLinkage reports settled units that no audit row can be matched to.
//
// The join uses usageLogBillingIDExpr, so the pre-billing_id rows recorded as
// "legacy-<id>" link correctly instead of looking orphaned. Only rows inside the
// retention window are counted: older rows are expected to have been pruned by
// now, and flagging them would make the gauge unusable. A non-zero value means a
// charge reached the ledger while its audit row never arrived (or was deleted
// outside the guarded endpoints).
//
// Query shape: the created_at bound is not only the retention filter, it is what
// keeps this query proportional to the retention window instead of the whole
// ledger (idx_billing_settlements_created). Ledger rows retired by the retention
// worker always fall outside that window — a row only becomes retirable once its
// audit evidence is gone — so the archive aggregate does not have to be counted
// here.
func (b *BillingReconciler) ledgerLinkage() (units int64, rows int64, err error) {
	retention, _ := ResolveUsageLogRetention()
	cutoff := time.Time{}
	if retention > 0 {
		cutoff = time.Now().Add(-retention)
	}
	cutoffValue := b.repo.dialectTimePrecise(cutoff)
	query := b.repo.db.Rebind(`SELECT COUNT(*), COALESCE(SUM(s.cost_units), 0)
		FROM billing_settlements s
		WHERE s.created_at >= ?
		  AND NOT EXISTS (
			SELECT 1 FROM usage_logs l WHERE ` + usageLogBillingIDExpr + ` = s.billing_id
		  )`)
	if err := b.repo.db.QueryRow(query, cutoffValue).Scan(&rows, &units); err != nil {
		return 0, 0, fmt.Errorf("ledger linkage query: %w", err)
	}
	return units, rows, nil
}

// walletMirrorDrift is the absolute difference between each user's balance_units
// (authoritative) and the rounded float mirror in balance.
func (b *BillingReconciler) walletMirrorDrift() (int64, error) {
	query := `SELECT COALESCE(SUM(ABS(COALESCE(balance_units, 0) - CAST(ROUND(COALESCE(balance, 0) * 1000000000) AS BIGINT))), 0), COALESCE(SUM(COALESCE(balance_units, 0)), 0) FROM users`
	if b.repo.Dialect() != "postgres" {
		query = `SELECT COALESCE(SUM(ABS(COALESCE(balance_units, 0) - CAST(ROUND(COALESCE(balance, 0) * 1000000000) AS INTEGER))), 0), COALESCE(SUM(COALESCE(balance_units, 0)), 0) FROM users`
	}
	var drift, total int64
	if err := b.repo.db.QueryRow(query).Scan(&drift, &total); err != nil {
		return 0, fmt.Errorf("wallet mirror drift query: %w", err)
	}
	// Publish the wallet total in the same pass: the reserved-units gauge is only
	// interpretable against the money actually held, and this scan already visits
	// every row.
	telemetry.GlobalMetrics.BillingWalletTotalUnits.Store(total)
	return drift, nil
}

// tenantLedgerWindow bounds the per-tenant settlement-vs-audit comparison.
//
// It has to be shorter than the audit retention window (365 days by default) so
// that pruning an expired usage row can never masquerade as tenant drift, and it
// ends before billingSettlementRepairGrace so a row still travelling through the
// async pipeline is not counted as missing.
const (
	tenantLedgerWindow = 7 * 24 * time.Hour
	tenantLedgerTopN   = 5
)

// tenantLedgerTotals is one tenant's settled and audited totals inside the
// reconciliation window.
type tenantLedgerTotals struct {
	Tenant        string
	SettledUnits  int64
	SettledTokens int64
	AuditUnits    int64
}

// tenantLedgerDrift reports, per tenant, the difference between the money the
// settlement ledger recorded and the money the audit trail recorded inside the
// reconciliation window.
//
// This is the read path for billing_settlements.tenant_id and
// billing_settlements.total_tokens: both are written by settlement for
// attribution, but the wallet-ledger view that consumed them was cut in v1.1, so
// without this comparison nothing ever read them. Only the cost side is treated
// as an invariant:
//
//   - a charge rejected for currency reasons zeroes its cost but keeps its token
//     counts and deliberately writes no ledger row, so token totals legitimately
//     differ for such rows;
//   - the token figures are therefore reported (summary log) rather than compared.
func (b *BillingReconciler) tenantLedgerDrift() (driftUnits int64, driftCount int, tenants []tenantLedgerTotals, err error) {
	until := time.Now().Add(-billingSettlementRepairGrace)
	since := until.Add(-tenantLedgerWindow)
	sinceArg := b.repo.dialectTimePrecise(since)
	untilArg := b.repo.dialectTimePrecise(until)

	// A UNION ALL of the two sources, summed per tenant, is portable to both
	// engines (SQLite has no FULL OUTER JOIN) and lets a tenant that only exists
	// on one side still show up as drift.
	query := b.repo.db.Rebind(`SELECT tenant_id,
			COALESCE(SUM(settled_units), 0) AS settled_units,
			COALESCE(SUM(settled_tokens), 0) AS settled_tokens,
			COALESCE(SUM(audit_units), 0) AS audit_units
		FROM (
			SELECT COALESCE(tenant_id, '') AS tenant_id, cost_units AS settled_units,
			       total_tokens AS settled_tokens, 0 AS audit_units
			  FROM billing_settlements
			 WHERE created_at >= ? AND created_at < ?
			UNION ALL
			SELECT COALESCE(tenant_id, ''), 0, 0, cost_units
			  FROM usage_logs
			 WHERE created_at >= ? AND created_at < ?
		) per_tenant
		GROUP BY tenant_id
		ORDER BY settled_units DESC, tenant_id ASC`)
	rowsRead, err := b.repo.db.Query(query, sinceArg, untilArg, sinceArg, untilArg)
	if err != nil {
		return 0, 0, nil, fmt.Errorf("tenant ledger drift query: %w", err)
	}
	defer rowsRead.Close()

	tenants = make([]tenantLedgerTotals, 0, 16)
	for rowsRead.Next() {
		var row tenantLedgerTotals
		if err := rowsRead.Scan(&row.Tenant, &row.SettledUnits, &row.SettledTokens, &row.AuditUnits); err != nil {
			return 0, 0, nil, fmt.Errorf("scan tenant ledger drift: %w", err)
		}
		if diff := row.SettledUnits - row.AuditUnits; diff != 0 {
			if diff < 0 {
				diff = -diff
			}
			driftUnits += diff
			driftCount++
		}
		tenants = append(tenants, row)
	}
	if err := rowsRead.Err(); err != nil {
		return 0, 0, nil, fmt.Errorf("iterate tenant ledger drift: %w", err)
	}
	return driftUnits, driftCount, tenants, nil
}
