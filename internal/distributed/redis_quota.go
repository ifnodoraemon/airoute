package distributed

import (
	"context"
	"fmt"
	"time"

	"github.com/ifnodoraemon/airoute/internal/telemetry"
	"github.com/redis/go-redis/v9"
)

// Rate limiting is a strict 60s sliding window, in two implementations that now
// share the same semantics:
//
//   - This package (used whenever Redis is reachable): the window is kept in Redis
//     as a sorted set of admission timestamps, shared by every replica
//     (slidingWindowLua). Any 60s interval holds at most `rpm` admitted requests.
//   - internal/middleware (used when Redis is not configured, and as the fail-open
//     path when a Redis call errors or exceeds redisRateLimitTimeout): the same
//     window kept locally per key, with the same (now-window, now] boundary -
//     evict, then count, then admit (see rpmWindowSpan there, which mirrors
//     rpmWindowMs below).
//
// So a Redis outage only costs the cross-replica view: each node falls back to its
// own window, which is exactly as strict for the traffic that node sees. Quota is
// not loosened during the outage; the only difference is that a key spread across
// several replicas can temporarily be admitted up to `rpm` per replica instead of
// `rpm` cluster-wide, and the count converges again once Redis returns. That is the
// deliberate availability trade-off (fail open rather than reject traffic when the
// shared counter is unreachable).
//
// AllowRPM itself has no local fallback: on a Redis error it returns
// (true, err), i.e. it fails open and leaves the enforcement to the caller's
// local limiter.

const rpmWindowMs int64 = 60_000

// slidingWindowLuaSrc is an atomic Lua script implementing distributed
// sliding-window rate limiting.
//
// It matches the local window in internal/middleware (see the package note at the
// top of this file): the ZCARD check happens before the ZADD, and the eviction
// drops everything at or before now-window, so the count is exactly the number of
// admissions inside the last 60s either way.
const slidingWindowLuaSrc = `
local key = KEYS[1]
local limit = tonumber(ARGV[1])
local now = tonumber(ARGV[2])
local window = tonumber(ARGV[3])
local clearBefore = now - window

redis.call('ZREMRANGEBYSCORE', key, 0, clearBefore)
local current = redis.call('ZCARD', key)
if current < limit then
    local seq = redis.call('INCR', key .. ':seq')
    local member = tostring(now) .. ':' .. tostring(seq)
    redis.call('ZADD', key, now, member)
    redis.call('PEXPIRE', key, window)
    redis.call('PEXPIRE', key .. ':seq', window)
    return 1
else
    return 0
end
`

var slidingWindowLua = redis.NewScript(slidingWindowLuaSrc)

// AllowRPM checks rate limiting using Redis Sliding Window across the entire cluster.
//
// The window is strict: `rpm` admissions per rolling 60s, counted from the
// timestamps of the admissions themselves. Callers that fall back to the local
// limiter on error get the same sliding window per node (see the package note
// above); the limit is not loosened, only the cross-replica view is lost.
func (c *Client) AllowRPM(ctx context.Context, apiKey string, rpm int) (bool, error) {
	if !c.IsActive() || rpm <= 0 {
		return true, nil // unmetered or not active
	}

	key := fmt.Sprintf("airoute:ratelimit:rpm:%s", apiKey)
	nowMs := time.Now().UnixMilli()

	res, err := slidingWindowLua.Run(ctx, c.rdb, []string{key}, rpm, nowMs, rpmWindowMs).Result()
	if err != nil {
		// On Redis failure, fail-open to ensure API availability
		telemetry.ScopedLogger("sync").Warn("redis rate limit query error, failing open to preserve traffic", "error", err.Error())
		return true, err
	}

	allowed, ok := res.(int64)
	if !ok || allowed != 1 {
		return false, nil
	}
	return true, nil
}
