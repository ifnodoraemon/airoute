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

// billingAuditLagWarnFloorUnits is the smallest settlement-vs-audit difference
// worth a WARN, in 1e-9 CNY units (0.01 CNY).
//
// Both sides of the gauge are cut at the same watermark, so what can still remain
// on a healthy system is flush latency: a charge settled just before the cut
// whose audit row lands just after it. That is a fraction of a second of traffic.
// One cent is comfortably above it and still below anything an operator would
// investigate; the gauge itself keeps reporting the exact number, so a small but
// persistent gap is still visible on /metrics.
const billingAuditLagWarnFloorUnits int64 = 10_000_000

// auditLagIsMaterial reports whether a lag is worth waking an operator for.
func auditLagIsMaterial(lag int64) bool {
	return lag >= billingAuditLagWarnFloorUnits
}

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
	if err := b.publishWalletTotal(); err != nil {
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
	telemetry.GlobalMetrics.BillingAuditLagUnits.Store(auditLag)
	telemetry.GlobalMetrics.BillingLedgerUnlinkedUnits.Store(unlinkedUnits)

	if ledgerDrift != 0 {
		telemetry.WarnLogger(telemetry.ScopedLogger("billing"), "billing-reconcile-drift", 6*time.Hour,
			"billing ledgers do not agree",
			"ledger_drift_cny", FromUnits(ledgerDrift),
			"keys_with_drift", ledgerKeys,
		)
	}
	if auditLagIsMaterial(auditLag) {
		telemetry.WarnLogger(telemetry.ScopedLogger("billing"), "billing-audit-lag", 6*time.Hour,
			"settlement ledger and audit trail disagree",
			"audit_lag_cny", FromUnits(auditLag),
			"audit_warn_floor_cny", FromUnits(billingAuditLagWarnFloorUnits),
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
