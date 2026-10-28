package controlplane

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/ifnodoraemon/airoute/internal/distributed"
	"github.com/ifnodoraemon/airoute/internal/telemetry"
	"github.com/redis/go-redis/v9"
)

// Login throttling constants.
const (
	// maxLoginFailuresPerAccount is the failed-attempt budget for one account.
	// It stays at 5 because it is the account-takeover guard.
	maxLoginFailuresPerAccount = 5
	// maxLoginFailuresPerIP is deliberately larger: the address bucket is a
	// spraying defence across many accounts, and a low per-IP budget would let one
	// noisy client lock out everyone behind a shared egress address.
	maxLoginFailuresPerIP = 20
	loginLockoutWindow    = 15 * time.Minute

	// loginSharedPrefix namespaces the Redis counters.
	loginSharedPrefix = "airoute:login:v1:"
	// loginSharedTimeout bounds each Redis round trip; a slow cluster must not
	// stall the login endpoint.
	loginSharedTimeout = 300 * time.Millisecond
)

// loginAttempt tracks failed password attempts for one account/IP pair.
type loginAttempt struct {
	failures    int
	lockedUntil time.Time
	lastFailure time.Time
}

// loginClient is the identity a login attempt is throttled against.
type loginClient struct {
	Username string
	IP       string
	// IPIsForwarded is true only when IP was taken from a trusted proxy's
	// forwarding header. Without a configured trusted proxy the TCP peer is the
	// proxy itself, so locking that address would let a single attacker lock out
	// every user of the gateway; the account bucket still applies.
	IPIsForwarded bool
}

// loginBucket is one throttle dimension for an attempt.
type loginBucket struct {
	kind    string
	subject string
	limit   int
}

// loginAttemptStore is the counter backend. The local implementation is the
// single-node default; the Redis implementation keeps one budget for the whole
// cluster so a brute-force run cannot spread across replicas.
type loginAttemptStore interface {
	// locked reports whether the bucket is currently locked and for how much
	// longer.
	locked(ctx context.Context, bucket loginBucket) (bool, time.Duration, error)
	// fail records one failed attempt, locking the bucket when its limit is
	// reached.
	fail(ctx context.Context, bucket loginBucket) error
	// reset clears both the failure counter and any lock for the bucket.
	reset(ctx context.Context, bucket loginBucket) error
}

// localLoginStore keeps the original in-process counters.
type localLoginStore struct {
	mu      sync.Mutex
	entries map[string]*loginAttempt
	cleanup time.Time
}

func newLocalLoginStore() *localLoginStore {
	return &localLoginStore{entries: make(map[string]*loginAttempt)}
}

func loginBucketKey(bucket loginBucket) string {
	return bucket.kind + "\x00" + bucket.subject
}

func (s *localLoginStore) entryLocked(key string) *loginAttempt {
	attempt, ok := s.entries[key]
	if !ok {
		attempt = &loginAttempt{}
		s.entries[key] = attempt
	}
	return attempt
}

func (s *localLoginStore) locked(_ context.Context, bucket loginBucket) (bool, time.Duration, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.maybeCleanupLocked()
	attempt := s.entries[loginBucketKey(bucket)]
	if attempt == nil {
		return false, 0, nil
	}
	if remaining := time.Until(attempt.lockedUntil); remaining > 0 {
		return true, remaining, nil
	}
	return false, 0, nil
}

func (s *localLoginStore) fail(_ context.Context, bucket loginBucket) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now()
	attempt := s.entryLocked(loginBucketKey(bucket))
	// Reset the counter if the previous failure is stale.
	if !attempt.lastFailure.IsZero() && now.Sub(attempt.lastFailure) > loginLockoutWindow {
		attempt.failures = 0
	}
	attempt.failures++
	attempt.lastFailure = now
	if attempt.failures >= bucket.limit {
		attempt.lockedUntil = now.Add(loginLockoutWindow)
		attempt.failures = 0
	}
	return nil
}

func (s *localLoginStore) reset(_ context.Context, bucket loginBucket) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.entries, loginBucketKey(bucket))
	return nil
}

// maybeCleanupLocked drops entries that have been idle for a while.
func (s *localLoginStore) maybeCleanupLocked() {
	now := time.Now()
	if now.Sub(s.cleanup) < 5*time.Minute {
		return
	}
	s.cleanup = now
	for key, attempt := range s.entries {
		idle := now.After(attempt.lockedUntil) && now.Sub(attempt.lastFailure) > loginLockoutWindow
		if idle {
			delete(s.entries, key)
		}
	}
}

// redisLoginStore keeps the counters in Redis so every replica shares one
// budget. Both keys expire with the lockout window, so no explicit cleanup is
// needed.
type redisLoginStore struct{}

// activeRedisRDB returns the cluster Redis client, or an error when the
// deployment runs without one (the guard then falls back to local counters).
func activeRedisRDB() (*redis.Client, error) {
	client := distributed.GetClient()
	if client == nil || !client.IsActive() {
		return nil, fmt.Errorf("redis is not active")
	}
	rdb := client.GetRDB()
	if rdb == nil {
		return nil, fmt.Errorf("redis client is unavailable")
	}
	return rdb, nil
}

func (redisLoginStore) locked(ctx context.Context, bucket loginBucket) (bool, time.Duration, error) {
	rdb, err := activeRedisRDB()
	if err != nil {
		return false, 0, err
	}
	ttlCtx, cancel := context.WithTimeout(ctx, loginSharedTimeout)
	defer cancel()
	remaining, err := rdb.TTL(ttlCtx, loginRedisKey(bucket, "lock")).Result()
	if err != nil {
		return false, 0, err
	}
	if remaining > 0 {
		return true, remaining, nil
	}
	return false, 0, nil
}

func (redisLoginStore) fail(ctx context.Context, bucket loginBucket) error {
	rdb, err := activeRedisRDB()
	if err != nil {
		return err
	}
	failCtx, cancel := context.WithTimeout(ctx, loginSharedTimeout)
	defer cancel()

	failKey := loginRedisKey(bucket, "fail")
	pipe := rdb.Pipeline()
	incr := pipe.Incr(failCtx, failKey)
	pipe.Expire(failCtx, failKey, loginLockoutWindow)
	if _, err := pipe.Exec(failCtx); err != nil {
		return err
	}
	if incr.Val() < int64(bucket.limit) {
		return nil
	}
	// Budget spent: lock the bucket and drop the counter so the lock itself
	// defines the remaining window.
	lockCtx, lockCancel := context.WithTimeout(ctx, loginSharedTimeout)
	defer lockCancel()
	lockPipe := rdb.Pipeline()
	lockPipe.Set(lockCtx, loginRedisKey(bucket, "lock"), "1", loginLockoutWindow)
	lockPipe.Del(lockCtx, failKey)
	_, err = lockPipe.Exec(lockCtx)
	return err
}

func (redisLoginStore) reset(ctx context.Context, bucket loginBucket) error {
	rdb, err := activeRedisRDB()
	if err != nil {
		return err
	}
	resetCtx, cancel := context.WithTimeout(ctx, loginSharedTimeout)
	defer cancel()
	return rdb.Del(resetCtx, loginRedisKey(bucket, "fail"), loginRedisKey(bucket, "lock")).Err()
}

// loginRedisKey derives an opaque key so usernames/addresses never appear
// verbatim in Redis (and cannot collide through separator characters).
func loginRedisKey(bucket loginBucket, suffix string) string {
	sum := sha256.Sum256([]byte(bucket.kind + "\x00" + bucket.subject))
	return loginSharedPrefix + bucket.kind + ":" + suffix + ":" + hex.EncodeToString(sum[:16])
}

// loginGuard throttles password guessing per account and (when attributable)
// per client address.
type loginGuard struct {
	local *localLoginStore
	// shared, when non-nil, is consulted first and keeps the counters across
	// replicas. Errors fall back to local counters so a cache outage cannot
	// disable the lockout entirely.
	shared loginAttemptStore
	// warnedShared throttles the Redis-fallback warning to once per process.
	warnedShared atomic.Bool
}

var globalLoginGuard = newLoginGuard()

func newLoginGuard() *loginGuard {
	return &loginGuard{
		local:  newLocalLoginStore(),
		shared: redisLoginStore{},
	}
}

// accountBuckets returns the per-account throttle dimension of an attempt.
//
// It is separate from the per-IP dimension because the two have different reset
// rules: a successful login proves the password of *this account* and may clear
// its own failure counter, while the address budget tracks attempts against many
// accounts and must only age out.
func accountBuckets(client loginClient) []loginBucket {
	return []loginBucket{{
		kind:    "user",
		subject: normalizeLoginSubject(client.Username),
		limit:   maxLoginFailuresPerAccount,
	}}
}

// ipBuckets returns the per-address throttle dimension, which only applies when
// the address is attributable (taken from a trusted proxy's forwarding header).
// Without attribution the peer is the proxy itself, and locking it would let one
// attacker lock out every user behind that proxy.
func ipBuckets(client loginClient) []loginBucket {
	if !client.IPIsForwarded || strings.TrimSpace(client.IP) == "" {
		return nil
	}
	return []loginBucket{{
		kind:    "ip",
		subject: strings.TrimSpace(client.IP),
		limit:   maxLoginFailuresPerIP,
	}}
}

// loginBuckets returns every throttle dimension that applies to this attempt.
func loginBuckets(client loginClient) []loginBucket {
	buckets := accountBuckets(client)
	return append(buckets, ipBuckets(client)...)
}

func normalizeLoginSubject(username string) string {
	return strings.ToLower(strings.TrimSpace(username))
}

// check reports whether the login attempt may proceed and how long the caller
// must wait when it is throttled.
func (g *loginGuard) check(ctx context.Context, client loginClient) (bool, time.Duration) {
	buckets := loginBuckets(client)
	if g.shared != nil {
		var retry time.Duration
		locked := false
		sharedFailed := false
		for _, bucket := range buckets {
			isLocked, remaining, err := g.shared.locked(ctx, bucket)
			if err != nil {
				sharedFailed = true
				break
			}
			if isLocked {
				locked = true
				if remaining > retry {
					retry = remaining
				}
			}
		}
		if !sharedFailed {
			return !locked, retry
		}
		g.warnSharedFallback()
	}
	return g.checkLocal(ctx, buckets)
}

func (g *loginGuard) checkLocal(ctx context.Context, buckets []loginBucket) (bool, time.Duration) {
	var retry time.Duration
	for _, bucket := range buckets {
		locked, remaining, err := g.local.locked(ctx, bucket)
		if err != nil || !locked {
			continue
		}
		if remaining > retry {
			retry = remaining
		}
	}
	return retry == 0, retry
}

// failure records a failed attempt on every applicable bucket.
func (g *loginGuard) failure(ctx context.Context, client loginClient) {
	buckets := loginBuckets(client)
	if g.shared != nil {
		sharedFailed := false
		for _, bucket := range buckets {
			if err := g.shared.fail(ctx, bucket); err != nil {
				sharedFailed = true
				break
			}
		}
		if sharedFailed {
			g.warnSharedFallback()
		}
	}
	// Always keep the local counters current: it is the fallback path, and it also
	// keeps the lockout effective if the shared store fails on a later request.
	for _, bucket := range buckets {
		_ = g.local.fail(ctx, bucket)
	}
}

// success clears the failure counters after a valid login.
//
// Only the account bucket is cleared. The per-IP bucket is a cross-account
// spraying defence, not a property of the account that just logged in: clearing
// it here let anyone who owns a single valid account reset the address budget
// between batches ("19 sprayed attempts, one own-account login, repeat"), turning
// "20 attempts / 15 min per address" into an unbounded number of attempts. The
// address budget now only ages out with loginLockoutWindow.
func (g *loginGuard) success(ctx context.Context, client loginClient) {
	buckets := accountBuckets(client)
	if g.shared != nil {
		for _, bucket := range buckets {
			_ = g.shared.reset(ctx, bucket)
		}
	}
	for _, bucket := range buckets {
		_ = g.local.reset(ctx, bucket)
	}
}

func (g *loginGuard) warnSharedFallback() {
	if g.warnedShared.CompareAndSwap(false, true) {
		telemetry.ScopedLogger("auth").Warn("login throttling fell back to process-local counters because the shared Redis store is unavailable; brute-force budgets are per replica until it recovers")
	}
}
