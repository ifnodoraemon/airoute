package storage

import (
	"context"
	"encoding/json"
	"sync"
	"sync/atomic"
	"time"

	"github.com/ifnodoraemon/airoute/internal/distributed"
	"github.com/ifnodoraemon/airoute/internal/telemetry"
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
