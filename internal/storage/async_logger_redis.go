package storage

import (
	"context"
	"time"

	"github.com/ifnodoraemon/airoute/internal/distributed"
	"github.com/ifnodoraemon/airoute/internal/telemetry"
	"github.com/redis/go-redis/v9"
)

type logStreamConsumer struct {
	logger      *AsyncLogger
	client      *distributed.Client
	ctx         context.Context
	consumerID  string
	ackedUpTo   string
	claimStart  string
	trimCounter int
}

// Each worker owns its cursor. Messages are acknowledged only after persistence.
func (al *AsyncLogger) redisStreamWorker(client *distributed.Client) {
	defer al.wg.Done()
	c := &logStreamConsumer{logger: al, client: client, ctx: context.Background(), consumerID: streamConsumerID(), claimStart: "0-0"}
	c.start()
	if !c.recoverPending() {
		return
	}
	claimTicker := time.NewTicker(30 * time.Second)
	defer claimTicker.Stop()
	pruneTicker := time.NewTicker(10 * time.Minute)
	defer pruneTicker.Stop()
	for {
		select {
		case <-al.stopCh:
			return
		case <-pruneTicker.C:
			c.pruneIdle()
		case <-claimTicker.C:
			c.claimOrphans()
		default:
			c.read()
		}
	}
}

func (c *logStreamConsumer) start() {
	// Group creation is retried and never silently ignored: if the group does not
	// exist, every XREADGROUP fails with NOGROUP and the consumer can never make
	// progress, while writers keep appending happy in the belief that the record is
	// queued. A startup failure here used to be discarded (`_ =`).
	if err := c.logger.ensureLogConsumerGroup(c.ctx, c.client); err != nil {
		telemetry.ScopedLogger("system").Error("failed to create the audit-log consumer group; the consumer will keep retrying and log once recovered",
			"stream", RedisLogStreamKey,
			"group", RedisLogGroup,
			"error", err.Error(),
		)
	} else {
		c.logger.logConsumerGroupHealth(c.ctx, c.client, "startup")
	}
	telemetry.ScopedLogger("system").Info("started Redis Streams distributed log ingestion consumer",
		"stream", RedisLogStreamKey,
		"group", RedisLogGroup,
		"consumer", c.consumerID,
	)

}

func (c *logStreamConsumer) processMessages(msgs []redis.XMessage) bool {
	if len(msgs) == 0 {
		return true
	}
	batch, ackIDs := c.decodeMessages(msgs)
	if !c.persistBatch(batch) {
		return false
	}
	if len(ackIDs) > 0 {
		if err := c.client.AckLogs(c.ctx, RedisLogStreamKey, RedisLogGroup, ackIDs...); err == nil {
			c.ackedUpTo = ackIDs[len(ackIDs)-1]
		}
	}
	return true
}

func (c *logStreamConsumer) read() {
	msgs, err := c.client.ReadGroupLogs(c.ctx, RedisLogStreamKey, RedisLogGroup, c.consumerID, int64(c.logger.batchSize), c.logger.flushTimer)
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
			if ensureErr := c.logger.ensureLogConsumerGroup(c.ctx, c.client); ensureErr != nil {
				telemetry.ScopedLogger("system").Error("failed to recreate the audit-log consumer group; the consumer cannot make progress",
					"stream", RedisLogStreamKey, "group", RedisLogGroup, "error", ensureErr.Error())
				time.Sleep(time.Second)
				return
			}
			telemetry.GlobalMetrics.UsageLogGroupRecreated.Add(1)
			telemetry.ScopedLogger("system").Error("audit-log consumer group was missing and has been recreated; resuming ingestion",
				"stream", RedisLogStreamKey, "group", RedisLogGroup, "consumer", c.consumerID)
			c.logger.logConsumerGroupHealth(c.ctx, c.client, "recovered")
			// The rebuilt group starts from "0", so the backlog is readable
			// immediately; do not sleep here.
			return
		}
		telemetry.WarnLogger(telemetry.ScopedLogger("system"), "usage-log-stream-read", 30*time.Second,
			"failed to read the audit-log stream; retrying",
			"stream", RedisLogStreamKey, "group", RedisLogGroup, "error", err.Error())
		time.Sleep(200 * time.Millisecond)
		return
	}

	if len(msgs) > 0 {
		c.processMessages(msgs)
	}

	c.trimAcknowledged()
}
