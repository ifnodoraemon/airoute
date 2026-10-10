package distributed

import (
	"context"
	"encoding/json"
	"fmt"
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
