package middleware

import (
	"context"
	"math"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/ifnodoraemon/airoute/internal/distributed"
	"github.com/ifnodoraemon/airoute/internal/storage"
	"github.com/ifnodoraemon/airoute/internal/telemetry"
)

// Money-aware in-flight admission.
//
// Admission used to answer one question ("is the wallet balance positive?") and
// then hold a fixed 0.0005 CNY per in-flight request. A long-context request can
// cost hundreds of times that, so a burst of concurrent requests was authorised
// against money the wallet did not have; the overdraft floor only hid the debt
// after the service had already been delivered.
//
// The reservation below asks the question the wallet actually cares about:
//
//	balance >= sum of the amounts already pre-authorised + this request's amount
//
// and additionally caps how many requests one user may have in flight. A request
// that cannot be covered is refused with 402 before any upstream work starts,
// which is the correct direction to fail: refuse service, do not give it away.
const (
	// InflightReserveEnv overrides the coarse per-request amount pre-authorised
	// at admission, in CNY. Authentication runs before the request body is parsed
	// (the model, prompt size and max_tokens are not known yet), so this is a
	// conservative fixed estimate; dispatch-time top-up can refine it later.
	InflightReserveEnv = "GATEWAY_INFLIGHT_RESERVE_CNY"
	// MaxInflightEnv overrides the per-user concurrency cap. 0 disables the cap
	// (the amount ceiling still applies).
	MaxInflightEnv = "GATEWAY_MAX_INFLIGHT_PER_USER"

	// DefaultInflightReserveCNY is ~35x the old 0.0005 CNY reservation and 2-7x
	// the recent mean request cost, while staying small enough not to reject
	// ordinary low-balance traffic.
	DefaultInflightReserveCNY = 0.05
	// DefaultMaxInflightPerUser allows a normal burst but bounds the concurrent
	// exposure of one account.
	DefaultMaxInflightPerUser = 64

	// inflightQuotaTimeout bounds the Redis round trip. A stalled Redis must not
	// pin the request goroutine; the local tracker takes over instead.
	inflightQuotaTimeout = 300 * time.Millisecond
)

var (
	inflightReserveCNY = parseInflightReserveCNY(os.Getenv(InflightReserveEnv))
	maxInflightPerUser = parseMaxInflightPerUser(os.Getenv(MaxInflightEnv))
)

// InflightReserveCNY is the per-request amount pre-authorised at admission.
func InflightReserveCNY() float64 { return inflightReserveCNY }

func parseInflightReserveCNY(raw string) float64 {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return DefaultInflightReserveCNY
	}
	amount, err := strconv.ParseFloat(raw, 64)
	if err != nil || math.IsNaN(amount) || math.IsInf(amount, 0) || amount <= 0 {
		telemetry.ScopedLogger("sync").Warn("ignoring invalid "+InflightReserveEnv+", using default",
			"value", raw, "default_cny", DefaultInflightReserveCNY)
		return DefaultInflightReserveCNY
	}
	return amount
}

func parseMaxInflightPerUser(raw string) int {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return DefaultMaxInflightPerUser
	}
	limit, err := strconv.Atoi(raw)
	if err != nil || limit < 0 {
		telemetry.ScopedLogger("sync").Warn("ignoring invalid "+MaxInflightEnv+", using default",
			"value", raw, "default", DefaultMaxInflightPerUser)
		return DefaultMaxInflightPerUser
	}
	return limit
}

// localQuotaUsage is the local fallback's view of one user's outstanding
// reservations: the amount still pre-authorised plus the number of requests
// holding it.
type localQuotaUsage struct {
	amountUnits int64
	count       int
}

// UserQuotaTracker is the per-process fallback used when Redis is unavailable
// (and in tests / single-node deployments). It enforces the same amount and
// concurrency rules as the distributed Lua gate using integer sub-units.
type UserQuotaTracker struct {
	mu       sync.Mutex
	inFlight map[int64]*localQuotaUsage
}

var globalQuotaTracker = &UserQuotaTracker{
	inFlight: make(map[int64]*localQuotaUsage),
}

// QuotaBackend identifies where an in-flight reservation was taken, so the
// matching release always targets the same counter.
type QuotaBackend string

const (
	QuotaBackendNone  QuotaBackend = ""
	QuotaBackendRedis QuotaBackend = "redis"
	QuotaBackendLocal QuotaBackend = "local"
)

// ReserveUserQuotaAmount pre-authorises reserve CNY against userID's wallet for
// one in-flight request.
//
// It returns the backend that granted the reservation; the caller must pass it
// (and the same amount) to ReleaseUserQuotaAmount. A false result means either
// the wallet cannot cover the outstanding reservations plus this one, or the
// user already has the maximum number of requests in flight.
//
// Redis is authoritative when active; a Redis error logs a warning and falls
// back to the local tracker, preserving the gateway's fail-open availability
// posture while still enforcing the limits within this process.
func ReserveUserQuotaAmount(userID int64, balance, reserve float64) (bool, QuotaBackend) {
	if userID <= 0 {
		return true, QuotaBackendNone
	}
	if reserve <= 0 {
		reserve = inflightReserveCNY
	}

	if rClient := distributed.GetClient(); rClient != nil && rClient.IsActive() {
		ctx, cancel := context.WithTimeout(context.Background(), inflightQuotaTimeout)
		acquired, err := rClient.TryReserveQuotaAmount(ctx, userID, balance, reserve, maxInflightPerUser)
		cancel()
		if err == nil {
			if !acquired {
				return false, QuotaBackendNone
			}
			addReservedUnits(reserveQuotaUnits(reserve))
			return true, QuotaBackendRedis
		}
		telemetry.ScopedLogger("sync").Warn("redis in-flight reservation failed, falling back to local quota tracking",
			"error", err.Error(), "user_id", userID)
	}

	acquired, units := globalQuotaTracker.reserveLocal(userID, balance, reserve, maxInflightPerUser)
	if !acquired {
		return false, QuotaBackendNone
	}
	addReservedUnits(units)
	return true, QuotaBackendLocal
}

// ReleaseUserQuotaAmount returns the pre-authorisation taken by
// ReserveUserQuotaAmount for one request.
//
// amount is the amount to release. The caller passes the actual settled cost
// when it is larger than the amount that was booked (an unexpected overrun must
// still leave the in-flight ledger); it must never pass less than the booked
// amount, because the aggregate key holds the sum of reservations and releasing
// less would leak the difference against the wallet permanently.
//
// The BillingReservedUnits gauge is decremented by exactly what the reservation
// added, on both backends, so the reserve/release pairs stay balanced even when
// the Redis call fails (the Redis key then expires on its TTL).
func ReleaseUserQuotaAmount(userID int64, backend QuotaBackend, amount float64) {
	if userID <= 0 || backend == QuotaBackendNone {
		return
	}
	units := reserveQuotaUnits(amount)
	if units <= 0 {
		units = reserveQuotaUnits(inflightReserveCNY)
	}

	switch backend {
	case QuotaBackendRedis:
		if rClient := distributed.GetClient(); rClient != nil && rClient.IsActive() {
			ctx, cancel := context.WithTimeout(context.Background(), inflightQuotaTimeout)
			err := rClient.ReleaseQuotaAmount(ctx, userID, units)
			cancel()
			if err != nil {
				// The reservation is not lost: the Redis key expires on its TTL.
				// The gauge is still decremented below, because this request is no
				// longer in flight from this process's point of view.
				telemetry.ScopedLogger("sync").Warn("redis in-flight reservation release failed; key will expire on TTL",
					"error", err.Error(), "user_id", userID)
			}
			addReservedUnits(-units)
			return
		}
		// Redis went away between reserve and release: the key expires on its TTL.
		addReservedUnits(-units)
	case QuotaBackendLocal:
		released := globalQuotaTracker.releaseLocal(userID, units)
		addReservedUnits(-released)
	}
}

// reserveLocal books a reservation in the local tracker. It returns the number
// of units booked so the caller can mirror the booking in the telemetry gauge
// with exactly the same integer.
func (t *UserQuotaTracker) reserveLocal(userID int64, balance, reserve float64, maxInflight int) (bool, int64) {
	needUnits := reserveQuotaUnits(reserve)
	balanceUnits := storage.ToUnits(balance)

	t.mu.Lock()
	defer t.mu.Unlock()
	if t.inFlight == nil {
		t.inFlight = make(map[int64]*localQuotaUsage)
	}

	var reserved int64
	var count int
	if usage := t.inFlight[userID]; usage != nil {
		reserved = usage.amountUnits
		count = usage.count
	}
	// balance >= reserved + need, the same rule the Lua script applies.
	if balanceUnits-reserved-needUnits < 0 {
		return false, 0
	}
	if maxInflight > 0 && count+1 > maxInflight {
		return false, 0
	}

	usage := t.inFlight[userID]
	if usage == nil {
		usage = &localQuotaUsage{}
		t.inFlight[userID] = usage
	}
	usage.amountUnits += needUnits
	usage.count++
	return true, needUnits
}

// topUpLocal raises one user's local amount reservation by upToUnits, clamped so
// the total never exceeds the wallet balance. It returns the units added so the
// caller can mirror the booking in the telemetry gauge exactly.
func (t *UserQuotaTracker) topUpLocal(userID int64, balance float64, upToUnits int64) int64 {
	if userID <= 0 || upToUnits <= 0 {
		return 0
	}
	balanceUnits := storage.ToUnits(balance)

	t.mu.Lock()
	defer t.mu.Unlock()
	usage := t.inFlight[userID]
	if usage == nil {
		// The reservation this top-up belongs to is gone (released or expired):
		// booking a fresh amount would never be released.
		return 0
	}
	room := balanceUnits - usage.amountUnits
	if room <= 0 {
		return 0
	}
	add := upToUnits
	if add > room {
		add = room
	}
	usage.amountUnits += add
	return add
}

// releaseLocal returns the units actually released, which is the booking size
// clamped to what the local tracker still holds for the user. Released units can
// be fewer than requested if a reservation expired or was never booked, so the
// caller must decrement the gauge by this value, not by the request.
func (t *UserQuotaTracker) releaseLocal(userID int64, units int64) int64 {
	if units <= 0 {
		return 0
	}
	t.mu.Lock()
	defer t.mu.Unlock()

	usage := t.inFlight[userID]
	if usage == nil {
		return 0
	}
	if units > usage.amountUnits {
		units = usage.amountUnits
	}
	usage.amountUnits -= units
	if usage.count > 0 {
		usage.count--
	}
	if usage.amountUnits <= 0 && usage.count <= 0 {
		delete(t.inFlight, userID)
	}
	return units
}

// reserveQuotaUnits converts a CNY amount to the ledger's integer 1e-9 units,
// rounding up so a reservation never under-covers the amount it stands for.
// Amounts too large to represent saturate instead of wrapping negative, which
// would turn a huge reservation into a free one.
func reserveQuotaUnits(amountCNY float64) int64 {
	if amountCNY <= 0 || math.IsNaN(amountCNY) || math.IsInf(amountCNY, 0) {
		return 0
	}
	units := math.Ceil(amountCNY * float64(storage.UnitsPerCurrency))
	if units > math.MaxInt64 {
		return math.MaxInt64
	}
	if units < 1 {
		return 1
	}
	return int64(units)
}

// addReservedUnits maintains the BillingReservedUnits gauge, which reports the
// amount currently pre-authorised against wallets. Positive deltas are a plain
// add; negative deltas clamp at zero rather than letting float/settlement
// mismatches drive the gauge negative.
func addReservedUnits(delta int64) {
	if delta == 0 || telemetry.GlobalMetrics == nil {
		return
	}
	gauge := &telemetry.GlobalMetrics.BillingReservedUnits
	if delta > 0 {
		gauge.Add(delta)
		return
	}
	for {
		current := gauge.Load()
		next := current + delta
		if next < 0 {
			next = 0
		}
		if gauge.CompareAndSwap(current, next) {
			return
		}
	}
}
