package distributed

import (
	"context"
	"errors"
	"time"

	"github.com/ifnodoraemon/airoute/internal/budget"
	"github.com/redis/go-redis/v9"
)

// BudgetCounterKV adapts the Redis client to budget.KV so budget counters are
// shared by every replica. It returns nil when Redis is not active, and callers
// fall back to the in-memory counter.
func (c *Client) BudgetCounterKV() budget.KV {
	if c == nil || c.rdb == nil || !c.IsActive() {
		return nil
	}
	return &budgetKV{rdb: c.rdb}
}

type budgetKV struct {
	rdb *redis.Client
}

func (k *budgetKV) IncrByWithTTL(ctx context.Context, key string, delta int64, ttl time.Duration) (int64, error) {
	pipe := k.rdb.TxPipeline()
	incr := pipe.IncrBy(ctx, key, delta)
	if ttl > 0 {
		pipe.Expire(ctx, key, ttl)
	}
	if _, err := pipe.Exec(ctx); err != nil {
		return 0, err
	}
	return incr.Val(), nil
}

func (k *budgetKV) GetInt(ctx context.Context, key string) (int64, error) {
	value, err := k.rdb.Get(ctx, key).Int64()
	if errors.Is(err, redis.Nil) {
		return 0, nil
	}
	return value, err
}

func (k *budgetKV) Delete(ctx context.Context, key string) error {
	return k.rdb.Del(ctx, key).Err()
}
