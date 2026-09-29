package distributed

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/ifnodoraemon/airoute/internal/telemetry"
	"github.com/redis/go-redis/v9"
)

const (
	ReloadChannel    = "nano:cluster:reload"
	ConfigVersionKey = "airoute:cluster:config_version"
)

// ClusterEvent represents an inter-node cluster message.
type ClusterEvent struct {
	Event     string `json:"event"`
	Reason    string `json:"reason"`
	NodeID    string `json:"node_id,omitempty"`
	Timestamp int64  `json:"timestamp"`
}

// Client wraps redis.Client for enterprise distributed gateway operations.
type Client struct {
	rdb    *redis.Client
	mu     sync.RWMutex
	active bool
}

var (
	globalClient *Client
	clientMu     sync.RWMutex
)

// InitRedis initializes the global Redis connection if configured.
// Gracefully degrades to local memory mode if redisURL is empty or unreachable.
func InitRedis(redisURL string) *Client {
	clientMu.Lock()
	defer clientMu.Unlock()

	if redisURL == "" {
		telemetry.Logger.Info("Redis not configured, running in local memory Zero-Dependency mode")
		globalClient = nil
		return nil
	}

	opt, err := redis.ParseURL(redisURL)
	if err != nil {
		telemetry.Logger.Warn("failed to parse REDIS_URL, falling back to local memory", "error", err.Error(), "url", redisURL)
		globalClient = nil
		return nil
	}

	opt.PoolSize = 128
	opt.MinIdleConns = 16
	opt.DialTimeout = 2 * time.Second
	opt.ReadTimeout = 1 * time.Second
	opt.WriteTimeout = 1 * time.Second

	rdb := redis.NewClient(opt)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	if err := rdb.Ping(ctx).Err(); err != nil {
		telemetry.Logger.Warn("cannot reach Redis server, gracefully falling back to local in-memory operation", "error", err.Error())
		_ = rdb.Close()
		globalClient = nil
		return nil
	}

	telemetry.Logger.Info("connected to Redis server successfully (Enterprise Distributed Cluster Mode ACTIVATED)",
		"addr", opt.Addr,
		"db", opt.DB,
	)

	globalClient = &Client{
		rdb:    rdb,
		active: true,
	}
	return globalClient
}

// GetClient returns the active distributed client or nil.
func GetClient() *Client {
	clientMu.RLock()
	defer clientMu.RUnlock()
	return globalClient
}

// IsActive returns whether Redis distributed features are active.
func (c *Client) IsActive() bool {
	if c == nil || c.rdb == nil {
		return false
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.active
}

// PublishReload broadcasts a cache-invalidation event across all cluster replicas and increments config version.
func (c *Client) PublishReload(ctx context.Context, reason string) error {
	if !c.IsActive() {
		return nil
	}

	newVer, _ := c.rdb.Incr(ctx, ConfigVersionKey).Result()

	evt := ClusterEvent{
		Event:     "reload",
		Reason:    reason,
		Timestamp: time.Now().UnixNano(),
	}
	payload, _ := json.Marshal(evt)

	err := c.rdb.Publish(ctx, ReloadChannel, string(payload)).Err()
	telemetry.Logger.Info("published cluster reload broadcast", "reason", reason, "version", newVer)
	return err
}

// SubscribeReload listens for real-time cache invalidation events from peer cluster nodes
// and periodically reconciles cluster configuration version to heal missed broadcasts.
func (c *Client) SubscribeReload(onReload func(reason string), stopChan <-chan struct{}) {
	if !c.IsActive() {
		return
	}

	go func() {
		pubsub := c.rdb.Subscribe(context.Background(), ReloadChannel)
		defer pubsub.Close()

		ch := pubsub.Channel()
		telemetry.Logger.Info("subscribed to Redis cluster invalidation channel", "channel", ReloadChannel)

		var localVersion int64
		if v, err := c.rdb.Get(context.Background(), ConfigVersionKey).Int64(); err == nil {
			localVersion = v
		}

		// Self-healing periodic ticker to catch any dropped pub/sub events
		reconcileTicker := time.NewTicker(15 * time.Second)
		defer reconcileTicker.Stop()

		for {
			select {
			case msg, ok := <-ch:
				if !ok {
					return
				}
				var evt ClusterEvent
				if err := json.Unmarshal([]byte(msg.Payload), &evt); err == nil {
					telemetry.Logger.Info("received Redis cluster reload broadcast (<1ms latency)", "reason", evt.Reason)
					if v, err := c.rdb.Get(context.Background(), ConfigVersionKey).Int64(); err == nil && v > localVersion {
						localVersion = v
					}
					onReload(evt.Reason)
				}
			case <-reconcileTicker.C:
				if v, err := c.rdb.Get(context.Background(), ConfigVersionKey).Int64(); err == nil {
					if v > localVersion {
						telemetry.Logger.Info("reconciled cluster config version divergence, triggering self-healing reload", "localVersion", localVersion, "remoteVersion", v)
						localVersion = v
						onReload("version_sync_reconciliation")
					}
				}
			case <-stopChan:
				return
			}
		}
	}()
}

// slidingWindowLua is an atomic Lua script implementing distributed sliding-window rate limiting.
var slidingWindowLua = redis.NewScript(`
local key = KEYS[1]
local limit = tonumber(ARGV[1])
local now = tonumber(ARGV[2])
local window = 60000 -- 60s in milliseconds
local clearBefore = now - window

redis.call('ZREMRANGEBYSCORE', key, 0, clearBefore)
local current = redis.call('ZCARD', key)
if current < limit then
    local seq = redis.call('INCR', key .. ':seq')
    local member = tostring(now) .. ':' .. tostring(seq)
    redis.call('ZADD', key, now, member)
    redis.call('PEXPIRE', key, window)
    redis.call('PEXPIRE', key .. ':seq', window)
    return 1
else
    return 0
end
`)

// AllowRPM checks rate limiting using Redis Sliding Window across the entire cluster.
func (c *Client) AllowRPM(ctx context.Context, apiKey string, rpm int) (bool, error) {
	if !c.IsActive() || rpm <= 0 {
		return true, nil // unmetered or not active
	}

	key := fmt.Sprintf("nano:ratelimit:rpm:%s", apiKey)
	nowMs := time.Now().UnixNano() / int64(time.Millisecond)

	res, err := slidingWindowLua.Run(ctx, c.rdb, []string{key}, rpm, nowMs).Result()
	if err != nil {
		// On Redis failure, fail-open to ensure API availability
		telemetry.Logger.Warn("redis rate limit query error, failing open to preserve traffic", "error", err.Error())
		return true, err
	}

	allowed, ok := res.(int64)
	if !ok || allowed != 1 {
		return false, nil
	}
	return true, nil
}

// RecordTokens atomically increments the distributed usage counter for an API key.
func (c *Client) RecordTokens(ctx context.Context, apiKey string, tokens int) error {
	if !c.IsActive() || tokens <= 0 {
		return nil
	}
	key := "nano:keys:tokens"
	return c.rdb.HIncrBy(ctx, key, apiKey, int64(tokens)).Err()
}

// GetSessionChannel retrieves the pinned provider channel name for a sticky session.
func (c *Client) GetSessionChannel(ctx context.Context, model, sessionID string) (string, error) {
	if !c.IsActive() || sessionID == "" {
		return "", nil
	}
	key := fmt.Sprintf("nano:session:%s:%s", model, sessionID)
	val, err := c.rdb.Get(ctx, key).Result()
	if err == redis.Nil {
		return "", nil
	}
	return val, err
}

// SetSessionChannel pins a session to a specific provider channel with a TTL (e.g. 30m) across the cluster.
func (c *Client) SetSessionChannel(ctx context.Context, model, sessionID, channelName string, ttl time.Duration) error {
	if !c.IsActive() || sessionID == "" || channelName == "" {
		return nil
	}
	key := fmt.Sprintf("nano:session:%s:%s", model, sessionID)
	return c.rdb.Set(ctx, key, channelName, ttl).Err()
}

// GetRDB returns the underlying redis.Client.
func (c *Client) GetRDB() *redis.Client {
	if c == nil {
		return nil
	}
	return c.rdb
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

// inFlightQuotaAcquireLua atomically checks and reserves in-flight user quota across the cluster.
var inFlightQuotaAcquireLua = redis.NewScript(`
local key = KEYS[1]
local balance = tonumber(ARGV[1])
local minCost = tonumber(ARGV[2])
local ttl = tonumber(ARGV[3])

local current = tonumber(redis.call('GET', key) or '0')
if balance - (current + 1) * minCost < 0 then
    return 0
else
    redis.call('INCR', key)
    redis.call('EXPIRE', key, ttl)
    return 1
end
`)

// inFlightQuotaReleaseLua atomically decrements user's in-flight request counter across the cluster.
var inFlightQuotaReleaseLua = redis.NewScript(`
local key = KEYS[1]
local current = tonumber(redis.call('GET', key) or '0')
if current <= 1 then
    redis.call('DEL', key)
else
    redis.call('DECR', key)
end
return 1
`)

// TryAcquireInFlightQuota atomically checks and reserves in-flight user quota across the cluster.
func (c *Client) TryAcquireInFlightQuota(ctx context.Context, userID int64, balance, minReserveCost float64) (bool, error) {
	if !c.IsActive() || userID <= 0 {
		return true, nil
	}
	key := fmt.Sprintf("nano:inflight:user:%d", userID)
	ttl := 120 // 120s safety timeout to avoid permanently trapped in-flight quota if node crashes
	res, err := inFlightQuotaAcquireLua.Run(ctx, c.rdb, []string{key}, balance, minReserveCost, ttl).Result()
	if err != nil {
		return false, err
	}
	val, ok := res.(int64)
	return ok && val == 1, nil
}

// ReleaseInFlightQuota decrements user's in-flight request counter across the cluster.
func (c *Client) ReleaseInFlightQuota(ctx context.Context, userID int64) error {
	if !c.IsActive() || userID <= 0 {
		return nil
	}
	key := fmt.Sprintf("nano:inflight:user:%d", userID)
	return inFlightQuotaReleaseLua.Run(ctx, c.rdb, []string{key}).Err()
}

