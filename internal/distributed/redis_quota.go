package distributed

import (
	"context"
	"fmt"
	"time"

	"github.com/ifnodoraemon/airoute/internal/telemetry"
	"github.com/redis/go-redis/v9"
)

// slidingWindowLua is an atomic Lua script implementing distributed sliding-window rate limiting.
var slidingWindowLua = redis.NewScript(`
local key = KEYS[1]
local limit = tonumber(ARGV[1])
local now = tonumber(ARGV[2])
local window = 60000 -- 60s in milliseconds
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
`)

// inFlightQuotaAcquireLua atomically checks and reserves in-flight user quota across the cluster.
var inFlightQuotaAcquireLua = redis.NewScript(`
local key = KEYS[1]
local balance = tonumber(ARGV[1])
local minCost = tonumber(ARGV[2])
local ttl = tonumber(ARGV[3])

local current = tonumber(redis.call('GET', key) or '0')
if balance - (current + 1) * minCost < 0 then
    return 0
else
    redis.call('INCR', key)
    redis.call('EXPIRE', key, ttl)
    return 1
end
`)

// inFlightQuotaReleaseLua atomically decrements user's in-flight request counter across the cluster.
var inFlightQuotaReleaseLua = redis.NewScript(`
local key = KEYS[1]
local current = tonumber(redis.call('GET', key) or '0')
if current <= 1 then
    redis.call('DEL', key)
else
    redis.call('DECR', key)
end
return 1
`)

// AllowRPM checks rate limiting using Redis Sliding Window across the entire cluster.
func (c *Client) AllowRPM(ctx context.Context, apiKey string, rpm int) (bool, error) {
	if !c.IsActive() || rpm <= 0 {
		return true, nil // unmetered or not active
	}

	key := fmt.Sprintf("nano:ratelimit:rpm:%s", apiKey)
	nowMs := time.Now().UnixNano() / int64(time.Millisecond)

	res, err := slidingWindowLua.Run(ctx, c.rdb, []string{key}, rpm, nowMs).Result()
	if err != nil {
		// On Redis failure, fail-open to ensure API availability
		telemetry.Logger.Warn("redis rate limit query error, failing open to preserve traffic", "error", err.Error())
		return true, err
	}

	allowed, ok := res.(int64)
	if !ok || allowed != 1 {
		return false, nil
	}
	return true, nil
}

// RecordTokens atomically increments the distributed usage counter for an API key.
func (c *Client) RecordTokens(ctx context.Context, apiKey string, tokens int) error {
	if !c.IsActive() || tokens <= 0 {
		return nil
	}
	key := "nano:keys:tokens"
	return c.rdb.HIncrBy(ctx, key, apiKey, int64(tokens)).Err()
}

// TryAcquireInFlightQuota atomically checks and reserves in-flight user quota across the cluster.
func (c *Client) TryAcquireInFlightQuota(ctx context.Context, userID int64, balance, minReserveCost float64) (bool, error) {
	if !c.IsActive() || userID <= 0 {
		return true, nil
	}
	key := fmt.Sprintf("nano:inflight:user:%d", userID)
	ttl := 120 // 120s safety timeout to avoid permanently trapped in-flight quota if node crashes
	res, err := inFlightQuotaAcquireLua.Run(ctx, c.rdb, []string{key}, balance, minReserveCost, ttl).Result()
	if err != nil {
		return false, err
	}
	val, ok := res.(int64)
	return ok && val == 1, nil
}

// ReleaseInFlightQuota decrements user's in-flight request counter across the cluster.
func (c *Client) ReleaseInFlightQuota(ctx context.Context, userID int64) error {
	if !c.IsActive() || userID <= 0 {
		return nil
	}
	key := fmt.Sprintf("nano:inflight:user:%d", userID)
	return inFlightQuotaReleaseLua.Run(ctx, c.rdb, []string{key}).Err()
}
