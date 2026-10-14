package distributed

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
)

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
	return err
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

// TrimStream caps stream size to prevent unbounded memory growth.
func (c *Client) TrimStream(ctx context.Context, stream string, maxLen int64) error {
	if !c.IsActive() || maxLen <= 0 {
		return nil
	}
	return c.rdb.XTrimMaxLenApprox(ctx, stream, maxLen, 0).Err()
}
