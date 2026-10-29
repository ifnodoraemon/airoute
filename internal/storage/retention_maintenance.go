package storage

import (
	"os"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/ifnodoraemon/airoute/internal/telemetry"
)

// Opportunistic retention for the two tables that previously grew without bound.
//
// The gateway has no dedicated maintenance worker for them (usage_logs has one,
// but that worker belongs to the audit trail). Both tables are written rarely —
// one row per verification mail and one row per video task — so pruning from the
// write path itself is enough: each write is at most one bounded DELETE, and the
// atomic slot makes a burst of writes prune only once per interval.
const (
	defaultVerificationRetentionDays = 7
	defaultVideoTaskRetentionDays    = 30

	// Bound each DELETE so a long-neglected table cannot hold a write lock for a
	// long time on the SQLite deployment.
	retentionPruneBatchSize = 500

	// At most one prune attempt per interval, per repository.
	retentionPruneInterval = time.Hour

	// When a prune attempt fails the throttle slot is wound back to "retry in a
	// minute" instead of leaving it at "ran now": otherwise one transient database
	// error silently froze that table's cleanup for a full interval.
	retentionPruneRetryInterval = time.Minute
)

// verificationRetention is how long expired verification codes are kept after
// they expire (useful when investigating abuse), configurable via
// GATEWAY_VERIFICATION_RETENTION_DAYS. 0 keeps every row.
func verificationRetention() time.Duration {
	return time.Duration(envRetentionDays("GATEWAY_VERIFICATION_RETENTION_DAYS", defaultVerificationRetentionDays)) * 24 * time.Hour
}

// videoTaskRetention bounds how long a video task record is kept. Video polling
// is bounded by the provider (minutes to an hour), so a month is generous while
// stopping the table from growing forever. Configurable via
// GATEWAY_VIDEO_TASK_RETENTION_DAYS; 0 keeps every row.
func videoTaskRetention() time.Duration {
	return time.Duration(envRetentionDays("GATEWAY_VIDEO_TASK_RETENTION_DAYS", defaultVideoTaskRetentionDays)) * 24 * time.Hour
}

// envRetentionDays reads a non-negative day count, falling back to the default on
// an unparsable value (reported so a typo cannot silently change retention).
func envRetentionDays(name string, fallback int) int {
	raw := strings.TrimSpace(os.Getenv(name))
	if raw == "" {
		return fallback
	}
	days, err := strconv.Atoi(raw)
	if err != nil || days < 0 {
		telemetry.WarnLogger(telemetry.ScopedLogger("system"), "retention-config:"+name, 10*time.Minute,
			"ignoring invalid retention configuration; using the default",
			"variable", name, "value", raw, "default_days", fallback)
		return fallback
	}
	return days
}

// shouldRunRetention reports whether the throttled prune may run now.
func shouldRunRetention(slot *atomic.Int64, interval time.Duration) bool {
	now := time.Now().UnixNano()
	previous := slot.Load()
	if previous != 0 && now-previous < int64(interval) {
		return false
	}
	return slot.CompareAndSwap(previous, now)
}

// maybePrune runs one opportunistic retention pass: it honours the configured
// retention window and the once-per-interval slot, then reports the bounded
// delete. Both opportunistic tables share this shape, so the mechanics live here.
func (r *Repository) maybePrune(slot *atomic.Int64, retention func() time.Duration, purge func(time.Time) (int64, error), warnKey, warnMsg, infoMsg string) {
	if r == nil || r.db == nil {
		return
	}
	window := retention()
	if window <= 0 || !shouldRunRetention(slot, retentionPruneInterval) {
		return
	}
	n, err := purge(time.Now().Add(-window))
	if err != nil {
		// Retry soon rather than a full interval later: the slot was consumed before
		// the attempt, so without this a single failure means an hour of no cleanup
		// (and no log after the first one).
		rescheduleRetentionRetry(slot, retentionPruneInterval, retentionPruneRetryInterval)
		telemetry.WarnLogger(telemetry.ScopedLogger("system"), warnKey, 10*time.Minute, warnMsg, "error", err.Error(),
			"retry_in", retentionPruneRetryInterval.String())
	} else if n > 0 {
		telemetry.ScopedLogger("system").Info(infoMsg, "deleted", n)
	}
}

// rescheduleRetentionRetry rewinds the throttle slot so the attempt becomes due
// again after retryInterval, without allowing a tight retry loop on a database
// that is still failing.
func rescheduleRetentionRetry(slot *atomic.Int64, interval, retryInterval time.Duration) {
	if slot == nil {
		return
	}
	if retryInterval <= 0 || retryInterval >= interval {
		slot.Store(0)
		return
	}
	now := time.Now().UnixNano()
	slot.Store(now - int64(interval) + int64(retryInterval))
}

// maybePruneVerificationCodes prunes expired verification codes from the write
// path, at most once per retentionPruneInterval.
func (r *Repository) maybePruneVerificationCodes() {
	r.maybePrune(&r.verificationPruneAt, verificationRetention, r.PurgeVerificationCodesBefore,
		"verification-prune-failed", "failed to prune expired verification codes", "pruned expired verification codes")
}

// maybePruneVideoTasks prunes old video task records from the write path, at most
// once per retentionPruneInterval.
func (r *Repository) maybePruneVideoTasks() {
	r.maybePrune(&r.videoTaskPruneAt, videoTaskRetention, r.PurgeVideoTasksBefore,
		"video-task-prune-failed", "failed to prune expired video tasks", "pruned expired video tasks")
}

// PurgeVerificationCodesBefore deletes verification codes that expired before
// cutoff, in bounded batches. Expiry (not creation) is the cutoff so a code's
// whole lifetime plus the retention window is always kept.
func (r *Repository) PurgeVerificationCodesBefore(cutoff time.Time) (int64, error) {
	// verification_codes.expires_at is TIMESTAMPTZ on PostgreSQL and a UTC text
	// column on SQLite; purgeBeforeCutoff binds the right shape for each.
	return r.purgeBeforeCutoff(
		`DELETE FROM verification_codes WHERE id IN (SELECT id FROM verification_codes WHERE expires_at < ? ORDER BY id LIMIT ?)`,
		cutoff, retentionPruneBatchSize)
}

// PurgeVideoTasksBefore deletes video task records created before cutoff, in
// bounded batches.
func (r *Repository) PurgeVideoTasksBefore(cutoff time.Time) (int64, error) {
	return r.purgeBeforeCutoff(
		`DELETE FROM video_tasks WHERE id IN (SELECT id FROM video_tasks WHERE created_at < ? ORDER BY created_at LIMIT ?)`,
		cutoff, retentionPruneBatchSize)
}

// purgeBeforeCutoff runs one bounded-batch retention DELETE: the statement
// deletes at most batchSize rows per iteration, and the loop stops as soon as a
// short batch comes back (the table is drained) or a statement fails.
//
// It is the single implementation behind every retention purge: callers only
// supply their own DELETE (tables differ in the name and in which timestamp
// column the cutoff applies to) and their batch bound, so the loop mechanics
// cannot drift between tables. total holds what was deleted before a failure, so
// a partial prune is still reported. The cutoff is bound through dialectTime:
// the timestamp columns are TIMESTAMPTZ on PostgreSQL and UTC text on SQLite.
func (r *Repository) purgeBeforeCutoff(deleteStmt string, cutoff time.Time, batchSize int) (int64, error) {
	if r == nil || r.db == nil {
		return 0, nil
	}
	cutoffValue := r.dialectTime(cutoff)
	var total int64
	for {
		res, err := r.db.Exec(deleteStmt, cutoffValue, batchSize)
		if err != nil {
			return total, err
		}
		deleted, err := res.RowsAffected()
		if err != nil {
			return total, err
		}
		total += deleted
		if deleted < int64(batchSize) {
			return total, nil
		}
	}
}
