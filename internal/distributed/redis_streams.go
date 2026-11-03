package distributed

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
)

// ErrConsumerGroupMissing reports that a stream read failed because the consumer
// group does not exist on the server (Redis error prefix "NOGROUP"). Callers must
// treat it as a rebuild signal, not as "no messages": a surviving consumer that
// keeps retrying without recreating the group never makes progress again, and the
// pipeline stalls silently while writers keep appending to the stream.
var ErrConsumerGroupMissing = fmt.Errorf("redis consumer group missing")

// IsConsumerGroupMissing reports whether err means the consumer group vanished.
//
// Redis returns "NOGROUP No such key ... or consumer group ..." for XREADGROUP /
// XACK when the group (or the whole stream key) is gone. The message text is the
// only portable signal (go-redis does not expose a typed error for it), so the
// check is on the wire prefix rather than on an error type.
func IsConsumerGroupMissing(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "NOGROUP") || strings.Contains(msg, "no such key")
}

// EnsureConsumerGroup ensures a Redis Stream consumer group exists.
// Uses "0" as start ID to consume all existing messages including backlog.
func (c *Client) EnsureConsumerGroup(ctx context.Context, stream, group string) error {
	if !c.IsActive() {
		return fmt.Errorf("redis client not active")
	}
	err := c.rdb.XGroupCreateMkStream(ctx, stream, group, "0").Err()
	if err != nil && (strings.Contains(err.Error(), "BUSYGROUP") || strings.Contains(err.Error(), "already exists")) {
		return nil
	}
	if err != nil && IsConsumerGroupMissing(err) {
		return fmt.Errorf("%w: %v", ErrConsumerGroupMissing, err)
	}
	return err
}

// StreamGroupStats is a point-in-time view of one consumer group.
type StreamGroupStats struct {
	// Pending is the number of entries delivered but not yet acknowledged across
	// every consumer of the group.
	Pending int64
	// Consumers is the number of consumer entries registered in the group. With a
	// stable consumer identity (NODE_ID) it approximates the live replica count;
	// entries left by processes that no longer run are swept by
	// PruneIdleConsumers (see redis_stream_consumers.go).
	Consumers int64
	// Lag is the number of entries added to the stream that the group has not
	// delivered yet (Redis >= 7), or -1 when the server does not report it.
	Lag int64
}

// ConsumerGroupStats returns the group's pending/consumer/lag numbers, which is
// what makes "the ingestion pipeline is stuck" visible at startup and on demand.
func (c *Client) ConsumerGroupStats(ctx context.Context, stream, group string) (StreamGroupStats, error) {
	if !c.IsActive() {
		return StreamGroupStats{}, fmt.Errorf("redis client not active")
	}
	groups, err := c.rdb.XInfoGroups(ctx, stream).Result()
	if err != nil {
		if IsConsumerGroupMissing(err) {
			return StreamGroupStats{}, fmt.Errorf("%w: %v", ErrConsumerGroupMissing, err)
		}
		return StreamGroupStats{}, err
	}
	for _, info := range groups {
		if info.Name != group {
			continue
		}
		return StreamGroupStats{Pending: info.Pending, Consumers: info.Consumers, Lag: info.Lag}, nil
	}
	return StreamGroupStats{}, fmt.Errorf("%w: group %q not present on stream %q", ErrConsumerGroupMissing, group, stream)
}

// DeadLetterLog appends one undecodable stream entry to the dead-letter stream and
// keeps that stream bounded.
//
// A poison entry cannot be repaired by replaying it, so it must not stay in the
// billing PEL — but dropping it without a trace loses audit evidence. The DLQ
// keeps the raw payload (truncated) plus the reason, and the MAXLEN trim is safe
// here because the DLQ is diagnostic, not billing-bearing: the audit row it
// describes was never written.
func (c *Client) DeadLetterLog(ctx context.Context, stream, originalID, reason, payload string, maxLen int64) error {
	if !c.IsActive() {
		return fmt.Errorf("redis client not active")
	}
	if maxLen <= 0 {
		maxLen = 1000
	}
	// Exact MAXLEN: the DLQ is small and diagnostic, and an approximate trim would
	// not enforce the bound at all on a stream that never grows past a few hundred
	// entries (Redis can only trim whole macro nodes approximately).
	if err := c.rdb.XAdd(ctx, &redis.XAddArgs{
		Stream: stream,
		MaxLen: maxLen,
		Approx: false,
		Values: map[string]interface{}{
			"original_id": originalID,
			"reason":      reason,
			"payload":     payload,
			"source":      "airoute-usage-log-consumer",
		},
	}).Err(); err != nil {
		return fmt.Errorf("append dead letter: %w", err)
	}
	return nil
}

// XAddLog pushes a message to a distributed Redis stream for asynchronous ingestion.
func (c *Client) XAddLog(ctx context.Context, stream string, data string) error {
	if !c.IsActive() {
		return fmt.Errorf("redis client not active")
	}
	return c.rdb.XAdd(ctx, &redis.XAddArgs{
		Stream: stream,
		Values: map[string]interface{}{"data": data},
	}).Err()
}

// ReadGroupLogs reads new pending messages from a Redis Stream consumer group.
func (c *Client) ReadGroupLogs(ctx context.Context, stream, group, consumer string, count int64, block time.Duration) ([]redis.XMessage, error) {
	if !c.IsActive() {
		return nil, fmt.Errorf("redis client not active")
	}
	streams, err := c.rdb.XReadGroup(ctx, &redis.XReadGroupArgs{
		Group:    group,
		Consumer: consumer,
		Streams:  []string{stream, ">"},
		Count:    count,
		Block:    block,
	}).Result()
	if err != nil {
		if err == redis.Nil {
			return nil, nil
		}
		return nil, err
	}
	if len(streams) == 0 {
		return nil, nil
	}
	return streams[0].Messages, nil
}

// ReadPendingPEL reads unacknowledged messages previously delivered to this consumer in the PEL.
func (c *Client) ReadPendingPEL(ctx context.Context, stream, group, consumer string, count int64) ([]redis.XMessage, error) {
	if !c.IsActive() {
		return nil, fmt.Errorf("redis client not active")
	}
	streams, err := c.rdb.XReadGroup(ctx, &redis.XReadGroupArgs{
		Group:    group,
		Consumer: consumer,
		Streams:  []string{stream, "0"},
		Count:    count,
	}).Result()
	if err != nil {
		if err == redis.Nil {
			return nil, nil
		}
		return nil, err
	}
	if len(streams) == 0 {
		return nil, nil
	}
	return streams[0].Messages, nil
}

// AutoClaimLogs reclaims pending messages from dead or hung consumers that exceeded minIdle duration.
func (c *Client) AutoClaimLogs(ctx context.Context, stream, group, consumer string, minIdle time.Duration, start string, count int64) ([]redis.XMessage, string, error) {
	if !c.IsActive() {
		return nil, "", fmt.Errorf("redis client not active")
	}
	if start == "" {
		start = "0-0"
	}
	msgs, nextStart, err := c.rdb.XAutoClaim(ctx, &redis.XAutoClaimArgs{
		Stream:   stream,
		Group:    group,
		Consumer: consumer,
		MinIdle:  minIdle,
		Start:    start,
		Count:    count,
	}).Result()
	if err != nil {
		if err == redis.Nil {
			return nil, "0-0", nil
		}
		return nil, start, err
	}
	return msgs, nextStart, nil
}

// AckLogs acknowledges processed messages in a consumer group.
func (c *Client) AckLogs(ctx context.Context, stream, group string, ids ...string) error {
	if !c.IsActive() || len(ids) == 0 {
		return nil
	}
	return c.rdb.XAck(ctx, stream, group, ids...).Err()
}

// TrimStreamBefore drops entries older than minID. Unlike a length cap it can
// never evict a message that has not been acknowledged yet, because callers pass
// the last acknowledged id. A MAXLEN cap raced with the pending-entries list and
// could silently delete billing records that had not been persisted.
func (c *Client) TrimStreamBefore(ctx context.Context, stream, minID string) error {
	if !c.IsActive() || minID == "" {
		return nil
	}
	return c.rdb.XTrimMinID(ctx, stream, minID).Err()
}

// MinPendingID returns the smallest id still pending in the consumer group, or ""
// when the group has no pending entries. In a multi-node cluster each consumer
// has its own PEL, so trimming by one consumer's acknowledged id is only safe
// when the whole group has nothing outstanding.
func (c *Client) MinPendingID(ctx context.Context, stream, group string) (string, error) {
	if !c.IsActive() {
		return "", fmt.Errorf("redis client not active")
	}
	pending, err := c.rdb.XPendingExt(ctx, &redis.XPendingExtArgs{
		Stream: stream,
		Group:  group,
		Start:  "-",
		End:    "+",
		Count:  1,
	}).Result()
	if err != nil {
		if err == redis.Nil {
			return "", nil
		}
		return "", err
	}
	if len(pending) == 0 {
		return "", nil
	}
	return pending[0].ID, nil
}
