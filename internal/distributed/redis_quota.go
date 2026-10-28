package distributed

import (
	"context"
	"fmt"
	"math"
	"time"

	"github.com/ifnodoraemon/airoute/internal/telemetry"
	"github.com/redis/go-redis/v9"
)

// Rate limiting exists in two implementations, and they are not equivalent:
//
//   - This package (used whenever Redis is reachable): a strict 60s sliding
//     window. Any 60s interval holds at most `rpm` admitted requests, because
//     every admission is timestamped and counted (slidingWindowLua).
//   - internal/middleware (used when Redis is not configured, and as the
//     fail-open path when a Redis call errors): a token bucket that starts full
//     and refills at rpm/60 per second. An idle key can therefore admit a full
//     `rpm` burst immediately and keep spending refill afterwards, so a 60s
//     interval can hold noticeably more than rpm.
//
// The consequence for operators: a Redis outage does not just downgrade the
// limiter to a local one, it loosens it - keys that already spent their quota in
// the Redis window may be granted a fresh full bucket once the fallback takes
// over, and the excess is only reconciled when Redis returns. That is a
// deliberate availability trade-off (fail open rather than reject traffic when
// the counter is unreachable), not a bug, but it means RPM is a soft limit during
// an outage. Making the two agree would require the local bucket to start empty
// (capacity 1) or to keep a sliding window too; that code lives in the middleware
// package.
//
// AllowRPM itself has no local fallback: on a Redis error it returns
// (true, err), i.e. it fails open and leaves the enforcement to the caller's
// local limiter.

// quotaUnitsPerCNY is the number of integer sub-units (1e-9 CNY) in one CNY.
// Reservations travel as integers so Redis arithmetic cannot drift the way
// repeated float additions would, and so the values line up with the ledger.
const quotaUnitsPerCNY = 1_000_000_000.0

// inflightQuotaTTLSeconds is the safety net for a node that dies between
// reserve and release: a leaked reservation expires instead of blocking the
// wallet forever.
const inflightQuotaTTLSeconds = 120

// rpmWindowMs is the width of the RPM sliding window. It is passed to the Lua
// script rather than hardcoded there so the window this process enforces and the
// window the script evicts on cannot drift apart.
const rpmWindowMs int64 = 60_000

// rpmClearBefore returns the newest timestamp that is already outside the window
// ending at nowMs. ZREMRANGEBYSCORE is inclusive, so a request admitted exactly
// rpmWindowMs ago no longer counts: the window is (now-window, now].
func rpmClearBefore(nowMs int64) int64 {
	return nowMs - rpmWindowMs
}

// slidingWindowLuaSrc is an atomic Lua script implementing distributed
// sliding-window rate limiting.
//
// It is stricter than the local token bucket it falls back to (see the package
// note at the top of this file): the ZCARD check happens before the ZADD, and the
// eviction drops everything at or before now-window, so the count is exactly the
// number of admissions inside the last 60s.
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

// quotaAmountReserveLua is the money-aware admission gate. It books an amount
// reservation and a concurrency slot atomically, and only if both the wallet
// headroom and the per-user in-flight cap allow it:
//
//	KEYS[1] amount key  (sum of pre-authorised 1e-9 units)
//	KEYS[2] count key   (in-flight request count)
//	ARGV[1] balance     (1e-9 units the caller observed for the wallet)
//	ARGV[2] need        (1e-9 units to pre-authorise for this request)
//	ARGV[3] ttl         (seconds, refreshed on every reserve)
//	ARGV[4] maxInflight (>=1 caps concurrent requests; <=0 disables the cap)
//
// Returns 1 when the request is admitted, 0 when either limit rejects it.
const quotaAmountReserveLuaSrc = `
local amountKey = KEYS[1]
local countKey = KEYS[2]
local balance = tonumber(ARGV[1])
local need = tonumber(ARGV[2])
local ttl = tonumber(ARGV[3])
local maxInflight = tonumber(ARGV[4])

local reserved = tonumber(redis.call('GET', amountKey) or '0')
local inflight = tonumber(redis.call('GET', countKey) or '0')

-- Never authorise more than the wallet can cover: balance >= reserved + need.
if balance - reserved - need < 0 then
    return 0
end
if maxInflight > 0 and inflight + 1 > maxInflight then
    return 0
end

redis.call('INCRBY', amountKey, need)
redis.call('INCR', countKey)
redis.call('EXPIRE', amountKey, ttl)
redis.call('EXPIRE', countKey, ttl)
return 1
`

var quotaAmountReserveLua = redis.NewScript(quotaAmountReserveLuaSrc)

// quotaAmountReleaseLua returns a reservation. The amount decrement is clamped
// to what is actually booked, so a release can never drive the aggregate
// negative or eat another request's reservation; the count is clamped to zero
// the same way. Both keys are deleted once empty rather than left at 0.
const quotaAmountReleaseLuaSrc = `
local amountKey = KEYS[1]
local countKey = KEYS[2]
local units = tonumber(ARGV[1])

local reserved = tonumber(redis.call('GET', amountKey) or '0')
if reserved > 0 and units > 0 then
    if units > reserved then
        units = reserved
    end
    local remaining = redis.call('DECRBY', amountKey, units)
    if remaining <= 0 then
        redis.call('DEL', amountKey)
    end
end

local inflight = tonumber(redis.call('GET', countKey) or '0')
if inflight <= 1 then
    redis.call('DEL', countKey)
else
    redis.call('DECR', countKey)
end
return 1
`

var quotaAmountReleaseLua = redis.NewScript(quotaAmountReleaseLuaSrc)

// inFlightQuotaTopUpLua raises an existing reservation up to the dispatch-time
// estimate without changing the in-flight request count.
//
// Admission reserves a coarse amount before the body is parsed; once the model,
// prompt and max_tokens are known the dispatcher tops the same reservation up.
// The add is clamped to what the wallet can still cover (balance - reserved), so
// a top-up can never reserve money the wallet does not have, and it returns the
// number of units actually added (0 when the key expired or no room remains).
var inFlightQuotaTopUpLua = redis.NewScript(`
local key = KEYS[1]
local balance = tonumber(ARGV[1])
local delta = tonumber(ARGV[2])
local ttl = tonumber(ARGV[3])

if redis.call('EXISTS', key) == 0 then
    return 0
end
local current = tonumber(redis.call('GET', key) or '0')
local room = balance - current
local add = delta
if add > room then
    add = room
end
if add <= 0 then
    return 0
end
redis.call('INCRBY', key, add)
redis.call('EXPIRE', key, ttl)
return add
`)

// TopUpQuotaAmount raises userID's in-flight amount reservation by upToUnits,
// clamped so the total never exceeds balanceUnits. It returns the units added.
//
// A missing key (the reservation expired) adds nothing: the request is still
// protected by the balance check it already passed, and recreating the key here
// would book an amount that no release will ever match.
func (c *Client) TopUpQuotaAmount(ctx context.Context, userID int64, balanceUnits, upToUnits int64) (int64, error) {
	if !c.IsActive() || userID <= 0 || upToUnits <= 0 {
		return 0, nil
	}
	key := fmt.Sprintf("airoute:inflight:amt:%d", userID)
	ttl := 120
	res, err := inFlightQuotaTopUpLua.Run(ctx, c.rdb, []string{key}, balanceUnits, upToUnits, ttl).Result()
	if err != nil {
		return 0, err
	}
	added, ok := res.(int64)
	if !ok {
		return 0, nil
	}
	return added, nil
}

// AllowRPM checks rate limiting using Redis Sliding Window across the entire cluster.
//
// The window is strict: `rpm` admissions per rolling 60s, counted from the
// timestamps of the admissions themselves. Callers that fall back to the local
// token bucket on error get a looser limit (see the package note above).
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

// inflightQuotaAmountKeys returns the per-user amount and count keys used by the
// money-aware in-flight reservation. They are separate keys so a crash-expiry on
// one cannot silently release the other.
func inflightQuotaAmountKeys(userID int64) (amountKey, countKey string) {
	return fmt.Sprintf("airoute:inflight:amt:%d", userID), fmt.Sprintf("airoute:inflight:cnt:%d", userID)
}

// quotaBalanceUnits converts a wallet balance in CNY to integer 1e-9 units.
// Balances round to the nearest sub-unit (matching the ledger's own conversion)
// because the balance is a read-only snapshot, while reservations round up.
func quotaBalanceUnits(balanceCNY float64) int64 {
	return quotaAmountUnits(balanceCNY, math.Round)
}

// quotaReserveUnits converts a reservation amount in CNY to integer 1e-9 units,
// rounding up: a reservation must never be smaller than the amount it stands for.
func quotaReserveUnits(reserveCNY float64) int64 {
	if reserveCNY <= 0 || math.IsNaN(reserveCNY) || math.IsInf(reserveCNY, 0) {
		return 0
	}
	units := quotaAmountUnits(reserveCNY, math.Ceil)
	if units < 1 {
		return 1
	}
	return units
}

func quotaAmountUnits(cny float64, round func(float64) float64) int64 {
	if cny <= 0 || math.IsNaN(cny) || math.IsInf(cny, 0) {
		return 0
	}
	units := round(cny * quotaUnitsPerCNY)
	if units > math.MaxInt64 {
		return math.MaxInt64
	}
	return int64(units)
}

// TryReserveQuotaAmount atomically pre-authorises reserveCNY of the user's
// wallet for one in-flight request, enforcing both the amount ceiling
// (balance >= outstanding reservations + this reservation) and the concurrency
// cap (outstanding requests < maxInflight).
//
// It reports (true, nil) when no Redis client is active: the local tracker owns
// the reservation in that case, exactly as it does when this returns an error
// (the caller logs the failure and falls back to the local tracker).
func (c *Client) TryReserveQuotaAmount(ctx context.Context, userID int64, balanceCNY, reserveCNY float64, maxInflight int) (bool, error) {
	if !c.IsActive() || userID <= 0 {
		return true, nil
	}
	amountKey, countKey := inflightQuotaAmountKeys(userID)
	res, err := quotaAmountReserveLua.Run(ctx, c.rdb, []string{amountKey, countKey},
		quotaBalanceUnits(balanceCNY), quotaReserveUnits(reserveCNY), inflightQuotaTTLSeconds, maxInflight).Result()
	if err != nil {
		return false, err
	}
	allowed, ok := res.(int64)
	return ok && allowed == 1, nil
}

// ReleaseQuotaAmount returns units of pre-authorised balance and one in-flight
// slot for the user. It is idempotent in the safe direction: releasing more than
// is booked clamps at zero instead of going negative.
func (c *Client) ReleaseQuotaAmount(ctx context.Context, userID int64, units int64) error {
	if !c.IsActive() || userID <= 0 || units <= 0 {
		return nil
	}
	amountKey, countKey := inflightQuotaAmountKeys(userID)
	return quotaAmountReleaseLua.Run(ctx, c.rdb, []string{amountKey, countKey}, units).Err()
}
