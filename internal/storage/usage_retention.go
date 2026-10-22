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
// nil repository) disables it, so an operator can opt out and keep every row.
func InitUsageLogRetention(repo *Repository) *UsageLogRetention {
	retention, interval := ResolveUsageLogRetention()
	worker := &UsageLogRetention{
		repo:      repo,
		retention: retention,
		interval:  interval,
		stop:      make(chan struct{}),
	}
	if repo == nil || retention <= 0 {
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

func (w *UsageLogRetention) pruneOnce() {
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
