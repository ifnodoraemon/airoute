package middleware

import (
	"math"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/ifnodoraemon/airoute/internal/telemetry"
)

// resetQuotaTrackerForTest installs an isolated tracker and the admission
// settings a test needs, restoring the process-wide state afterwards. The Redis
// client is never initialised in this package's tests, so every reservation here
// exercises the local fallback.
func resetQuotaTrackerForTest(t *testing.T, reserve float64, maxInflight int) {
	t.Helper()
	previousReserve, previousMax := inflightReserveCNY, maxInflightPerUser
	previousTracker := globalQuotaTracker
	inflightReserveCNY, maxInflightPerUser = reserve, maxInflight
	globalQuotaTracker = &UserQuotaTracker{inFlight: make(map[int64]*localQuotaUsage)}
	t.Cleanup(func() {
		inflightReserveCNY, maxInflightPerUser = previousReserve, previousMax
		globalQuotaTracker = previousTracker
	})
}

// reservedUnitsGauge reads the shared pre-authorisation gauge. Assertions use
// deltas because the gauge is process-wide.
func reservedUnitsGauge() int64 {
	return telemetry.GlobalMetrics.BillingReservedUnits.Load()
}

func TestReserveUserQuotaAmountRejectsUncoveredBalance(t *testing.T) {
	resetQuotaTrackerForTest(t, 0.05, 0)
	gaugeBefore := reservedUnitsGauge()
	userID := int64(9_100_001)
	const (
		balance = 0.12
		reserve = 0.05
	)

	// 0.12 covers two 0.05 reservations, leaving 0.02; a third reservation would
	// promise money the wallet does not have.
	for i := 0; i < 2; i++ {
		acquired, backend := ReserveUserQuotaAmount(userID, balance, reserve)
		if !acquired || backend != QuotaBackendLocal {
			t.Fatalf("reservation %d = (%v, %q), want (true, %q)", i+1, acquired, backend, QuotaBackendLocal)
		}
	}

	acquired, backend := ReserveUserQuotaAmount(userID, balance, reserve)
	if acquired {
		t.Fatalf("third reservation was admitted against a 0.12 wallet already holding 0.10 of reservations")
	}
	if backend != QuotaBackendNone {
		t.Fatalf("rejected reservation reported backend %q, want none", backend)
	}
	if got, want := reservedUnitsGauge()-gaugeBefore, 2*reserveQuotaUnits(reserve); got != want {
		t.Fatalf("reserved gauge delta = %d, want %d", got, want)
	}

	// Releasing one reservation restores the headroom, so a retry is admitted.
	ReleaseUserQuotaAmount(userID, QuotaBackendLocal, reserve)
	if got, want := reservedUnitsGauge()-gaugeBefore, reserveQuotaUnits(reserve); got != want {
		t.Fatalf("gauge delta after release = %d, want %d", got, want)
	}
	if acquired, _ := ReserveUserQuotaAmount(userID, balance, reserve); !acquired {
		t.Fatalf("request should be admitted after a release restored wallet headroom")
	}

	// Leave the shared gauge flat for the rest of the package's tests.
	ReleaseUserQuotaAmount(userID, QuotaBackendLocal, reserve)
	ReleaseUserQuotaAmount(userID, QuotaBackendLocal, reserve)
	if got := reservedUnitsGauge() - gaugeBefore; got != 0 {
		t.Fatalf("gauge not restored, delta = %d", got)
	}
}

func TestReserveUserQuotaAmountRejectsAboveInflightCap(t *testing.T) {
	resetQuotaTrackerForTest(t, 0.05, 2)
	gaugeBefore := reservedUnitsGauge()
	userID := int64(9_100_002)

	// The wallet could cover thousands of requests; only the concurrency cap may
	// reject here, which is what makes this a cap test rather than a balance test.
	for i := 0; i < 2; i++ {
		if acquired, _ := ReserveUserQuotaAmount(userID, 1_000, 0.05); !acquired {
			t.Fatalf("reservation %d within the cap was rejected", i+1)
		}
	}
	acquired, backend := ReserveUserQuotaAmount(userID, 1_000, 0.05)
	if acquired {
		t.Fatalf("third concurrent request was admitted with maxInflight=2")
	}
	if backend != QuotaBackendNone {
		t.Fatalf("rejected reservation reported backend %q, want none", backend)
	}

	// A finished request frees a slot.
	ReleaseUserQuotaAmount(userID, QuotaBackendLocal, 0.05)
	if acquired, _ := ReserveUserQuotaAmount(userID, 1_000, 0.05); !acquired {
		t.Fatalf("request should be admitted after a slot was released")
	}
	ReleaseUserQuotaAmount(userID, QuotaBackendLocal, 0.05)
	ReleaseUserQuotaAmount(userID, QuotaBackendLocal, 0.05)

	if got := reservedUnitsGauge() - gaugeBefore; got != 0 {
		t.Fatalf("gauge not restored, delta = %d", got)
	}
}

func TestReleaseUserQuotaAmountNeverDoubleDecrements(t *testing.T) {
	resetQuotaTrackerForTest(t, 0.05, 0)
	gaugeBefore := reservedUnitsGauge()
	userID := int64(9_100_003)

	if acquired, _ := ReserveUserQuotaAmount(userID, 1, 0.05); !acquired {
		t.Fatalf("reservation was rejected")
	}
	ReleaseUserQuotaAmount(userID, QuotaBackendLocal, 0.05)
	if got := reservedUnitsGauge() - gaugeBefore; got != 0 {
		t.Fatalf("gauge delta after paired release = %d, want 0", got)
	}

	// A duplicate release (retry, or a defer running twice) has nothing left to
	// return: it must not drive the gauge or the tracker negative.
	ReleaseUserQuotaAmount(userID, QuotaBackendLocal, 0.05)
	ReleaseUserQuotaAmount(userID, QuotaBackendLocal, 0.05)
	if got := reservedUnitsGauge() - gaugeBefore; got != 0 {
		t.Fatalf("gauge delta after duplicate releases = %d, want 0", got)
	}
}

func TestReserveUserQuotaAmountUsesConfiguredDefaultWhenUnset(t *testing.T) {
	resetQuotaTrackerForTest(t, 0.05, 0)
	gaugeBefore := reservedUnitsGauge()
	userID := int64(9_100_004)

	for _, reserve := range []float64{0, -1} {
		if acquired, _ := ReserveUserQuotaAmount(userID, 1, reserve); !acquired {
			t.Fatalf("reserve=%v: reservation was rejected", reserve)
		}
		ReleaseUserQuotaAmount(userID, QuotaBackendLocal, reserve)
	}
	if got := reservedUnitsGauge() - gaugeBefore; got != 0 {
		t.Fatalf("gauge delta = %d, want 0", got)
	}
}

func TestReserveUserQuotaAmountSkipsOwnerlessRequests(t *testing.T) {
	resetQuotaTrackerForTest(t, 0.05, 0)
	gaugeBefore := reservedUnitsGauge()

	acquired, backend := ReserveUserQuotaAmount(0, 0, 0.05)
	if !acquired || backend != QuotaBackendNone {
		t.Fatalf("userID 0 = (%v, %q), want (true, none)", acquired, backend)
	}
	ReleaseUserQuotaAmount(0, QuotaBackendNone, 0.05)
	ReleaseUserQuotaAmount(1, QuotaBackendNone, 0.05)
	if got := reservedUnitsGauge() - gaugeBefore; got != 0 {
		t.Fatalf("gauge delta = %d, want 0", got)
	}
}

func TestReserveUserQuotaAmountConcurrentBurstRespectsAmountCeiling(t *testing.T) {
	resetQuotaTrackerForTest(t, 0.05, 0)
	gaugeBefore := reservedUnitsGauge()
	userID := int64(9_100_006)

	const (
		balance    = 1.0
		reserve    = 0.05
		goroutines = 50
		want       = 20 // 1.0 / 0.05, and a 1e9-unit balance divides exactly
	)

	var admitted atomic.Int64
	var wg sync.WaitGroup
	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if acquired, _ := ReserveUserQuotaAmount(userID, balance, reserve); acquired {
				admitted.Add(1)
			}
		}()
	}
	wg.Wait()

	if got := admitted.Load(); got != want {
		t.Fatalf("admitted %d concurrent reservations, want %d (balance %v / reserve %v)", got, want, balance, reserve)
	}
	if got, wantUnits := reservedUnitsGauge()-gaugeBefore, int64(want)*reserveQuotaUnits(reserve); got != wantUnits {
		t.Fatalf("gauge delta = %d, want %d", got, wantUnits)
	}

	for i := int64(0); i < admitted.Load(); i++ {
		ReleaseUserQuotaAmount(userID, QuotaBackendLocal, reserve)
	}
	if got := reservedUnitsGauge() - gaugeBefore; got != 0 {
		t.Fatalf("gauge not restored, delta = %d", got)
	}
}

func TestSetSettledCostIsReadableDuringTheRequest(t *testing.T) {
	gin.SetMode(gin.TestMode)
	context, _ := gin.CreateTestContext(httptest.NewRecorder())

	if _, ok := SettledCost(context); ok {
		t.Fatalf("fresh context reported a settled cost")
	}

	SetSettledCost(context, 0.031)
	settled, ok := SettledCost(context)
	if !ok || settled != 0.031 {
		t.Fatalf("SettledCost = (%v, %v), want (0.031, true)", settled, ok)
	}
}

func TestSetSettledCostIgnoresInvalidValues(t *testing.T) {
	gin.SetMode(gin.TestMode)

	for name, cost := range map[string]float64{
		"negative":     -1,
		"negative_inf": math.Inf(-1),
		"NaN":          math.NaN(),
		"positive_inf": math.Inf(1),
	} {
		t.Run(name, func(t *testing.T) {
			context, _ := gin.CreateTestContext(httptest.NewRecorder())
			SetSettledCost(context, cost)
			if _, ok := SettledCost(context); ok {
				t.Fatalf("invalid cost %v was accepted", cost)
			}
		})
	}

	// A nil context must be a no-op, not a panic: callers report settlement from
	// paths that may not have a Gin context.
	SetSettledCost(nil, 1)
	if _, ok := SettledCost(nil); ok {
		t.Fatalf("nil context reported a settled cost")
	}
}

func TestSettleReleaseAmountPrefersSettledCostOnlyWhenHigher(t *testing.T) {
	gin.SetMode(gin.TestMode)
	const reserve = 0.05

	// No settlement reported: return the reservation unchanged.
	context, _ := gin.CreateTestContext(httptest.NewRecorder())
	if got := settleReleaseAmount(context, reserve); got != reserve {
		t.Fatalf("no settled cost: release = %v, want %v", got, reserve)
	}

	// A settled cost larger than the reservation: the overrun is acknowledged.
	SetSettledCost(context, 0.37)
	if got := settleReleaseAmount(context, reserve); got != 0.37 {
		t.Fatalf("overrun: release = %v, want 0.37", got)
	}

	// A settled cost below the reservation must NOT shrink the release: the
	// distributed counter holds bookings, so releasing less would leak the
	// difference against the wallet on every request.
	context, _ = gin.CreateTestContext(httptest.NewRecorder())
	SetSettledCost(context, 0.003)
	if got := settleReleaseAmount(context, reserve); got != reserve {
		t.Fatalf("underrun: release = %v, want %v", got, reserve)
	}
}

func TestParseInflightAdmissionSettings(t *testing.T) {
	if got := parseInflightReserveCNY(""); got != DefaultInflightReserveCNY {
		t.Fatalf("empty reserve = %v, want %v", got, DefaultInflightReserveCNY)
	}
	if got := parseInflightReserveCNY("0.2"); got != 0.2 {
		t.Fatalf("reserve = %v, want 0.2", got)
	}
	for _, invalid := range []string{"abc", "0", "-1", "NaN", "+Inf"} {
		if got := parseInflightReserveCNY(invalid); got != DefaultInflightReserveCNY {
			t.Fatalf("invalid reserve %q = %v, want default %v", invalid, got, DefaultInflightReserveCNY)
		}
	}

	if got := parseMaxInflightPerUser(""); got != DefaultMaxInflightPerUser {
		t.Fatalf("empty cap = %v, want %v", got, DefaultMaxInflightPerUser)
	}
	if got := parseMaxInflightPerUser("8"); got != 8 {
		t.Fatalf("cap = %v, want 8", got)
	}
	if got := parseMaxInflightPerUser("0"); got != 0 {
		t.Fatalf("cap 0 = %v, want 0 (uncapped)", got)
	}
	for _, invalid := range []string{"abc", "-3", "1.5"} {
		if got := parseMaxInflightPerUser(invalid); got != DefaultMaxInflightPerUser {
			t.Fatalf("invalid cap %q = %v, want default %v", invalid, got, DefaultMaxInflightPerUser)
		}
	}
}
