package storage

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sync"
	"sync/atomic"
	"time"

	"github.com/ifnodoraemon/airoute/internal/distributed"
	"github.com/ifnodoraemon/airoute/internal/telemetry"
	"github.com/redis/go-redis/v9"
)

const (
	RedisLogStreamKey = "airoute:stream:logs"
	RedisLogGroup     = "airoute_log_group"

	// logWriteTimeout bounds a single database flush of buffered usage logs.
	// Billing batches are small; anything slower than this is a database problem
	// that must surface as a retry rather than as a wedged worker.
	logWriteTimeout = 10 * time.Second
)

// AsyncLogger buffers and batches usage log records to prevent blocking the data plane.
// It supports distributed Redis Streams queueing across multi-node clusters,
// with a bounded in-memory buffer that applies backpressure instead of
// silently dropping billing records when the database is unavailable.
type AsyncLogger struct {
	repo       *Repository
	ch         chan *UsageLogRecord
	stopCh     chan struct{}
	stopOnce   sync.Once
	stopped    atomic.Bool
	wg         sync.WaitGroup
	batchSize  int
	flushTimer time.Duration
}

// GlobalAsyncLogger is the application-wide async logger instance.
var GlobalAsyncLogger *AsyncLogger

// InitAsyncLogger initializes the background async logger worker.
func InitAsyncLogger(repo *Repository, bufferSize, batchSize int, flushInterval time.Duration) *AsyncLogger {
	if bufferSize <= 0 {
		bufferSize = 10000
	}
	if batchSize <= 0 {
		batchSize = 100
	}
	if flushInterval <= 0 {
		flushInterval = 500 * time.Millisecond
	}

	al := &AsyncLogger{
		repo:       repo,
		ch:         make(chan *UsageLogRecord, bufferSize),
		stopCh:     make(chan struct{}),
		batchSize:  batchSize,
		flushTimer: flushInterval,
	}

	GlobalAsyncLogger = al

	// Start local in-memory worker
	al.wg.Add(1)
	go al.worker()

	// If distributed Redis is available, start distributed stream consumer
	redisClient := distributed.GetClient()
	if redisClient != nil && redisClient.IsActive() {
		al.wg.Add(1)
		go al.redisStreamWorker(redisClient)
	}

	return al
}

// Record queues a usage log record asynchronously. When the local buffer is
// full it waits for space so a database outage cannot silently lose billing.
// In distributed mode, writes directly to Redis Streams for cluster-wide log aggregation;
// gracefully falls back to local memory buffer if Redis is not configured or fails.
func (al *AsyncLogger) Record(rec *UsageLogRecord) {
	if al == nil || rec == nil {
		return
	}

	// Ensure trace_id is always set
	if rec.TraceID == "" {
		if rec.SessionID != "" {
			rec.TraceID = rec.SessionID
		} else {
			rec.TraceID = fmt.Sprintf("tr-%x", time.Now().UnixMicro())
		}
	}

	// Try distributed message queue first if active
	redisClient := distributed.GetClient()
	if redisClient != nil && redisClient.IsActive() {
		data, err := json.Marshal(rec)
		if err == nil {
			ctx, cancel := context.WithTimeout(context.Background(), 250*time.Millisecond)
			err = redisClient.XAddLog(ctx, RedisLogStreamKey, string(data))
			cancel()
			if err == nil {
				return
			}
			telemetry.Logger.Warn("failed to push log to Redis Stream, falling back to local memory buffer", "error", err.Error())
		}
	}

	// Fallback to local memory buffered channel
	if al.ch == nil {
		return
	}
	if al.stopped.Load() {
		telemetry.Logger.Warn("async logger already stopped; dropping log record", "trace_id", rec.TraceID)
		telemetry.GlobalMetrics.UsageLogRecordsDropped.Add(1)
		return
	}
	timer := time.NewTimer(500 * time.Millisecond)
	defer timer.Stop()
	select {
	case al.ch <- rec:
		telemetry.GlobalMetrics.UsageLogQueueDepth.Store(int64(len(al.ch)))
	case <-al.stopCh:
		telemetry.Logger.Error("usage log submitted after logger stopped", "trace_id", rec.TraceID)
		telemetry.GlobalMetrics.UsageLogRecordsDropped.Add(1)
	case <-timer.C:
		telemetry.Logger.Error("async logger buffer full and flush stalled; dropping log record to prevent gateway freeze",
			"trace_id", rec.TraceID,
			"model", rec.Model,
			"buffer_capacity", cap(al.ch),
		)
		telemetry.GlobalMetrics.UsageLogRecordsDropped.Add(1)
	}
}

// Stop gracefully flushes remaining logs and stops workers.
func (al *AsyncLogger) Stop() {
	if al == nil {
		return
	}
	al.stopOnce.Do(func() {
		al.stopped.Store(true)
		close(al.stopCh)
	})
	al.wg.Wait()
}

func (al *AsyncLogger) worker() {
	defer al.wg.Done()

	ticker := time.NewTicker(al.flushTimer)
	defer ticker.Stop()

	var batch []*UsageLogRecord

	failures := 0
	const maxFailures = 5
	flush := func() bool {
		if len(batch) == 0 {
			return true
		}
		// Bound each flush: an unbounded wait would wedge this worker, fill the
		// queue and eventually block request completion on backpressure.
		flushCtx, cancel := context.WithTimeout(context.Background(), logWriteTimeout)
		err := al.repo.BatchRecordUsageLogsContext(flushCtx, batch)
		cancel()
		if err != nil {
			failures++
			telemetry.GlobalMetrics.UsageLogFlushFailures.Add(1)
			telemetry.Logger.Error("failed to persist usage logs",
				"error", err.Error(), "count", len(batch), "attempt", failures)
			if failures >= maxFailures {
				// Give up on this batch rather than letting memory grow without
				// bound; the records are surfaced loudly so they can be
				// reconciled from the Redis stream or upstream provider bills.
				telemetry.Logger.Error("dropping unpersistable usage log batch after repeated failures",
					"count", len(batch), "attempts", failures)
				telemetry.GlobalMetrics.UsageLogRecordsDropped.Add(uint64(len(batch)))
				batch = batch[:0]
				failures = 0
				return true
			}
			return false
		}
		telemetry.GlobalMetrics.UsageLogRecordsFlushed.Add(uint64(len(batch)))
		batch = batch[:0]
		failures = 0
		return true
	}

	for {
		select {
		case rec := <-al.ch:
			telemetry.GlobalMetrics.UsageLogQueueDepth.Store(int64(len(al.ch)))
			batch = append(batch, rec)
			if len(batch) >= al.batchSize {
				flush()
			}

		case <-ticker.C:
			flush()

		case <-al.stopCh:
			// Drain remaining in buffer
			for {
				select {
				case rec := <-al.ch:
					batch = append(batch, rec)
				default:
					for attempt := 0; attempt < 3 && !flush(); attempt++ {
						time.Sleep(time.Duration(attempt+1) * 100 * time.Millisecond)
					}
					if len(batch) > 0 {
						telemetry.Logger.Error("usage logs remain unpersisted at shutdown", "count", len(batch))
						telemetry.GlobalMetrics.UsageLogRecordsDropped.Add(uint64(len(batch)))
					}
					return
				}
			}
		}
	}
}

// redisStreamWorker continuously reads and batches log records from Redis Streams,
// persisting them to database in bulk with automatic consumer-group acknowledgement,
// PEL recovery on startup, exponential backoff retries, and AutoClaim for orphaned messages.
func (al *AsyncLogger) redisStreamWorker(client *distributed.Client) {
	defer al.wg.Done()

	hostname, err := os.Hostname()
	if err != nil || hostname == "" {
		hostname = "worker"
	}
	consumerID := fmt.Sprintf("%s-%d", hostname, os.Getpid())
	ctx := context.Background()

	_ = client.EnsureConsumerGroup(ctx, RedisLogStreamKey, RedisLogGroup)
	telemetry.Logger.Info("started Redis Streams distributed log ingestion consumer",
		"stream", RedisLogStreamKey,
		"group", RedisLogGroup,
		"consumer", consumerID,
	)

	// ackedUpTo is the highest stream id this consumer has persisted and
	// acknowledged. The periodic trim uses it as a floor, so trimming can only
	// ever discard history that is already in the database.
	ackedUpTo := ""

	processStreamMessages := func(msgs []redis.XMessage) bool {
		if len(msgs) == 0 {
			return true
		}
		ackIDs := make([]string, 0, len(msgs))
		var streamBatch []*UsageLogRecord
		for _, msg := range msgs {
			ackIDs = append(ackIDs, msg.ID)
			raw, ok := msg.Values["data"].(string)
			if !ok || raw == "" {
				continue
			}
			var rec UsageLogRecord
			if err := json.Unmarshal([]byte(raw), &rec); err == nil {
				streamBatch = append(streamBatch, &rec)
			}
		}

		if len(streamBatch) > 0 {
			var writeErr error
			for attempt := 1; attempt <= 3; attempt++ {
				writeCtx, cancel := context.WithTimeout(ctx, logWriteTimeout)
				writeErr = al.repo.BatchRecordUsageLogsContext(writeCtx, streamBatch)
				cancel()
				if writeErr == nil {
					break
				}
				time.Sleep(time.Duration(attempt*100) * time.Millisecond)
			}
			if writeErr != nil {
				telemetry.GlobalMetrics.UsageLogFlushFailures.Add(1)
				telemetry.Logger.Error("failed to batch write logs from Redis stream after retries, keeping in PEL for recovery",
					"error", writeErr.Error(),
					"count", len(streamBatch),
				)
				return false // Keep unacknowledged in PEL
			}
			telemetry.GlobalMetrics.UsageLogRecordsFlushed.Add(uint64(len(streamBatch)))
		}

		if len(ackIDs) > 0 {
			if err := client.AckLogs(ctx, RedisLogStreamKey, RedisLogGroup, ackIDs...); err == nil {
				ackedUpTo = ackIDs[len(ackIDs)-1]
			}
		}
		return true
	}

	// 1. Startup: process any unacknowledged messages in our PEL from previous restart.
	// This is bounded on purpose. A failing database used to make the loop re-read
	// the same batch immediately, over and over, and could never make progress.
	// Whatever stays unacknowledged here is retried by the periodic AutoClaim
	// below, so backing off costs nothing.
	const maxPELRecoveryAttempts = 5
	lastRecovered := ""
	for attempt := 1; ; attempt++ {
		pendingMsgs, err := client.ReadPendingPEL(ctx, RedisLogStreamKey, RedisLogGroup, consumerID, int64(al.batchSize))
		if err != nil {
			telemetry.Logger.Warn("failed to read unacknowledged PEL messages on startup", "error", err.Error())
			break
		}
		if len(pendingMsgs) == 0 {
			break
		}
		if lastRecovered == pendingMsgs[0].ID {
			// XACK did not take effect, so the next read returns the same head.
			// Rewriting it would duplicate billing rows; the AutoClaim ticker
			// retries the leftovers, so stop here instead of spinning.
			telemetry.Logger.Warn("startup PEL recovery stopped making progress; remaining messages stay pending for AutoClaim",
				"next_id", pendingMsgs[0].ID)
			break
		}
		telemetry.Logger.Info("re-processing unacknowledged PEL messages on startup", "count", len(pendingMsgs))
		if processStreamMessages(pendingMsgs) {
			lastRecovered = pendingMsgs[0].ID
			continue
		}
		if attempt >= maxPELRecoveryAttempts {
			telemetry.Logger.Error("abandoning startup PEL recovery after repeated database failures; messages stay pending for AutoClaim",
				"attempts", attempt,
				"pending", len(pendingMsgs),
			)
			break
		}
		backoff := time.Duration(attempt) * 500 * time.Millisecond
		select {
		case <-al.stopCh:
			return
		case <-time.After(backoff):
		}
	}

	trimCounter := 0
	claimTicker := time.NewTicker(30 * time.Second)
	defer claimTicker.Stop()
	claimStart := "0-0"

	for {
		select {
		case <-al.stopCh:
			return

		case <-claimTicker.C:
			// Reclaim messages from dead or unresponsive consumers in group
			claimedMsgs, nextStart, err := client.AutoClaimLogs(ctx, RedisLogStreamKey, RedisLogGroup, consumerID, 30*time.Second, claimStart, int64(al.batchSize))
			if err == nil {
				claimStart = nextStart
				if len(claimedMsgs) > 0 {
					telemetry.Logger.Info("reclaimed orphaned PEL messages from dead consumers", "count", len(claimedMsgs))
					processStreamMessages(claimedMsgs)
				}
			}

		default:
			msgs, err := client.ReadGroupLogs(ctx, RedisLogStreamKey, RedisLogGroup, consumerID, int64(al.batchSize), al.flushTimer)
			if err != nil {
				time.Sleep(200 * time.Millisecond)
				continue
			}

			if len(msgs) > 0 {
				processStreamMessages(msgs)
			}

			trimCounter++
			if trimCounter >= 100 {
				trimCounter = 0
				// Trim only acknowledged history. MAXLEN could evict entries still
				// sitting unacknowledged in some consumer's PEL, silently losing
				// billing records; MINID can only drop history, and the group-wide
				// pending check makes that safe across every node, not just this
				// consumer.
				if pendingID, err := client.MinPendingID(ctx, RedisLogStreamKey, RedisLogGroup); err != nil {
					telemetry.Logger.Warn("failed to inspect pending log stream entries; skipping trim", "error", err.Error())
				} else if pendingID == "" {
					if err := client.TrimStreamBefore(ctx, RedisLogStreamKey, ackedUpTo); err != nil {
						telemetry.Logger.Warn("failed to trim acknowledged log stream entries", "error", err.Error())
					}
				}
			}
		}
	}
}
