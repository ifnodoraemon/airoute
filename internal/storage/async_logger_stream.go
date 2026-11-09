package storage

import (
	"context"
	"time"

	"github.com/ifnodoraemon/airoute/internal/distributed"
	"github.com/ifnodoraemon/airoute/internal/telemetry"
	"github.com/redis/go-redis/v9"
)

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
