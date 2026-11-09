package middleware

import (
	"github.com/ifnodoraemon/airoute/internal/storage"
	"github.com/ifnodoraemon/airoute/internal/telemetry"
	"math"
	"os"
	"strconv"
	"strings"
)

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

func reserveQuotaUnits(amountCNY float64) int64 {
	if amountCNY <= 0 || math.IsNaN(amountCNY) || math.IsInf(amountCNY, 0) {
		return 0
	}
	units := math.Ceil(amountCNY * float64(storage.UnitsPerCurrency))
	if units >= math.MaxInt64 {
		return math.MaxInt64
	}
	if units < 1 {
		return 1
	}
	return int64(units)
}

func isFiniteNonNegative(value float64) bool {
	return value >= 0 && !math.IsNaN(value) && !math.IsInf(value, 0)
}
