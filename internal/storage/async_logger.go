package storage

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/ifnodoraemon/airoute/internal/distributed"
	"github.com/ifnodoraemon/airoute/internal/telemetry"
)

const (
	RedisLogStreamKey = "nano:stream:logs"
	RedisLogGroup     = "nano_log_group"
)

// AsyncLogger buffers and batches usage log records to prevent blocking the data plane.
// It supports distributed Redis Streams queueing across multi-node clusters,
// with graceful fallback to an ultra-fast in-memory buffered channel.
type AsyncLogger struct {
	repo       *Repository
	ch         chan *UsageLogRecord
	stopCh     chan struct{}
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

// Record queues a usage log record asynchronously without blocking.
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
			rec.TraceID = fmt.Sprintf("tr-%x", time.Now().UnixNano())
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
	select {
	case al.ch <- rec:
	default:
		telemetry.Logger.Warn("async usage log buffer full, dropping log record to preserve data plane performance")
	}
}

// Stop gracefully flushes remaining logs and stops workers.
func (al *AsyncLogger) Stop() {
	if al == nil {
		return
	}
	close(al.stopCh)
	al.wg.Wait()
}

func (al *AsyncLogger) worker() {
	defer al.wg.Done()

	ticker := time.NewTicker(al.flushTimer)
	defer ticker.Stop()

	var batch []*UsageLogRecord

	flush := func() {
		if len(batch) == 0 {
			return
		}
		_ = al.repo.BatchRecordUsageLogs(batch)
		batch = batch[:0]
	}

	for {
		select {
		case rec := <-al.ch:
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
					flush()
					return
				}
			}
		}
	}
}

// redisStreamWorker continuously reads and batches log records from Redis Streams,
// persisting them to database in bulk with automatic consumer-group acknowledgement.
func (al *AsyncLogger) redisStreamWorker(client *distributed.Client) {
	defer al.wg.Done()

	consumerID := fmt.Sprintf("worker-%d-%x", os.Getpid(), time.Now().UnixNano()%1000000)
	ctx := context.Background()

	_ = client.EnsureConsumerGroup(ctx, RedisLogStreamKey, RedisLogGroup)
	telemetry.Logger.Info("started Redis Streams distributed log ingestion consumer",
		"stream", RedisLogStreamKey,
		"group", RedisLogGroup,
		"consumer", consumerID,
	)

	trimCounter := 0

	for {
		select {
		case <-al.stopCh:
			return
		default:
		}

		msgs, err := client.ReadGroupLogs(ctx, RedisLogStreamKey, RedisLogGroup, consumerID, int64(al.batchSize), al.flushTimer)
		if err != nil {
			time.Sleep(200 * time.Millisecond)
			continue
		}

		if len(msgs) == 0 {
			continue
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
			_ = al.repo.BatchRecordUsageLogs(streamBatch)
		}

		if len(ackIDs) > 0 {
			_ = client.AckLogs(ctx, RedisLogStreamKey, RedisLogGroup, ackIDs...)
		}

		trimCounter++
		if trimCounter >= 100 {
			trimCounter = 0
			_ = client.TrimStream(ctx, RedisLogStreamKey, 50000)
		}
	}
}
