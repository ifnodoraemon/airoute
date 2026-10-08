package middleware

import (
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/ifnodoraemon/airoute/internal/distributed"
	"github.com/ifnodoraemon/airoute/internal/model"
)

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
		if !ok || k.RPM <= 0 {
			c.Next()
			return
		}

		// 1. Check Enterprise Distributed Redis cluster rate limit
		client := distributed.GetClient()
		if client != nil && client.IsActive() {
			allowed, err := client.AllowRPM(c.Request.Context(), k.Key, k.RPM)
			if err == nil {
				if !allowed {
					c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{
						"error": gin.H{
							"message": "Cluster rate limit exceeded (RPM limit reached). Please slow down requests.",
							"type":    "rate_limit_error",
							"code":    "rate_limit_exceeded",
						},
					})
					return
				}
				c.Next()
				return
			}
			// On Redis error/timeout, gracefully fall through to local in-memory token bucket
		}

		// 2. Fallback to high-performance local in-memory token bucket
		bucket := GlobalRateLimiter.getBucket(k.Key, k.RPM)
		if !bucket.allow() {
			c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{
				"error": gin.H{
					"message": "Rate limit exceeded (RPM limit reached). Please slow down requests.",
					"type":    "rate_limit_error",
					"code":    "rate_limit_exceeded",
				},
			})
			return
		}

		c.Next()
	}
}
