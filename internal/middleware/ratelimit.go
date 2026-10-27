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
// otherwise ready to stream. On timeout the middleware falls through to the
// local token bucket, which is the same fail-open path used for a Redis error.
const redisRateLimitTimeout = 300 * time.Millisecond

// tokenBucket tracks request tokens for a key.
type tokenBucket struct {
	mu         sync.Mutex
	tokens     float64
	capacity   float64
	rate       float64 // tokens per second
	lastUpdate time.Time
}

func (tb *tokenBucket) allow() bool {
	tb.mu.Lock()
	defer tb.mu.Unlock()

	now := time.Now()
	elapsed := now.Sub(tb.lastUpdate).Seconds()
	tb.lastUpdate = now

	// Refill tokens
	tb.tokens += elapsed * tb.rate
	if tb.tokens > tb.capacity {
		tb.tokens = tb.capacity
	}

	if tb.tokens >= 1.0 {
		tb.tokens -= 1.0
		return true
	}
	return false
}

// Allow reports whether a token is available and consumes it.
func (tb *tokenBucket) Allow() bool {
	return tb.allow()
}

// Capacity returns current bucket capacity.
func (tb *tokenBucket) Capacity() float64 {
	tb.mu.Lock()
	defer tb.mu.Unlock()
	return tb.capacity
}

// Rate returns current refill rate.
func (tb *tokenBucket) Rate() float64 {
	tb.mu.Lock()
	defer tb.mu.Unlock()
	return tb.rate
}

// RateLimiter manages buckets for all API keys.
type RateLimiter struct {
	mu      sync.RWMutex
	buckets map[string]*tokenBucket
}

var GlobalRateLimiter = &RateLimiter{
	buckets: make(map[string]*tokenBucket),
}

// GetBucket returns or updates token bucket for key.
func (rl *RateLimiter) GetBucket(key string, rpm int) *tokenBucket {
	return rl.getBucket(key, rpm)
}

func (rl *RateLimiter) getBucket(key string, rpm int) *tokenBucket {
	expectedCapacity := float64(rpm)
	expectedRate := float64(rpm) / 60.0

	updateBucket := func(b *tokenBucket) *tokenBucket {
		b.mu.Lock()
		if b.capacity != expectedCapacity || b.rate != expectedRate {
			b.capacity = expectedCapacity
			b.rate = expectedRate
			if b.tokens > expectedCapacity {
				b.tokens = expectedCapacity
			}
		}
		b.mu.Unlock()
		return b
	}

	rl.mu.RLock()
	b, exists := rl.buckets[key]
	rl.mu.RUnlock()
	if exists {
		return updateBucket(b)
	}

	rl.mu.Lock()
	defer rl.mu.Unlock()
	if b, exists = rl.buckets[key]; exists {
		return updateBucket(b)
	}

	b = &tokenBucket{
		tokens:     expectedCapacity,
		capacity:   expectedCapacity,
		rate:       expectedRate,
		lastUpdate: time.Now(),
	}
	rl.buckets[key] = b
	return b
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

// CleanupIdle purges buckets that have not been accessed within maxIdle duration.
func (rl *RateLimiter) CleanupIdle(maxIdle time.Duration) {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now()
	for k, b := range rl.buckets {
		b.mu.Lock()
		idle := now.Sub(b.lastUpdate)
		b.mu.Unlock()
		if idle > maxIdle {
			delete(rl.buckets, k)
		}
	}
}

// RateLimitMiddleware enforces RPM rate limits per API key.
// Priority:
// 1. Enterprise Redis Distributed Sliding Window (if REDIS_URL is configured and reachable)
// 2. Local In-Memory Token Bucket (graceful zero-dependency fallback)
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
		AbortWithDenial(c, outcome.Denial)
	}
}
