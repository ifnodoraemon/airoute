package storage

import (
	"context"
	"time"

	"github.com/ifnodoraemon/airoute/internal/telemetry"
)

type localUsageBatch struct {
	records  []*UsageLogRecord
	failures int
}

func (al *AsyncLogger) worker() {
	defer al.wg.Done()
	ticker := time.NewTicker(al.flushTimer)
	defer ticker.Stop()
	var batch localUsageBatch
	for {
		select {
		case rec := <-al.ch:
			telemetry.GlobalMetrics.UsageLogQueueDepth.Store(int64(len(al.ch)))
			batch.records = append(batch.records, rec)
			if len(batch.records) >= al.batchSize {
				al.flushLocalBatch(&batch)
			}
		case <-ticker.C:
			al.flushLocalBatch(&batch)
		case <-al.stopCh:
			al.drainLocalBatch(&batch)
			return
		}
	}
}

func (al *AsyncLogger) flushLocalBatch(batch *localUsageBatch) bool {
	if len(batch.records) == 0 {
		return true
	}
	// Bound each flush: an unbounded wait would wedge this worker, fill the
	// queue and eventually block request completion on backpressure.
	flushCtx, cancel := context.WithTimeout(context.Background(), logWriteTimeout)
	err := al.repo.BatchRecordUsageLogsContext(flushCtx, batch.records)
	cancel()
	if err != nil {
		batch.failures++
		telemetry.GlobalMetrics.UsageLogFlushFailures.Add(1)
		telemetry.ScopedLogger("system").Error("failed to persist usage logs",
			"error", err.Error(), "count", len(batch.records), "attempt", batch.failures)
		if batch.failures >= 5 {
			// Give up on the database for now, but not on the money: spool the
			// batch to disk so it is replayed later. Dropping it here would
			// silently discard up to a hundred charges.
			al.spillRecords(batch.records, "repeated flush failures")
			batch.records = batch.records[:0]
			batch.failures = 0
			return true
		}
		return false
	}
	telemetry.GlobalMetrics.UsageLogRecordsFlushed.Add(uint64(len(batch.records)))
	batch.records = batch.records[:0]
	batch.failures = 0
	return true
}

func (al *AsyncLogger) drainLocalBatch(batch *localUsageBatch) {
	for {
		select {
		case rec := <-al.ch:
			batch.records = append(batch.records, rec)
		default:
			for attempt := 0; attempt < 3 && !al.flushLocalBatch(batch); attempt++ {
				time.Sleep(time.Duration(attempt+1) * 100 * time.Millisecond)
			}
			if len(batch.records) > 0 {
				al.spillRecords(batch.records, "unpersisted at shutdown")
			}
			return
		}
	}
}

// usageLogXAddFallbackWarnKey throttles the Redis-stream fallback warning.
const usageLogXAddFallbackWarnKey = "usage-log-xadd-fallback"

// warnUsageLogXAddFallback reports that a usage record had to fall back from the
// Redis stream to the local buffer.
//
// This runs once per usage record while Redis is unavailable, so an unthrottled
// WARN turned a Redis hiccup into a log storm proportional to request volume (the
// same defect class as the per-request circuit-breaker alerts).
func warnUsageLogXAddFallback(err error) {
	telemetry.WarnLogger(telemetry.ScopedLogger("system"), usageLogXAddFallbackWarnKey, 30*time.Second,
		"failed to push log to Redis Stream, falling back to local memory buffer", "error", err.Error())
}
