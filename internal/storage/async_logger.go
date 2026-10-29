package storage

import (
	"context"
	"encoding/json"
	"os"
	"strconv"
	"strings"
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

	// RedisLogDeadLetterStream receives stream entries the consumer could not
	// decode. They are acknowledged (so they cannot wedge the billing PEL) but kept
	// here with their original id, reason and payload, because a dropped record
	// used to leave no trace at all.
	RedisLogDeadLetterStream = "airoute:stream:dead"
	// usageLogDeadLetterMaxLen bounds the dead-letter stream. It is diagnostic, not
	// billing-bearing (the audit row was never written), so a MAXLEN trim is safe.
	usageLogDeadLetterMaxLen = 1000
	// usageLogDeadLetterPayloadLimit bounds how much of the undecodable payload is
	// copied into the DLQ entry.
	usageLogDeadLetterPayloadLimit = 4096

	// defaultUsageSpillReplayInterval is how often the spill file is retried while
	// the process runs. The replay used to happen exactly once, at startup, so
	// records spilled by a mid-life database outage stayed on disk until the next
	// restart - and were dropped once the file hit its size cap.
	defaultUsageSpillReplayInterval = 60 * time.Second

	// usageLogGroupEnsureAttempts is how many times the consumer retries the
	// consumer-group creation before it reports the failure. It keeps retrying on
	// the read path afterwards, so a Redis that is slow to start does not leave the
	// consumer permanently unable to read.
	usageLogGroupEnsureAttempts = 5
	// usageLogGroupEnsureBackoff is the base backoff between group-creation tries.
	usageLogGroupEnsureBackoff = 200 * time.Millisecond
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
	// spill is the last-resort local append-only file. A record that no queue
	// accepts is written here instead of being dropped, because a dropped record is
	// an uncollected charge. It is replayed periodically (and immediately after a
	// spill) rather than only at the next start.
	spill *usageSpill
	// spillReplayInterval is how often the replay loop runs. A field (not a
	// constant) so tests can drive it without waiting a minute.
	spillReplayInterval time.Duration
	// spillReplayKick asks the replay loop to run now, e.g. right after a batch was
	// spilled: a database that has just recovered must not wait a full interval.
	spillReplayKick chan struct{}
	// spillReplayMu serialises replay passes so the periodic loop and a kick cannot
	// process the same file at the same time.
	spillReplayMu sync.Mutex
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
		repo:                repo,
		ch:                  make(chan *UsageLogRecord, bufferSize),
		stopCh:              make(chan struct{}),
		batchSize:           batchSize,
		flushTimer:          flushInterval,
		spill:               newUsageSpill(),
		spillReplayInterval: resolveUsageSpillReplayInterval(),
		spillReplayKick:     make(chan struct{}, 1),
	}

	GlobalAsyncLogger = al

	// Replay spilled records continuously, not only at startup: a database outage
	// that starts after boot must not leave audit rows (and the charges they
	// carry) on disk until the next restart.
	al.wg.Add(1)
	go al.spillReplayLoop()

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

	// Ensure trace_id is always set. It must never borrow the session id: a
	// session spans many requests, so the audit row would lose its per-request
	// correlation key (see ensureTraceID).
	ensureTraceID(rec)

	// Assign the billing idempotency key here, once, before the record can be
	// duplicated by any transport: the key then survives Redis redelivery, so a
	// replayed message is written (and charged) at most once.
	if rec.BillingID == "" {
		rec.BillingID = NewBillingID()
	}

	// The Redis payload and the spill file are persisted outside the database, so
	// they must not carry a cleartext credential either: normalize before either
	// transport sees the record.
	rec.APIKey = NormalizeStoredAPIKeyReference(rec.APIKey)

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
			warnUsageLogXAddFallback(err)
		}
	}

	// Fallback to local memory buffered channel
	if al.ch == nil {
		return
	}
	if al.stopped.Load() {
		al.spillRecords([]*UsageLogRecord{rec}, "logger stopped")
		return
	}
	timer := time.NewTimer(500 * time.Millisecond)
	defer timer.Stop()
	select {
	case al.ch <- rec:
		telemetry.GlobalMetrics.UsageLogQueueDepth.Store(int64(len(al.ch)))
	case <-al.stopCh:
		al.spillRecords([]*UsageLogRecord{rec}, "logger stopped while queueing")
	case <-timer.C:
		al.spillRecords([]*UsageLogRecord{rec}, "in-memory buffer full and flush stalled")
	}
}

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
			telemetry.ScopedLogger("system").Error("failed to persist usage logs",
				"error", err.Error(), "count", len(batch), "attempt", failures)
			if failures >= maxFailures {
				// Give up on the database for now, but not on the money: spool the
				// batch to disk so it is replayed later. Dropping it here would
				// silently discard up to a hundred charges.
				al.spillRecords(batch, "repeated flush failures")
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
						al.spillRecords(batch, "unpersisted at shutdown")
					}
					return
				}
			}
		}
	}
}

// ensureLogConsumerGroup creates the audit-log consumer group, retrying with
// backoff. It reports the last error instead of discarding it: a consumer without
// its group can never read again, and that used to be completely silent.
func (al *AsyncLogger) ensureLogConsumerGroup(ctx context.Context, client *distributed.Client) error {
	var lastErr error
	backoff := usageLogGroupEnsureBackoff
	for attempt := 1; attempt <= usageLogGroupEnsureAttempts; attempt++ {
		attemptCtx, cancel := context.WithTimeout(ctx, logWriteTimeout)
		lastErr = client.EnsureConsumerGroup(attemptCtx, RedisLogStreamKey, RedisLogGroup)
		cancel()
		if lastErr == nil {
			return nil
		}
		telemetry.WarnLogger(telemetry.ScopedLogger("system"), "usage-log-group-ensure", 10*time.Second,
			"failed to create the audit-log consumer group; retrying",
			"attempt", attempt, "attempts", usageLogGroupEnsureAttempts, "error", lastErr.Error())
		select {
		case <-al.stopCh:
			return lastErr
		case <-time.After(backoff):
		}
		if backoff < 2*time.Second {
			backoff *= 2
		}
	}
	return lastErr
}

// logConsumerGroupHealth logs the group's pending/consumer/lag numbers so "the
// ingestion pipeline is not moving" is visible at startup and after a recovery.
func (al *AsyncLogger) logConsumerGroupHealth(ctx context.Context, client *distributed.Client, stage string) {
	stats, err := client.ConsumerGroupStats(ctx, RedisLogStreamKey, RedisLogGroup)
	if err != nil {
		telemetry.WarnLogger(telemetry.ScopedLogger("system"), "usage-log-group-health", 30*time.Second,
			"cannot read audit-log consumer group health", "stage", stage, "error", err.Error())
		return
	}
	telemetry.ScopedLogger("system").Info("audit-log consumer group health",
		"stage", stage,
		"stream", RedisLogStreamKey,
		"group", RedisLogGroup,
		"pending", stats.Pending,
		"consumers", stats.Consumers,
		"lag", stats.Lag,
	)
}

// deadLetterStreamEntry records an undecodable stream entry in the dead-letter
// stream and counts it.
//
// The entry is still ACKed by the caller (see processStreamMessages): replaying it
// can never succeed, and leaving it pending would wedge the billing PEL forever.
// The loss is made explicit instead — a counter, an ERROR line with the entry id
// and reason, and a bounded DLQ copy of the payload.
func (al *AsyncLogger) deadLetterStreamEntry(ctx context.Context, client *distributed.Client, msg redis.XMessage, reason, payload string) {
	telemetry.GlobalMetrics.UsageLogStreamPoisoned.Add(1)
	if len(payload) > usageLogDeadLetterPayloadLimit {
		// Cut on a rune boundary so the DLQ copy stays valid UTF-8 (a raw byte cut
		// can split a multi-byte sequence and make the diagnostic payload unreadable).
		cut := usageLogDeadLetterPayloadLimit
		for cut > 0 && (payload[cut]&0xC0) == 0x80 {
			cut--
		}
		payload = payload[:cut]
	}
	dlqErr := client.DeadLetterLog(ctx, RedisLogDeadLetterStream, msg.ID, reason, payload, usageLogDeadLetterMaxLen)
	telemetry.ScopedLogger("system").Error("dead-lettered an undecodable usage log stream entry",
		"stream", RedisLogStreamKey,
		"entry_id", msg.ID,
		"reason", reason,
		"payload_bytes", len(payload),
		"dead_letter_stream", RedisLogDeadLetterStream,
	)
	if dlqErr != nil {
		telemetry.WarnLogger(telemetry.ScopedLogger("system"), "usage-log-dead-letter", 30*time.Second,
			"failed to write the dead-letter entry; the poison record is only counted now",
			"entry_id", msg.ID, "error", dlqErr.Error())
	}
}

// redisStreamWorker continuously reads and batches log records from Redis Streams,
// persisting them to database in bulk with automatic consumer-group acknowledgement,
// PEL recovery on startup, exponential backoff retries, and AutoClaim for orphaned messages.
func (al *AsyncLogger) redisStreamWorker(client *distributed.Client) {
	defer al.wg.Done()

	// A stable identity (NODE_ID) keeps one consumer entry per replica across
	// restarts instead of one per process start; the read loop below also sweeps
	// entries that have been idle for a day.
	consumerID := streamConsumerID()
	ctx := context.Background()

	// Group creation is retried and never silently ignored: if the group does not
	// exist, every XREADGROUP fails with NOGROUP and the consumer can never make
	// progress, while writers keep appending happy in the belief that the record is
	// queued. A startup failure here used to be discarded (`_ =`).
	if err := al.ensureLogConsumerGroup(ctx, client); err != nil {
		telemetry.ScopedLogger("system").Error("failed to create the audit-log consumer group; the consumer will keep retrying and log once recovered",
			"stream", RedisLogStreamKey,
			"group", RedisLogGroup,
			"error", err.Error(),
		)
	} else {
		al.logConsumerGroupHealth(ctx, client, "startup")
	}
	telemetry.ScopedLogger("system").Info("started Redis Streams distributed log ingestion consumer",
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
				// Undecodable payload: it cannot be replayed into a row, but it must
				// not disappear silently either. Dead-letter it (bounded) before the
				// ACK below removes it from the billing PEL.
				reason := "stream entry has no string \"data\" field"
				if ok {
					reason = "stream entry has an empty \"data\" field"
				}
				al.deadLetterStreamEntry(ctx, client, msg, reason, "")
				continue
			}
			var rec UsageLogRecord
			if err := json.Unmarshal([]byte(raw), &rec); err == nil {
				if rec.BillingID == "" {
					// Payload written by a binary that predates billing_id. The
					// stream entry id is stable across PEL recovery and AutoClaim,
					// so deriving the key from it makes the redelivery settle once.
					rec.BillingID = BillingIDFromStreamEntry(msg.ID)
				}
				streamBatch = append(streamBatch, &rec)
			} else {
				al.deadLetterStreamEntry(ctx, client, msg, "stream entry payload is not valid JSON: "+err.Error(), raw)
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
				telemetry.ScopedLogger("system").Error("failed to batch write logs from Redis stream after retries, keeping in PEL for recovery",
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
			telemetry.ScopedLogger("system").Warn("failed to read unacknowledged PEL messages on startup", "error", err.Error())
			break
		}
		if len(pendingMsgs) == 0 {
			break
		}
		if lastRecovered == pendingMsgs[0].ID {
			// XACK did not take effect, so the next read returns the same head.
			// Rewriting it would duplicate billing rows; the AutoClaim ticker
			// retries the leftovers, so stop here instead of spinning.
			telemetry.ScopedLogger("system").Warn("startup PEL recovery stopped making progress; remaining messages stay pending for AutoClaim",
				"next_id", pendingMsgs[0].ID)
			break
		}
		telemetry.ScopedLogger("system").Info("re-processing unacknowledged PEL messages on startup", "count", len(pendingMsgs))
		if processStreamMessages(pendingMsgs) {
			lastRecovered = pendingMsgs[0].ID
			continue
		}
		if attempt >= maxPELRecoveryAttempts {
			telemetry.ScopedLogger("system").Error("abandoning startup PEL recovery after repeated database failures; messages stay pending for AutoClaim",
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
	// Consumer entries are never garbage-collected by Redis; the claim loop only
	// reclaims their pending messages. Sweep entries that have been idle for a day
	// and own no pending messages, so XINFO keeps describing the replicas that are
	// actually alive rather than every process start in the last weeks.
	pruneTicker := time.NewTicker(10 * time.Minute)
	defer pruneTicker.Stop()
	claimStart := "0-0"

	for {
		select {
		case <-al.stopCh:
			return

		case <-pruneTicker.C:
			pruned, pruneErr := client.PruneIdleConsumers(ctx, RedisLogStreamKey, RedisLogGroup, consumerID, 24*time.Hour)
			if pruned > 0 {
				telemetry.ScopedLogger("system").Info("pruned idle audit-log stream consumers",
					"stream", RedisLogStreamKey, "group", RedisLogGroup, "count", pruned)
			}
			if pruneErr != nil && !distributed.IsConsumerGroupMissing(pruneErr) {
				telemetry.WarnLogger(telemetry.ScopedLogger("system"), "usage-log-consumer-prune", 10*time.Minute,
					"failed to prune idle audit-log stream consumers",
					"stream", RedisLogStreamKey, "group", RedisLogGroup, "error", pruneErr.Error())
			}

		case <-claimTicker.C:
			// Reclaim messages from dead or unresponsive consumers in group. This is
			// also the periodic health check: if the group vanished while the read
			// path was idle (no traffic, so no failing read to react to), recreate it
			// here instead of waiting for the next request.
			claimedMsgs, nextStart, err := client.AutoClaimLogs(ctx, RedisLogStreamKey, RedisLogGroup, consumerID, 30*time.Second, claimStart, int64(al.batchSize))
			if err != nil && distributed.IsConsumerGroupMissing(err) {
				if ensureErr := al.ensureLogConsumerGroup(ctx, client); ensureErr != nil {
					telemetry.ScopedLogger("system").Error("failed to recreate the missing audit-log consumer group during the periodic check",
						"stream", RedisLogStreamKey, "group", RedisLogGroup, "error", ensureErr.Error())
				} else {
					telemetry.GlobalMetrics.UsageLogGroupRecreated.Add(1)
					telemetry.ScopedLogger("system").Error("audit-log consumer group was missing and has been recreated during the periodic check",
						"stream", RedisLogStreamKey, "group", RedisLogGroup)
					al.logConsumerGroupHealth(ctx, client, "recovered")
					claimStart = "0-0"
				}
			}
			if err == nil {
				claimStart = nextStart
				if len(claimedMsgs) > 0 {
					telemetry.ScopedLogger("system").Info("reclaimed orphaned PEL messages from dead consumers", "count", len(claimedMsgs))
					processStreamMessages(claimedMsgs)
				}
			}

		default:
			msgs, err := client.ReadGroupLogs(ctx, RedisLogStreamKey, RedisLogGroup, consumerID, int64(al.batchSize), al.flushTimer)
			if err != nil {
				// Never a bare sleep. A missing group is a *rebuild* signal: while it
				// stays missing every read fails, writers keep appending, and nothing
				// is ever persisted - the exact silent stall this branch used to
				// create. Other failures are still counted and logged (throttled) so
				// "the pipeline is stuck" is visible without restarting.
				telemetry.GlobalMetrics.UsageLogStreamReadFailures.Add(1)
				if distributed.IsConsumerGroupMissing(err) {
					telemetry.WarnLogger(telemetry.ScopedLogger("system"), "usage-log-group-missing", 30*time.Second,
						"audit-log consumer group is missing; recreating it",
						"stream", RedisLogStreamKey, "group", RedisLogGroup, "error", err.Error())
					if ensureErr := al.ensureLogConsumerGroup(ctx, client); ensureErr != nil {
						telemetry.ScopedLogger("system").Error("failed to recreate the audit-log consumer group; the consumer cannot make progress",
							"stream", RedisLogStreamKey, "group", RedisLogGroup, "error", ensureErr.Error())
						time.Sleep(time.Second)
						continue
					}
					telemetry.GlobalMetrics.UsageLogGroupRecreated.Add(1)
					telemetry.ScopedLogger("system").Error("audit-log consumer group was missing and has been recreated; resuming ingestion",
						"stream", RedisLogStreamKey, "group", RedisLogGroup, "consumer", consumerID)
					al.logConsumerGroupHealth(ctx, client, "recovered")
					// The rebuilt group starts from "0", so the backlog is readable
					// immediately; do not sleep here.
					continue
				}
				telemetry.WarnLogger(telemetry.ScopedLogger("system"), "usage-log-stream-read", 30*time.Second,
					"failed to read the audit-log stream; retrying",
					"stream", RedisLogStreamKey, "group", RedisLogGroup, "error", err.Error())
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
					telemetry.ScopedLogger("system").Warn("failed to inspect pending log stream entries; skipping trim", "error", err.Error())
				} else if pendingID == "" {
					if err := client.TrimStreamBefore(ctx, RedisLogStreamKey, ackedUpTo); err != nil {
						telemetry.ScopedLogger("system").Warn("failed to trim acknowledged log stream entries", "error", err.Error())
					}
				}
			}
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
