package middleware

import (
	"sync"
	"time"
)

// tokenWindowSpan is the width of the TPM sliding window.
//
// It mirrors rpmWindowSpan (ratelimit.go) on purpose: both limits are a strict
// "any 60s interval holds at most N" window, so a key cannot spend a minute budget
// twice by straddling a window boundary. The previous TPM implementation was a
// fixed window (one `used` counter reset every minute), which let a client burn the
// whole budget at the end of one window and a second whole budget immediately after
// the boundary - 2x the configured TPM in about a second - while the code and the
// docs claimed a sliding window.
const tokenWindowSpan = time.Minute

// tokenUsage is one completed request's token consumption inside the window.
type tokenUsage struct {
	// at is the consumption timestamp in unix nanoseconds.
	at int64
	// tokens is the amount recorded by that request.
	tokens int
}

// tokenQuotaBucket tracks per-key token consumption inside a trailing minute.
//
// Consumption is kept as individual (timestamp, tokens) entries rather than a
// single counter, which is what makes the window slide: an entry stops counting as
// soon as it is older than tokenWindowSpan, instead of the whole counter being
// reset at a fixed boundary. Entries are evicted on every operation, so the slice
// holds at most the requests admitted in the last minute.
type tokenQuotaBucket struct {
	mu       sync.Mutex
	limit    int
	usage    []tokenUsage
	lastSeen time.Time
}

// usedLocked evicts entries at or before now-window and returns the tokens still
// inside the window.
//
// The boundary matches rpmWindow (ratelimit.go) and the Redis ZSET limiter: the
// window is (now-window, now], so consumption exactly one window old no longer
// counts.
func (b *tokenQuotaBucket) usedLocked(now time.Time) int {
	cutoff := now.Add(-tokenWindowSpan).UnixNano()
	kept := b.usage[:0]
	total := 0
	for _, entry := range b.usage {
		if entry.at > cutoff {
			kept = append(kept, entry)
			total += entry.tokens
		}
	}
	b.usage = kept
	return total
}

// allowAt reports whether the bucket may admit one more request under its TPM
// budget.
//
// The check is "used < limit", not "used + 0 <= limit": the request's own tokens
// are only known after it completes (there is no estimate at admission for every
// protocol), so a budget that is already spent must refuse. The previous
// "used + tokens <= limit" with tokens=0 admitted exactly one more request once the
// limit had been reached.
func (b *tokenQuotaBucket) allowAt(now time.Time) bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.lastSeen = now
	if b.limit <= 0 {
		return true
	}
	return b.usedLocked(now) < b.limit
}

// consumeAt records tokens consumed at now.
func (b *tokenQuotaBucket) consumeAt(now time.Time, tokens int) {
	if tokens <= 0 {
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	b.lastSeen = now
	// Evict first so the slice cannot grow past the window's admissions.
	b.usedLocked(now)
	b.usage = append(b.usage, tokenUsage{at: now.UnixNano(), tokens: tokens})
}

// remainingAt reports the tokens still available in the trailing window.
func (b *tokenQuotaBucket) remainingAt(now time.Time) int {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.lastSeen = now
	if b.limit <= 0 {
		return 0
	}
	used := b.usedLocked(now)
	if used >= b.limit {
		return 0
	}
	return b.limit - used
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
	now := time.Now()
	b, ok := t.buckets[key]
	if !ok {
		b = &tokenQuotaBucket{limit: limit, lastSeen: now}
		t.buckets[key] = b
		return b
	}
	b.mu.Lock()
	if b.limit != limit {
		// A hot-updated limit applies immediately. Recorded consumption is kept:
		// the tokens were spent under the previous configuration and still count
		// against the window, exactly like the RPM window keeps its admissions.
		b.limit = limit
	}
	b.lastSeen = now
	b.mu.Unlock()
	return b
}

// Allow reports whether the key may start a request under its TPM budget.
func (t *TokenQuotaTracker) Allow(key string, limit int) bool {
	if limit <= 0 {
		return true
	}
	return t.bucket(key, limit).allowAt(time.Now())
}

// Consume records token usage after a request completes.
func (t *TokenQuotaTracker) Consume(key string, limit, tokens int) {
	if limit <= 0 || tokens <= 0 {
		return
	}
	t.bucket(key, limit).consumeAt(time.Now(), tokens)
}

// Remaining returns the key's remaining tokens in the trailing window.
func (t *TokenQuotaTracker) Remaining(key string, limit int) int {
	if limit <= 0 {
		return 0
	}
	return t.bucket(key, limit).remainingAt(time.Now())
}

// CleanupIdle drops buckets that have not been touched within maxIdle. A bucket
// idle for longer than the window holds no live consumption, so dropping it cannot
// re-grant budget that was actually spent.
func (t *TokenQuotaTracker) CleanupIdle(maxIdle time.Duration) {
	now := time.Now()
	t.mu.Lock()
	defer t.mu.Unlock()
	for k, b := range t.buckets {
		b.mu.Lock()
		idle := now.Sub(b.lastSeen) > maxIdle
		b.mu.Unlock()
		if idle {
			delete(t.buckets, k)
		}
	}
}
