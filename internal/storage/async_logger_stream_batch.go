package storage

import (
	"context"
	"encoding/json"
	"time"

	"github.com/ifnodoraemon/airoute/internal/telemetry"
	"github.com/redis/go-redis/v9"
)

func (c *logStreamConsumer) decodeMessages(msgs []redis.XMessage) ([]*UsageLogRecord, []string) {
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
			c.logger.deadLetterStreamEntry(c.ctx, c.client, msg, reason, "")
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
			c.logger.deadLetterStreamEntry(c.ctx, c.client, msg, "stream entry payload is not valid JSON: "+err.Error(), raw)
		}
	}

	return streamBatch, ackIDs
}

func (c *logStreamConsumer) persistBatch(batch []*UsageLogRecord) bool {
	if len(batch) > 0 {
		var writeErr error
		for attempt := 1; attempt <= 3; attempt++ {
			writeCtx, cancel := context.WithTimeout(c.ctx, logWriteTimeout)
			writeErr = c.logger.repo.BatchRecordUsageLogsContext(writeCtx, batch)
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
				"count", len(batch),
			)
			return false // Keep unacknowledged in PEL
		}
		telemetry.GlobalMetrics.UsageLogRecordsFlushed.Add(uint64(len(batch)))
	}

	return true
}
