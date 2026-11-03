package distributed

import (
	"context"
	"fmt"
	"sort"
	"time"
)

// Consumer entries in a Redis Stream consumer group are created implicitly by
// XREADGROUP and must be removed explicitly with XGROUP DELCONSUMER; Redis never
// garbage-collects them. Before this file existed the audit-log consumer used a
// host-pid identity, so every restart of every replica left one more entry behind
// (measured: 136 entries, oldest idle ~3 days). They are harmless for delivery but
// they hide the real replica count in XINFO/StreamGroupStats and make "how many
// consumers are actually alive" unanswerable.
//
// Two changes fix that: the consumer identity is now stable across restarts when
// NODE_ID is set (see storage.streamConsumerID), and this sweeper removes entries
// that have been idle for longer than minIdle.
//
// A consumer is only removed when its Pending count is zero. A non-zero PEL means
// it still owns messages that have not been persisted, and DELCONSUMER would drop
// those entries from the pending list without a trace; they must be reclaimed
// first (the read loop already runs XAUTOCLAIM every 30s), so the next sweep will
// remove the entry.

// StreamConsumer is one consumer entry of a consumer group, as reported by
// XINFO CONSUMERS.
type StreamConsumer struct {
	Name     string
	Pending  int64
	Idle     time.Duration
	Inactive time.Duration
}

// ListConsumers returns the consumer entries registered in a group.
func (c *Client) ListConsumers(ctx context.Context, stream, group string) ([]StreamConsumer, error) {
	if !c.IsActive() {
		return nil, fmt.Errorf("redis client not active")
	}
	infos, err := c.rdb.XInfoConsumers(ctx, stream, group).Result()
	if err != nil {
		if IsConsumerGroupMissing(err) {
			return nil, fmt.Errorf("%w: %v", ErrConsumerGroupMissing, err)
		}
		return nil, err
	}
	out := make([]StreamConsumer, 0, len(infos))
	for _, info := range infos {
		out = append(out, StreamConsumer{
			Name:     info.Name,
			Pending:  info.Pending,
			Idle:     info.Idle,
			Inactive: info.Inactive,
		})
	}
	return out, nil
}

// DeleteConsumer removes one consumer entry from the group.
func (c *Client) DeleteConsumer(ctx context.Context, stream, group, name string) error {
	if !c.IsActive() {
		return fmt.Errorf("redis client not active")
	}
	return c.rdb.XGroupDelConsumer(ctx, stream, group, name).Err()
}

// staleConsumers is the selection rule behind PruneIdleConsumers, split out so it
// can be tested without a Redis server.
//
// Kept: the sweeper's own entry, any consumer that still owns pending messages
// (DELCONSUMER would silently discard that PEL), and anything that has been idle
// for less than minIdle.
func staleConsumers(consumers []StreamConsumer, self string, minIdle time.Duration) []string {
	stale := make([]string, 0)
	for _, consumer := range consumers {
		switch {
		case consumer.Name == "" || consumer.Name == self:
			continue
		case consumer.Pending > 0:
			continue
		case consumer.Idle < minIdle:
			continue
		}
		stale = append(stale, consumer.Name)
	}
	// Deterministic order keeps the log line and tests stable.
	sort.Strings(stale)
	return stale
}

// PruneIdleConsumers deletes consumer entries that have been idle for at least
// minIdle and own no pending messages. It returns how many were removed; a
// deletion error is reported after every candidate has been attempted, because one
// stuck entry must not stop the sweep.
func (c *Client) PruneIdleConsumers(ctx context.Context, stream, group, self string, minIdle time.Duration) (int, error) {
	consumers, err := c.ListConsumers(ctx, stream, group)
	if err != nil {
		return 0, err
	}
	pruned := 0
	var firstErr error
	for _, name := range staleConsumers(consumers, self, minIdle) {
		if err := c.DeleteConsumer(ctx, stream, group, name); err != nil {
			if firstErr == nil {
				firstErr = fmt.Errorf("delete consumer %q: %w", name, err)
			}
			continue
		}
		pruned++
	}
	return pruned, firstErr
}
