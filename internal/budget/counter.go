package budget

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// Counter holds live spend per budget period. The persisted budgets.used_units
// is the source of truth after a flush; this layer exists so the data plane can
// answer "how much has this budget consumed right now" without a database query
// on every request.
type Counter interface {
	Get(ctx context.Context, budgetID int64, period Period, at time.Time) (int64, error)
	Add(ctx context.Context, budgetID int64, period Period, at time.Time, delta int64) (int64, error)
	Reset(ctx context.Context, budgetID int64, period Period, at time.Time) error
}

// CounterKey is the stable key of one budget period. It is shared by the memory
// and Redis implementations so a failover between them reads the same logical
// slot.
func CounterKey(budgetID int64, period Period, at time.Time) string {
	return fmt.Sprintf("airoute:budget:%d:%s", budgetID, period.Key(at))
}

// MemoryCounter is the single-replica implementation; it also serves as the
// fallback when Redis is not configured or momentarily unavailable.
type MemoryCounter struct {
	mu     sync.Mutex
	values map[string]int64
}

// NewMemoryCounter creates an empty counter.
func NewMemoryCounter() *MemoryCounter {
	return &MemoryCounter{values: make(map[string]int64)}
}

func (c *MemoryCounter) Get(_ context.Context, budgetID int64, period Period, at time.Time) (int64, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.values[CounterKey(budgetID, period, at)], nil
}

func (c *MemoryCounter) Add(_ context.Context, budgetID int64, period Period, at time.Time, delta int64) (int64, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	key := CounterKey(budgetID, period, at)
	c.values[key] += delta
	if c.values[key] < 0 {
		c.values[key] = 0
	}
	return c.values[key], nil
}

func (c *MemoryCounter) Reset(_ context.Context, budgetID int64, period Period, at time.Time) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.values, CounterKey(budgetID, period, at))
	return nil
}

// KV is the small Redis surface the counter needs. Keeping it an interface means
// the budget package stays independent of the Redis client and tests never need
// a server.
type KV interface {
	IncrByWithTTL(ctx context.Context, key string, delta int64, ttl time.Duration) (int64, error)
	GetInt(ctx context.Context, key string) (int64, error)
	Delete(ctx context.Context, key string) error
}

// KVCounter is the cross-replica implementation.
type KVCounter struct {
	kv KV
}

// NewKVCounter wraps a KV backend. A nil KV makes every call fail loudly rather
// than silently reporting zero usage.
func NewKVCounter(kv KV) *KVCounter {
	return &KVCounter{kv: kv}
}

func (c *KVCounter) Get(ctx context.Context, budgetID int64, period Period, at time.Time) (int64, error) {
	if c == nil || c.kv == nil {
		return 0, fmt.Errorf("budget counter backend is not configured")
	}
	return c.kv.GetInt(ctx, CounterKey(budgetID, period, at))
}

func (c *KVCounter) Add(ctx context.Context, budgetID int64, period Period, at time.Time, delta int64) (int64, error) {
	if c == nil || c.kv == nil {
		return 0, fmt.Errorf("budget counter backend is not configured")
	}
	return c.kv.IncrByWithTTL(ctx, CounterKey(budgetID, period, at), delta, period.TTL())
}

func (c *KVCounter) Reset(ctx context.Context, budgetID int64, period Period, at time.Time) error {
	if c == nil || c.kv == nil {
		return fmt.Errorf("budget counter backend is not configured")
	}
	return c.kv.Delete(ctx, CounterKey(budgetID, period, at))
}
