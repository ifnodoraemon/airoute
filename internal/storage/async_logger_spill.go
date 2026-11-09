package storage

import (
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/ifnodoraemon/airoute/internal/telemetry"
)

// spillRecords writes records to the local append-only spill file when no queue
// will take them, and reports a billing-loss alert only if even that fails.
func (al *AsyncLogger) spillRecords(records []*UsageLogRecord, reason string) {
	if len(records) == 0 {
		return
	}
	written, err := al.spill.append(records)
	if err != nil {
		telemetry.GlobalMetrics.UsageLogRecordsDropped.Add(uint64(len(records)))
		telemetry.ScopedLogger("system").Error("BILLING LOSS: usage records could not be spooled or persisted",
			"reason", reason,
			"count", len(records),
			"error", err.Error(),
		)
		return
	}
	telemetry.GlobalMetrics.UsageLogRecordsSpilled.Add(uint64(written))
	telemetry.WarnLogger(telemetry.ScopedLogger("system"), "usage-spill-write", 30*time.Second,
		"usage log pipeline degraded; billing records spooled to disk",
		"reason", reason,
		"count", written,
	)
	// Try to drain it as soon as the database is healthy again instead of waiting
	// for the periodic pass (or worse, the next restart).
	al.kickSpillReplay()
}

// resolveUsageSpillReplayInterval reads the spill replay cadence.
// GATEWAY_USAGE_SPILL_REPLAY_SEC overrides the 60s default (tests use it).
func resolveUsageSpillReplayInterval() time.Duration {
	raw := strings.TrimSpace(os.Getenv("GATEWAY_USAGE_SPILL_REPLAY_SEC"))
	if raw != "" {
		if seconds, err := strconv.Atoi(raw); err == nil && seconds > 0 {
			return time.Duration(seconds) * time.Second
		}
	}
	return defaultUsageSpillReplayInterval
}

// spillReplayLoop replays spilled records continuously.
//
// It runs one pass immediately (records left by an earlier process are persisted
// before new traffic competes for the same database) and then once per interval,
// and can be woken by a kick when a batch has just been spilled. A failed pass
// keeps the files for the next attempt; only a successful pass removes them.
func (al *AsyncLogger) spillReplayLoop() {
	defer al.wg.Done()
	if al.spill == nil || !al.spill.enabled() || al.repo == nil {
		return
	}
	interval := al.spillReplayInterval
	if interval <= 0 {
		interval = defaultUsageSpillReplayInterval
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		al.replaySpillOnce()
		select {
		case <-al.stopCh:
			// One last pass so a shutdown does not strand records that landed while
			// the database was recovering.
			al.replaySpillOnce()
			return
		case <-ticker.C:
		case <-al.spillReplayKick:
		}
	}
}

// replaySpillOnce runs at most one replay pass at a time.
func (al *AsyncLogger) replaySpillOnce() {
	if al.spill == nil || !al.spill.enabled() || al.repo == nil {
		return
	}
	al.spillReplayMu.Lock()
	defer al.spillReplayMu.Unlock()
	replayed, err := al.spill.replay(al.repo)
	if err != nil {
		telemetry.ScopedLogger("system").Error("failed to replay spilled usage records; files are kept for the next attempt",
			"error", err.Error(), "replayed", replayed)
		return
	}
	if replayed > 0 {
		telemetry.GlobalMetrics.UsageLogSpillReplays.Add(uint64(replayed))
		telemetry.ScopedLogger("system").Info("replayed spilled usage records", "count", replayed)
	}
}

// kickSpillReplay asks the replay loop to run a pass now (best effort: the kick
// channel is depth 1, so a burst of spills collapses into one pass).
func (al *AsyncLogger) kickSpillReplay() {
	if al.spillReplayKick == nil {
		return
	}
	select {
	case al.spillReplayKick <- struct{}{}:
	default:
	}
}
