package middleware

import (
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/ifnodoraemon/nano-gateway/internal/distributed"
	"github.com/ifnodoraemon/nano-gateway/internal/model"
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

// RateLimiter manages buckets for all virtual keys.
type RateLimiter struct {
	mu      sync.RWMutex
	buckets map[string]*tokenBucket
}

var GlobalRateLimiter = &RateLimiter{
	buckets: make(map[string]*tokenBucket),
}

func (rl *RateLimiter) getBucket(key string, rpm int) *tokenBucket {
	rl.mu.RLock()
	b, exists := rl.buckets[key]
	rl.mu.RUnlock()
	if exists {
		return b
	}

	rl.mu.Lock()
	defer rl.mu.Unlock()
	if b, exists = rl.buckets[key]; exists {
		return b
	}

	rate := float64(rpm) / 60.0
	b = &tokenBucket{
		tokens:     float64(rpm),
		capacity:   float64(rpm),
		rate:       rate,
		lastUpdate: time.Now(),
	}
	rl.buckets[key] = b
	return b
}

// RateLimitMiddleware enforces RPM rate limits per virtual key.
// Priority:
// 1. Enterprise Redis Distributed Sliding Window (if REDIS_URL is configured and reachable)
// 2. Local In-Memory Token Bucket (graceful zero-dependency fallback)
func RateLimitMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		vkAny, exists := c.Get(ContextKeyVirtualKeyConfig)
		if !exists {
			c.Next()
			return
		}

		vk, ok := vkAny.(*model.VirtualKeyConfig)
		if !ok || vk.RPM <= 0 {
			c.Next()
			return
		}

		// 1. Check Enterprise Distributed Redis cluster rate limit
		client := distributed.GetClient()
		if client != nil && client.IsActive() {
			allowed, err := client.AllowRPM(c.Request.Context(), vk.Key, vk.RPM)
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
		bucket := GlobalRateLimiter.getBucket(vk.Key, vk.RPM)
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
