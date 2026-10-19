package middleware

import (
	"sync"
	"time"
)

// tokenQuotaBucket tracks per-key token consumption within a sliding minute
// window, implementing the TPM limit that used to be stored but never enforced.
type tokenQuotaBucket struct {
	mu         sync.Mutex
	limit      int
	used       int
	windowFrom time.Time
}

// allow reports whether the bucket can accept `tokens` more tokens without
// exceeding the configured per-minute limit.
func (b *tokenQuotaBucket) allow(tokens int) bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.rollLocked(time.Now())
	if b.limit <= 0 {
		return true
	}
	return b.used+tokens <= b.limit
}

// consume adds tokens to the current minute window.
func (b *tokenQuotaBucket) consume(tokens int) {
	if tokens <= 0 {
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	b.rollLocked(time.Now())
	b.used += tokens
}

// remaining reports the tokens still available in the current window.
func (b *tokenQuotaBucket) remaining() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.rollLocked(time.Now())
	if b.limit <= 0 {
		return 0
	}
	if b.used >= b.limit {
		return 0
	}
	return b.limit - b.used
}

// rollLocked resets the counter once a minute has elapsed.
func (b *tokenQuotaBucket) rollLocked(now time.Time) {
	if now.Sub(b.windowFrom) >= time.Minute {
		b.windowFrom = now
		b.used = 0
	}
}

// TokenQuotaTracker keeps one sliding-window bucket per API key.
type TokenQuotaTracker struct {
	mu      sync.Mutex
	buckets map[string]*tokenQuotaBucket
}

// GlobalTokenQuota is the process-wide TPM tracker.
var GlobalTokenQuota = &TokenQuotaTracker{buckets: make(map[string]*tokenQuotaBucket)}

func (t *TokenQuotaTracker) bucket(key string, limit int) *tokenQuotaBucket {
	t.mu.Lock()
	defer t.mu.Unlock()
	b, ok := t.buckets[key]
	if !ok {
		b = &tokenQuotaBucket{limit: limit, windowFrom: time.Now()}
		t.buckets[key] = b
	} else if b.limit != limit {
		b.mu.Lock()
		b.limit = limit
		b.mu.Unlock()
	}
	return b
}

// Allow reports whether the key may start a request under its TPM budget.
func (t *TokenQuotaTracker) Allow(key string, limit int) bool {
	if limit <= 0 {
		return true
	}
	return t.bucket(key, limit).allow(0)
}

// Consume records token usage after a request completes.
func (t *TokenQuotaTracker) Consume(key string, limit, tokens int) {
	if limit <= 0 || tokens <= 0 {
		return
	}
	t.bucket(key, limit).consume(tokens)
}

// Remaining returns the key's remaining tokens for this minute.
func (t *TokenQuotaTracker) Remaining(key string, limit int) int {
	if limit <= 0 {
		return 0
	}
	return t.bucket(key, limit).remaining()
}

// CleanupIdle drops buckets whose window has expired and holds no usage.
func (t *TokenQuotaTracker) CleanupIdle(maxIdle time.Duration) {
	now := time.Now()
	t.mu.Lock()
	defer t.mu.Unlock()
	for k, b := range t.buckets {
		b.mu.Lock()
		idle := now.Sub(b.windowFrom) > maxIdle && b.used == 0
		b.mu.Unlock()
		if idle {
			delete(t.buckets, k)
		}
	}
}
