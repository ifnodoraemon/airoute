package controlplane

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sync"
	"time"

	"github.com/ifnodoraemon/airoute/internal/distributed"
	"github.com/redis/go-redis/v9"
)

// Login attempt stores (local in-memory and shared Redis), extracted from
// login_guard.go to keep the guard logic under the size red line.

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