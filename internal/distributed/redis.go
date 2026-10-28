package distributed

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/ifnodoraemon/airoute/internal/telemetry"
	"github.com/redis/go-redis/v9"
)

const (
	ReloadChannel    = "airoute:cluster:reload"
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
		telemetry.ScopedLogger("sync").Info("Redis not configured, running in local memory Zero-Dependency mode")
		globalClient = nil
		return nil
	}

	opt, err := redis.ParseURL(redisURL)
	if err != nil {
		// Never log the raw URL here: it normally carries the Redis password in
		// the userinfo section, and this branch exists precisely because the URL
		// is malformed (so the operator needs the address, not the secret).
		telemetry.ScopedLogger("sync").Warn("failed to parse REDIS_URL, falling back to local memory", "error", err.Error(), "url", redactRedisURL(redisURL))
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
		telemetry.ScopedLogger("sync").Warn("cannot reach Redis server, gracefully falling back to local in-memory operation", "error", err.Error())
		_ = rdb.Close()
		globalClient = nil
		return nil
	}

	telemetry.ScopedLogger("sync").Info("connected to Redis server successfully (Enterprise Distributed Cluster Mode ACTIVATED)",
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

	// The version counter is how peers that missed the pub/sub message heal
	// themselves (see subscribeReloadOnce). A failed INCR does not stop the
	// broadcast - the message is still useful - but it must be visible: without the
	// version bump, a peer that was disconnected during this publish will not
	// notice the change until the next successful publish.
	newVer, versionErr := c.rdb.Incr(ctx, ConfigVersionKey).Result()
	if versionErr != nil {
		telemetry.ScopedLogger("sync").Warn(
			"failed to advance the cluster config version; detached peers may not observe this reload until the next one",
			"error", versionErr.Error(), "reason", reason)
	}

	evt := ClusterEvent{
		Event:     "reload",
		Reason:    reason,
		NodeID:    telemetry.LiveNodeID(),
		Timestamp: time.Now().UnixMicro(),
	}
	payload, err := json.Marshal(evt)
	if err != nil {
		return err
	}

	err = c.rdb.Publish(ctx, ReloadChannel, string(payload)).Err()
	telemetry.ScopedLogger("sync").Info("published cluster reload broadcast", "reason", reason, "version", newVer)
	return err
}

// SubscribeReload listens for real-time cache invalidation events from peer cluster nodes
// and periodically reconciles cluster configuration version to heal missed broadcasts.
//
// The subscription is re-established automatically: go-redis closes the channel
// when the connection drops, and the previous implementation simply returned,
// silently degrading the cluster to 10s polling until the process restarted.
func (c *Client) SubscribeReload(onReload func(reason string), stopChan <-chan struct{}) {
	if !c.IsActive() {
		return
	}

	go func() {
		var localVersion int64
		if v, err := c.rdb.Get(context.Background(), ConfigVersionKey).Int64(); err == nil {
			localVersion = v
		}

		backoff := time.Second
		for {
			select {
			case <-stopChan:
				return
			default:
			}

			if !c.subscribeReloadOnce(stopChan, onReload, &localVersion) {
				return
			}

			// Channel closed unexpectedly: resubscribe with capped backoff.
			telemetry.ScopedLogger("sync").Warn("redis cluster reload subscription lost, reconnecting",
				"retry_in", backoff.String())
			select {
			case <-time.After(backoff):
			case <-stopChan:
				return
			}
			if backoff < 30*time.Second {
				backoff *= 2
			}
			if backoff > 30*time.Second {
				backoff = 30 * time.Second
			}
		}
	}()
}

// subscribeReloadOnce runs one subscription lifetime. It returns true when the
// channel ended unexpectedly (caller should reconnect) and false when the
// caller should stop (stopChan closed).
func (c *Client) subscribeReloadOnce(stopChan <-chan struct{}, onReload func(reason string), localVersion *int64) bool {
	pubsub := c.rdb.Subscribe(context.Background(), ReloadChannel)
	defer pubsub.Close()

	ch := pubsub.Channel()
	telemetry.ScopedLogger("sync").Info("subscribed to Redis cluster invalidation channel", "channel", ReloadChannel)

	// Self-healing periodic ticker to catch any dropped pub/sub events
	reconcileTicker := time.NewTicker(15 * time.Second)
	defer reconcileTicker.Stop()

	for {
		select {
		case msg, ok := <-ch:
			if !ok {
				return true
			}
			var evt ClusterEvent
			if err := json.Unmarshal([]byte(msg.Payload), &evt); err == nil {
				// Skip this node's own broadcast: PublishReload runs after the local
				// reload that produced it, so acting on it again would reload twice
				// per write and double the database load for no state change.
				if evt.NodeID != "" && evt.NodeID == telemetry.LiveNodeID() {
					continue
				}
				telemetry.ScopedLogger("sync").Info("received Redis cluster reload broadcast (<1ms latency)", "reason", evt.Reason)
				if v, err := c.rdb.Get(context.Background(), ConfigVersionKey).Int64(); err == nil && v > *localVersion {
					*localVersion = v
				}
				onReload(evt.Reason)
			}
		case <-reconcileTicker.C:
			if v, err := c.rdb.Get(context.Background(), ConfigVersionKey).Int64(); err == nil {
				if v > *localVersion {
					telemetry.ScopedLogger("sync").Info("reconciled cluster config version divergence, triggering self-healing reload", "localVersion", *localVersion, "remoteVersion", v)
					*localVersion = v
					onReload("version_sync_reconciliation")
				}
			}
		case <-stopChan:
			return false
		}
	}
}

// GetSessionChannel retrieves the pinned provider channel name for a sticky session.
func (c *Client) GetSessionChannel(ctx context.Context, model, sessionID string) (string, error) {
	if !c.IsActive() || sessionID == "" {
		return "", nil
	}
	key := fmt.Sprintf("airoute:session:%s:%s", model, sessionID)
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
	key := fmt.Sprintf("airoute:session:%s:%s", model, sessionID)
	return c.rdb.Set(ctx, key, channelName, ttl).Err()
}

// GetRDB returns the underlying redis.Client.
func (c *Client) GetRDB() *redis.Client {
	if c == nil {
		return nil
	}
	return c.rdb
}

// redactRedisURL removes the password from a Redis URL so it can be logged.
//
// The URL is logged when it cannot be parsed, which is exactly the case where
// url.Parse may fail too: the fallback therefore masks the userinfo span by hand
// rather than falling back to the raw string.
func redactRedisURL(raw string) string {
	if raw == "" {
		return raw
	}
	if parsed, err := url.Parse(raw); err == nil && parsed.User != nil {
		if _, hasPassword := parsed.User.Password(); hasPassword {
			parsed.User = url.UserPassword(parsed.User.Username(), "REDACTED")
		}
		return parsed.String()
	}

	schemeEnd := strings.Index(raw, "://")
	hostStart := 0
	if schemeEnd >= 0 {
		hostStart = schemeEnd + 3
	}
	at := strings.LastIndex(raw, "@")
	if at <= hostStart || strings.Contains(raw[hostStart:at], "/") {
		// No userinfo section, or it does not belong to the authority.
		return raw
	}
	return raw[:hostStart] + "REDACTED" + raw[at:]
}
