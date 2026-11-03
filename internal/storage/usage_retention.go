package storage

import (
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/ifnodoraemon/airoute/internal/telemetry"
)

const (
	// A year keeps enough history for reconciliation and incident review while
	// stopping usage_logs from growing forever.
	defaultUsageLogRetentionDays = 365
	defaultUsageLogPruneHours    = 6
	usageLogPruneBatchSize       = 5000
	// Let migrations and boot-time warm-up finish before competing for the same
	// database the data plane needs immediately after start.
	usageLogPruneStartupDelay = 5 * time.Minute
)

// ResolveUsageLogRetention reads the retention window and prune interval from the
// environment. GATEWAY_USAGE_LOG_RETENTION_DAYS=0 keeps every row.
func ResolveUsageLogRetention() (retention time.Duration, interval time.Duration) {
	days := defaultUsageLogRetentionDays
	if value := strings.TrimSpace(os.Getenv("GATEWAY_USAGE_LOG_RETENTION_DAYS")); value != "" {
		if n, err := strconv.Atoi(value); err == nil && n >= 0 {
			days = n
		}
	}
	hours := defaultUsageLogPruneHours
	if value := strings.TrimSpace(os.Getenv("GATEWAY_USAGE_LOG_PRUNE_INTERVAL_HOURS")); value != "" {
		if n, err := strconv.Atoi(value); err == nil && n > 0 {
			hours = n
		}
	}
	return time.Duration(days) * 24 * time.Hour, time.Duration(hours) * time.Hour
}

// UsageLogRetention prunes audit rows that fall outside the retention window.
type UsageLogRetention struct {
	repo      *Repository
	retention time.Duration
	interval  time.Duration
	stop      chan struct{}
	stopOnce  sync.Once
	wg        sync.WaitGroup
}

// InitUsageLogRetention starts the pruning worker. A zero retention window (or a
// nil repository) disables audit-row pruning, so an operator can opt out and keep
// every row.
//
// A partitioned usage_logs keeps the worker running even with retention disabled: the
// audit dedupe registry (usage_audit_ids) has its own 30-day clock and the month
// partitions must stay ahead of the calendar, otherwise the registry grows forever
// and new audit rows start falling into the DEFAULT partition.
func InitUsageLogRetention(repo *Repository) *UsageLogRetention {
	retention, interval := ResolveUsageLogRetention()
	worker := &UsageLogRetention{
		repo:      repo,
		retention: retention,
		interval:  interval,
		stop:      make(chan struct{}),
	}
	partitioned := repo != nil && repo.usageLogsPartitioned()
	if repo == nil || retention <= 0 && !partitioned {
		return worker
	}
	worker.wg.Add(1)
	go worker.worker()
	return worker
}

// Stop waits for the pruning worker to exit.
func (w *UsageLogRetention) Stop() {
	if w == nil {
		return
	}
	w.stopOnce.Do(func() {
		close(w.stop)
	})
	w.wg.Wait()
}

func (w *UsageLogRetention) worker() {
	defer w.wg.Done()
	ticker := time.NewTicker(w.interval)
	defer ticker.Stop()

	select {
	case <-w.stop:
		return
	case <-time.After(usageLogPruneStartupDelay):
	}
	for {
		w.pruneOnce()
		select {
		case <-w.stop:
			return
		case <-ticker.C:
		}
	}
}

// usageAuditIDRetention is how long the partitioned-mode audit dedupe registry
// remembers a billing_id. See usageAuditIDRetentionDays.
const usageAuditIDRetention = time.Duration(usageAuditIDRetentionDays) * 24 * time.Hour

func (w *UsageLogRetention) pruneOnce() {
	if w.repo.usageLogsPartitioned() {
		// Partition health is independent of the audit retention window: keep the
		// calendar provisioned and trim the dedupe registry on its own clock first, so
		// a deployment that keeps audit rows forever still cannot grow those two
		// structures without bound.
		if err := w.repo.EnsureUsageLogMonthPartitions(usageLogPartitionMonthsAhead); err != nil {
			telemetry.ScopedLogger("billing").Warn("failed to provision usage_logs month partitions",
				"error", err.Error())
		}
		if deleted, err := w.repo.PurgeUsageAuditIDsBefore(time.Now().Add(-usageAuditIDRetention), usageLogPruneBatchSize); err != nil {
			telemetry.ScopedLogger("billing").Warn("failed to prune the audit dedupe registry",
				"error", err.Error())
		} else if deleted > 0 {
			telemetry.ScopedLogger("billing").Info("pruned expired audit dedupe ids",
				"deleted", deleted, "retention_days", usageAuditIDRetentionDays)
		}
	}

	if w.retention <= 0 {
		return
	}
	cutoff := time.Now().Add(-w.retention)
	deleted, err := w.repo.PurgeUsageLogsBefore(cutoff, usageLogPruneBatchSize)
	if err != nil {
		telemetry.ScopedLogger("billing").Warn("failed to prune expired usage logs",
			"error", err.Error(),
			"cutoff", cutoff.UTC().Format(time.RFC3339),
		)
		return
	}
	if deleted > 0 {
		telemetry.ScopedLogger("billing").Info("pruned expired usage logs",
			"deleted", deleted,
			"retention_days", int(w.retention.Hours()/24),
			"cutoff", cutoff.UTC().Format(time.RFC3339),
		)
	}
}

// PurgeUsageAuditIDsBefore deletes audit dedupe registry rows older than cutoff in
// bounded batches. It is a no-op unless usage_logs is partitioned (the registry only
// exists there) and is meant to run long after the audit rows themselves, so a
// redelivery inside the delivery window is still deduplicated.
func (r *Repository) PurgeUsageAuditIDsBefore(cutoff time.Time, batchSize int) (int64, error) {
	if r == nil || r.db == nil || !r.usageLogsPartitioned() {
		return 0, nil
	}
	if batchSize <= 0 {
		batchSize = 5000
	}
	var total int64
	for {
		res, err := r.db.Exec(`DELETE FROM `+usageAuditIDsTable+` WHERE billing_id IN (
			SELECT billing_id FROM `+usageAuditIDsTable+` WHERE created_at < ? LIMIT ?)`,
			cutoff.UTC(), batchSize)
		if err != nil {
			return total, err
		}
		deleted, err := res.RowsAffected()
		if err != nil {
			return total, err
		}
		total += deleted
		if deleted < int64(batchSize) {
			break
		}
	}
	return total, nil
}
