package storage

import (
	"errors"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/ifnodoraemon/airoute/internal/telemetry"
)

// The settlement ledger used to only grow: audit rows have a retention policy
// (usage_retention.go) but billing_settlements has none, so it accumulated every
// charge forever, together with its indexes. This file adds an opt-in retention
// policy for it.
//
// The ledger is a financial document, so the policy is deliberately conservative:
//
//   - It is off by default (GATEWAY_BILLING_LEDGER_RETENTION_DAYS=0 keeps every
//     row). Deleting accounting evidence is never something a deploy should do on
//     its own.
//   - A row is only retired when its audit row is already gone AND the row is
//     older than the cutoff. The audit trail is what makes a charge explainable;
//     a ledger row whose audit row still exists is never touched, no matter how
//     old it is. (In practice that means the window shrinks to
//     min(ledger retention, audit retention) rows, because audit retention is
//     what removes the evidence in the first place.)
//   - Retiring a row must not change the reconciliation numbers. The units are
//     moved into billing_ledger_archives (one aggregate row per api_key, created
//     by migration 41) in the same transaction as the DELETE, and auditLag /
//     ledgerDrift add that aggregate back. That is why the code does not adjust
//     the "billing_audit_lag_baseline_units" value directly: that key stores the
//     lag itself, so adding the retired amount to it would make the next
//     incremental pass over-report the whole archived history. Keeping the money
//     in the archive table keeps the incremental and the full recalibration in
//     agreement (asserted by billing_reconcile_incremental_test.go).
//   - Every batch records the amount in system_settings
//     (billing_ledger_archive_<yyyymm>) and the pass logs a WARN, so an auditor
//     can explain the disappearance from the database alone.
//
// Batch size is 5000 rows per transaction: a ledger can be tens of millions of
// rows, and one giant DELETE would hold locks (and, on SQLite, the single writer)
// for the whole run.
//
// Why not DDL partitioning, which the audit suggested for this table? Switching
// billing_settlements to a partitioned table is a table rewrite that needs a
// maintenance window, and it changes the primary key: `id` alone stops being
// unique across partitions and the unique(billing_id) index that the settlement
// hot path uses as its exactly-once gate would have to include the partition key.
// That is a scheduled-window operation, not something a startup migration or a
// background worker may do. This file retires rows online instead; a partitioned
// layout can be adopted later with a shadow table + backfill, and this policy
// would still apply to the new table.
const (
	// Opt-in: 0 keeps every row.
	defaultBillingLedgerRetentionDays = 0
	defaultBillingLedgerPruneHours    = 24
	billingLedgerPruneBatchSize       = 5000
	// One pass never does more than this many batches, so a huge backlog cannot
	// hold the maintenance slot (or the WAL) for an unbounded time. The remainder
	// is picked up by the next pass.
	billingLedgerPruneMaxBatches = 200
	// Migrations, the reconcile pass (3m) and the audit prune worker (5m) start
	// first; this one is the least urgent.
	billingLedgerPruneStartupDelay = 15 * time.Minute
	// Monthly audit trail of retired amounts, e.g. billing_ledger_archive_202610.
	billingLedgerArchiveKeyPrefix = "billing_ledger_archive_"
	// migration 23 gave audit rows that predate billing_id the deterministic key
	// "legacy-<usage_logs.id>"; a ledger row with that shape links back to the
	// usage row's primary key.
	legacyBillingIDPrefix = "legacy-"
)

// ResolveBillingLedgerRetention reads the ledger retention window and prune
// cadence from the environment.
// GATEWAY_BILLING_LEDGER_RETENTION_DAYS=0 (the default) keeps every ledger row.
func ResolveBillingLedgerRetention() (retention time.Duration, interval time.Duration) {
	days := defaultBillingLedgerRetentionDays
	if raw := strings.TrimSpace(os.Getenv("GATEWAY_BILLING_LEDGER_RETENTION_DAYS")); raw != "" {
		if value, err := strconv.Atoi(raw); err == nil && value >= 0 {
			days = value
		}
	}
	hours := defaultBillingLedgerPruneHours
	if raw := strings.TrimSpace(os.Getenv("GATEWAY_BILLING_LEDGER_PRUNE_INTERVAL_HOURS")); raw != "" {
		if value, err := strconv.Atoi(raw); err == nil && value > 0 {
			hours = value
		}
	}
	return time.Duration(days) * 24 * time.Hour, time.Duration(hours) * time.Hour
}

// BillingLedgerRetention retires ledger rows whose audit evidence is already
// gone and that fall outside the configured window.
type BillingLedgerRetention struct {
	repo      *Repository
	retention time.Duration
	interval  time.Duration
	stop      chan struct{}
	stopOnce  sync.Once
	wg        sync.WaitGroup
}

// InitBillingLedgerRetention starts the ledger retention worker. A zero window
// (the default) or a nil repository disables it, so no deployment deletes
// financial evidence unless it opts in.
func InitBillingLedgerRetention(repo *Repository) *BillingLedgerRetention {
	retention, interval := ResolveBillingLedgerRetention()
	worker := &BillingLedgerRetention{
		repo:      repo,
		retention: retention,
		interval:  interval,
		stop:      make(chan struct{}),
	}
	if repo == nil || retention <= 0 {
		return worker
	}
	if retention < 30*24*time.Hour {
		// Not an error: a window shorter than the audit retention simply means
		// almost nothing is ever deletable, because the audit row still exists.
		telemetry.ScopedLogger("billing").Warn(
			"billing ledger retention is very short; rows with a surviving audit row are never retired",
			"retention_days", int(retention.Hours()/24),
		)
	}
	worker.wg.Add(1)
	go worker.worker()
	return worker
}

// Stop waits for the worker to exit.
func (w *BillingLedgerRetention) Stop() {
	if w == nil {
		return
	}
	w.stopOnce.Do(func() { close(w.stop) })
	w.wg.Wait()
}

func (w *BillingLedgerRetention) worker() {
	defer w.wg.Done()
	ticker := time.NewTicker(w.interval)
	defer ticker.Stop()

	select {
	case <-w.stop:
		return
	case <-time.After(billingLedgerPruneStartupDelay):
	}
	for {
		if _, _, err := w.pruneOnce(); err != nil {
			telemetry.WarnLogger(telemetry.ScopedLogger("billing"), "billing-ledger-prune-failed", 10*time.Minute,
				"ledger retention pass failed", "error", err.Error())
		}
		select {
		case <-w.stop:
			return
		case <-ticker.C:
		}
	}
}

// pruneOnce runs one retention pass. It returns the number of retired ledger
// rows and the charge units moved into the archive aggregate.
func (w *BillingLedgerRetention) pruneOnce() (int64, int64, error) {
	if w == nil || w.repo == nil || w.repo.db == nil || w.retention <= 0 {
		return 0, 0, nil
	}
	cutoff := time.Now().UTC().Add(-w.retention)

	var deleted, archivedUnits int64
	for batch := 0; batch < billingLedgerPruneMaxBatches; batch++ {
		batchRows, batchUnits, err := w.pruneBatchWithRetry(cutoff)
		if err != nil {
			return deleted, archivedUnits, err
		}
		deleted += batchRows
		archivedUnits += batchUnits
		if batchRows < billingLedgerPruneBatchSize {
			break
		}
	}

	if deleted > 0 {
		telemetry.ScopedLogger("billing").Warn(
			"retired expired billing ledger rows into the archive aggregate",
			"rows", deleted,
			"archived_units", archivedUnits,
			"archived_cny", FromUnits(archivedUnits),
			"retention_days", int(w.retention.Hours()/24),
			"cutoff", cutoff.UTC().Format(time.RFC3339),
			"archive_keys", billingLedgerArchiveKeyPrefix+"<yyyymm>",
		)
	}
	return deleted, archivedUnits, nil
}

// errBillingLedgerPruneContended marks a batch that had to roll back because
// another replica retired (or the ledger changed under us) some of the scanned
// rows. It must never be reported as "nothing to do": the earlier code returned
// (0, 0, nil), which made pruneOnce stop the whole pass silently and made a
// replica collision invisible in the logs.
var errBillingLedgerPruneContended = errors.New("billing ledger prune contended")

// ledgerPruneDeletionMismatch classifies a DELETE whose row count disagrees with
// the scanned candidate count.
func ledgerPruneDeletionMismatch(deleted, want int64) error {
	if deleted == want {
		return nil
	}
	return errBillingLedgerPruneContended
}
