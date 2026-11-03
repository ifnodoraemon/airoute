package middleware

import (
	"strconv"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/ifnodoraemon/airoute/internal/model"
)

// redisRateLimitTimeout bounds the cross-replica RPM check. The request context
// carries no deadline of its own, and go-redis retries a failed command three
// times: without this a Redis hiccup added seconds to a request that was
// otherwise ready to stream. On timeout the middleware falls through to the local
// sliding window, which is the same fail-open path used for a Redis error.
const redisRateLimitTimeout = 300 * time.Millisecond

// rpmWindowSpan is the width of the local RPM sliding window.
//
// It deliberately mirrors internal/distributed/redis_quota.go (rpmWindowMs = 60s):
// the window is (now-window, now], i.e. admissions at or before now-window are
// evicted before the count is taken, and a request is admitted only while the
// count is below the limit. The previous local implementation was a token bucket
// that started full and refilled at rpm/60 per second, so the two limiters were
// not equivalent: an idle key could admit a full `rpm` burst instantly, and when a
// Redis error made the gateway fall back the key could spend the Redis window's
// quota a second time. Keeping the same sliding window locally removes that
// discrepancy - the fallback is now a strict as the cluster limiter.
const rpmWindowSpan = time.Minute

// rpmWindow records the admission timestamps of one key inside the trailing
// window. At most `rpm` timestamps are kept (a request is only recorded when the
// window is not full), which bounds memory per key to the configured RPM - the
// same bound the Redis ZSET has.
type rpmWindow struct {
	mu       sync.Mutex
	rpm      int
	hits     []int64 // unix nanoseconds of admitted requests, ascending
	lastSeen time.Time
}

// Allow reports whether a request may proceed and records it when it may.
func (w *rpmWindow) Allow() bool {
	return w.allowAt(time.Now())
}

// allowAt is Allow with an injectable clock so the window arithmetic can be tested
// without sleeping for a minute.
func (w *rpmWindow) allowAt(now time.Time) bool {
	w.mu.Lock()
	defer w.mu.Unlock()

	w.lastSeen = now
	if w.rpm <= 0 {
		return true
	}

	// Evict admissions at or before now-window: ZREMRANGEBYSCORE key 0 clearBefore
	// is inclusive, so the window is (now-window, now].
	cutoff := now.Add(-rpmWindowSpan).UnixNano()
	kept := w.hits[:0]
	for _, ts := range w.hits {
		if ts > cutoff {
			kept = append(kept, ts)
		}
	}
	w.hits = kept

	if len(w.hits) >= w.rpm {
		return false
	}
	w.hits = append(w.hits, now.UnixNano())
	return true
}

// RateLimiter manages one sliding window per key.
type RateLimiter struct {
	mu      sync.RWMutex
	buckets map[string]*rpmWindow
}

var GlobalRateLimiter = &RateLimiter{
	buckets: make(map[string]*rpmWindow),
}

// GetBucket returns the sliding window for key, creating it on first use and
// applying a hot-updated RPM. It is used both by the data-plane RPM fallback and by
// the per-IP/per-email anti-abuse limits (registration, verification codes,
// redemption codes), so all of them share one window semantic.
func (rl *RateLimiter) GetBucket(key string, rpm int) *rpmWindow {
	return rl.getBucket(key, rpm)
}

func (rl *RateLimiter) getBucket(key string, rpm int) *rpmWindow {
	updateWindow := func(w *rpmWindow) *rpmWindow {
		w.mu.Lock()
		if w.rpm != rpm {
			// A hot-updated limit starts a fresh window: keeping admissions counted
			// against the old limit would make the new limit decision depend on the
			// old configuration. Operators expect a raised limit to apply now.
			w.rpm = rpm
			w.hits = nil
		}
		w.lastSeen = time.Now()
		w.mu.Unlock()
		return w
	}

	rl.mu.RLock()
	w, exists := rl.buckets[key]
	rl.mu.RUnlock()
	if exists {
		return updateWindow(w)
	}

	rl.mu.Lock()
	defer rl.mu.Unlock()
	if w, exists = rl.buckets[key]; exists {
		return updateWindow(w)
	}

	w = &rpmWindow{rpm: rpm, lastSeen: time.Now()}
	rl.buckets[key] = w
	return w
}

func init() {
	go func() {
		ticker := time.NewTicker(10 * time.Minute)
		for range ticker.C {
			GlobalRateLimiter.CleanupIdle(15 * time.Minute)
			// The TPM tracker keeps one bucket per API key ever seen; without
			// this sweep the map only ever grows (deleted/rotated keys leak
			// forever).
			GlobalTokenQuota.CleanupIdle(15 * time.Minute)
		}
	}()
}

// CleanupIdle purges windows that have not been accessed within maxIdle duration.
func (rl *RateLimiter) CleanupIdle(maxIdle time.Duration) {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now()
	for k, w := range rl.buckets {
		w.mu.Lock()
		idle := now.Sub(w.lastSeen)
		w.mu.Unlock()
		if idle > maxIdle {
			delete(rl.buckets, k)
		}
	}
}

// RateLimitMiddleware enforces RPM rate limits per API key.
// Priority:
// 1. Enterprise Redis Distributed Sliding Window (if REDIS_URL is configured and reachable)
// 2. Local in-memory sliding window (graceful zero-dependency fallback)
//
// Both implement the same 60s sliding window (see rpmWindowSpan), so a Redis
// failover no longer loosens the limit: any 60s interval still holds at most `rpm`
// admitted requests. A local fallback only loses the cross-replica view, i.e. each
// replica enforces `rpm` on its own share of the traffic while Redis is down.
func RateLimitMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		kAny, exists := c.Get(ContextKeyAPIKeyConfig)
		if !exists {
			c.Next()
			return
		}

		k, ok := kAny.(*model.APIKeyConfig)
		if !ok {
			c.Next()
			return
		}

		// The TPM/RPM rules live in CheckKeyRateLimits so the MCP transport
		// applies the same limits; this handler only renders the outcome.
		outcome := CheckKeyRateLimits(c.Request.Context(), k)
		if outcome.RemainingTPM >= 0 {
			c.Header("X-RateLimit-Remaining-Tokens", strconv.Itoa(outcome.RemainingTPM))
		}
		if outcome.Denial == nil {
			c.Next()
			return
		}
		c.Set(ContextKeyErrorMessage, outcome.Denial.LogMessage)
		for name, value := range outcome.Denial.Headers {
			c.Header(name, value)
		}
		// The 429 is a rejection like any other: the console's request log is built
		// from usage_logs, so without this row a throttled client is invisible even
		// though the middleware's contract promises 402/403/429 stay visible.
		auditDenial(c, outcome.Denial)
		AbortWithDenial(c, outcome.Denial)
	}
}
