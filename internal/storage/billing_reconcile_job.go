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
	billingReconcileDriftLimit   = 500
)

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

	telemetry.GlobalMetrics.BillingLedgerDriftUnits.Store(ledgerDrift)
	telemetry.GlobalMetrics.BillingWalletDriftUnits.Store(walletDrift)
	telemetry.GlobalMetrics.BillingAuditLagUnits.Store(auditLag)

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
		)
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
func (b *BillingReconciler) repairSettlementGap() (int, error) {
	cutoff := time.Now().Add(-billingSettlementRepairGrace).UTC().Format(sqliteTimestampLayout)
	query := b.repo.db.Rebind(`INSERT INTO billing_settlements (billing_id, api_key, tenant_id, cost_units, total_tokens, created_at)
		SELECT l.billing_id, COALESCE(l.api_key, ''), COALESCE(l.tenant_id, ''), l.cost_units, l.total_tokens, l.created_at
		FROM usage_logs l
		WHERE l.billing_id IS NOT NULL AND l.billing_id <> ''
		  AND (l.cost_units <> 0 OR l.total_tokens <> 0)
		  AND l.created_at < ?
		  AND NOT EXISTS (SELECT 1 FROM billing_settlements s WHERE s.billing_id = l.billing_id)
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
func (b *BillingReconciler) ledgerDrift() (int64, int, error) {
	query := `SELECT COUNT(*), COALESCE(SUM(CASE WHEN COALESCE(k.used_cost_units, 0) >= COALESCE(s.settled, 0)
			THEN COALESCE(k.used_cost_units, 0) - COALESCE(s.settled, 0)
			ELSE COALESCE(s.settled, 0) - COALESCE(k.used_cost_units, 0) END), 0)
		FROM api_keys k
		LEFT JOIN (SELECT api_key, SUM(cost_units) AS settled FROM billing_settlements GROUP BY api_key) s ON s.api_key = k.key
		WHERE COALESCE(k.used_cost_units, 0) <> COALESCE(s.settled, 0)`
	var keys int
	var total int64
	if err := b.repo.db.QueryRow(query).Scan(&keys, &total); err != nil {
		return 0, 0, fmt.Errorf("ledger drift query: %w", err)
	}
	return total, keys, nil
}

// auditLag is the absolute difference between the total settled and the total
// recorded in the audit trail.
func (b *BillingReconciler) auditLag() (int64, error) {
	query := `SELECT COALESCE((SELECT SUM(cost_units) FROM billing_settlements), 0)
		- COALESCE((SELECT SUM(cost_units) FROM usage_logs), 0)`
	var lag int64
	if err := b.repo.db.QueryRow(query).Scan(&lag); err != nil {
		return 0, fmt.Errorf("audit lag query: %w", err)
	}
	if lag < 0 {
		lag = -lag
	}
	return lag, nil
}

// walletMirrorDrift is the absolute difference between each user's balance_units
// (authoritative) and the rounded float mirror in balance.
func (b *BillingReconciler) walletMirrorDrift() (int64, error) {
	query := `SELECT COALESCE(SUM(ABS(COALESCE(balance_units, 0) - CAST(ROUND(COALESCE(balance, 0) * 1000000000) AS BIGINT))), 0) FROM users`
	if b.repo.Dialect() != "postgres" {
		query = `SELECT COALESCE(SUM(ABS(COALESCE(balance_units, 0) - CAST(ROUND(COALESCE(balance, 0) * 1000000000) AS INTEGER))), 0) FROM users`
	}
	var drift int64
	if err := b.repo.db.QueryRow(query).Scan(&drift); err != nil {
		return 0, fmt.Errorf("wallet mirror drift query: %w", err)
	}
	return drift, nil
}
