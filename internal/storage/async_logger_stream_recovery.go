package storage

import (
	"time"

	"github.com/ifnodoraemon/airoute/internal/distributed"
	"github.com/ifnodoraemon/airoute/internal/telemetry"
)

func (c *logStreamConsumer) recoverPending() bool {
	// 1. Startup: process any unacknowledged messages in our PEL from previous restart.
	// This is bounded on purpose. A failing database used to make the loop re-read
	// the same batch immediately, over and over, and could never make progress.
	// Whatever stays unacknowledged here is retried by the periodic AutoClaim
	// below, so backing off costs nothing.
	const maxPELRecoveryAttempts = 5
	lastRecovered := ""
	for attempt := 1; ; attempt++ {
		pendingMsgs, err := c.client.ReadPendingPEL(c.ctx, RedisLogStreamKey, RedisLogGroup, c.consumerID, int64(c.logger.batchSize))
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
		if c.processMessages(pendingMsgs) {
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
		case <-c.logger.stopCh:
			return false
		case <-time.After(backoff):
		}
	}

	return true
}

func (c *logStreamConsumer) pruneIdle() {
	pruned, pruneErr := c.client.PruneIdleConsumers(c.ctx, RedisLogStreamKey, RedisLogGroup, c.consumerID, 24*time.Hour)
	if pruned > 0 {
		telemetry.ScopedLogger("system").Info("pruned idle audit-log stream consumers",
			"stream", RedisLogStreamKey, "group", RedisLogGroup, "count", pruned)
	}
	if pruneErr != nil && !distributed.IsConsumerGroupMissing(pruneErr) {
		telemetry.WarnLogger(telemetry.ScopedLogger("system"), "usage-log-consumer-prune", 10*time.Minute,
			"failed to prune idle audit-log stream consumers",
			"stream", RedisLogStreamKey, "group", RedisLogGroup, "error", pruneErr.Error())
	}

}

func (c *logStreamConsumer) claimOrphans() {
	// Reclaim messages from dead or unresponsive consumers in group. This is
	// also the periodic health check: if the group vanished while the read
	// path was idle (no traffic, so no failing read to react to), recreate it
	// here instead of waiting for the next request.
	claimedMsgs, nextStart, err := c.client.AutoClaimLogs(c.ctx, RedisLogStreamKey, RedisLogGroup, c.consumerID, 30*time.Second, c.claimStart, int64(c.logger.batchSize))
	if err != nil && distributed.IsConsumerGroupMissing(err) {
		if ensureErr := c.logger.ensureLogConsumerGroup(c.ctx, c.client); ensureErr != nil {
			telemetry.ScopedLogger("system").Error("failed to recreate the missing audit-log consumer group during the periodic check",
				"stream", RedisLogStreamKey, "group", RedisLogGroup, "error", ensureErr.Error())
		} else {
			telemetry.GlobalMetrics.UsageLogGroupRecreated.Add(1)
			telemetry.ScopedLogger("system").Error("audit-log consumer group was missing and has been recreated during the periodic check",
				"stream", RedisLogStreamKey, "group", RedisLogGroup)
			c.logger.logConsumerGroupHealth(c.ctx, c.client, "recovered")
			c.claimStart = "0-0"
		}
	}
	if err == nil {
		c.claimStart = nextStart
		if len(claimedMsgs) > 0 {
			telemetry.ScopedLogger("system").Info("reclaimed orphaned PEL messages from dead consumers", "count", len(claimedMsgs))
			c.processMessages(claimedMsgs)
		}
	}

}

func (c *logStreamConsumer) trimAcknowledged() {
	c.trimCounter++
	if c.trimCounter >= 100 {
		c.trimCounter = 0
		// Trim only acknowledged history. MAXLEN could evict entries still
		// sitting unacknowledged in some consumer's PEL, silently losing
		// billing records; MINID can only drop history, and the group-wide
		// pending check makes that safe across every node, not just this
		// consumer.
		if pendingID, err := c.client.MinPendingID(c.ctx, RedisLogStreamKey, RedisLogGroup); err != nil {
			telemetry.ScopedLogger("system").Warn("failed to inspect pending log stream entries; skipping trim", "error", err.Error())
		} else if pendingID == "" {
			if err := c.client.TrimStreamBefore(c.ctx, RedisLogStreamKey, c.ackedUpTo); err != nil {
				telemetry.ScopedLogger("system").Warn("failed to trim acknowledged log stream entries", "error", err.Error())
			}
		}
	}
}
