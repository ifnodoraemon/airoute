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

	telemetry.GlobalMetrics.BillingLedgerDriftUnits.Store(ledgerDrift)
	telemetry.GlobalMetrics.BillingWalletDriftUnits.Store(walletDrift)

	if ledgerDrift != 0 || walletDrift != 0 {
		telemetry.WarnLogger(telemetry.ScopedLogger("billing"), "billing-reconcile-drift", 6*time.Hour,
			"billing ledgers do not agree",
			"ledger_drift_cny", FromUnits(ledgerDrift),
			"wallet_drift_cny", FromUnits(walletDrift),
			"keys_with_drift", ledgerKeys,
		)
	}
}

// ledgerDrift is the absolute difference, summed over keys, between the key's
// used_cost_units and the cost recorded in usage_logs.
func (b *BillingReconciler) ledgerDrift() (int64, int, error) {
	drifts, err := b.repo.ReconcileMoney(billingReconcileDriftLimit)
	if err != nil {
		return 0, 0, err
	}
	var total int64
	for _, drift := range drifts {
		if drift.Difference < 0 {
			total -= drift.Difference
		} else {
			total += drift.Difference
		}
	}
	return total, len(drifts), nil
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
