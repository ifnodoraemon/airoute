package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/ifnodoraemon/airoute/internal/model"
)

// C-12: the local RPM limiter used to be a token bucket that started full. A Redis
// outage therefore handed out a second full window of requests (the cluster window
// had already counted them), and an idle key could burst `rpm` instantly. The local
// implementation is now the same 60s sliding window as
// internal/distributed/redis_quota.go: any 60s interval holds at most `rpm`
// admissions, no more and no fewer.
func TestRPMWindowMatchesRedisSlidingWindow(t *testing.T) {
	start := time.Date(2026, 10, 28, 12, 0, 0, 0, time.UTC)
	w := &rpmWindow{rpm: 5}

	// A full allowance is available immediately - a burst of `rpm` in the same
	// instant is what a sliding window permits (and what the product promises).
	for i := 0; i < 5; i++ {
		if !w.allowAt(start) {
			t.Fatalf("admission %d at t0 was denied; a fresh window must allow all 5", i+1)
		}
	}
	// The 6th within the same window is refused.
	if w.allowAt(start) {
		t.Fatal("6th admission inside the window was allowed, want 429")
	}
	// Still refused shortly before the window rolls over.
	if w.allowAt(start.Add(rpmWindowSpan - time.Second)) {
		t.Fatal("admission one second before the window expires was allowed")
	}
	// Exactly one window later the first admission is at the eviction boundary, so
	// the window has room again (Redis evicts scores <= now-window).
	if !w.allowAt(start.Add(rpmWindowSpan)) {
		t.Fatal("admission at t0+window was denied; the oldest hit must be evicted at the boundary")
	}
	// Recovery is not a one-off: after another full window the rest have expired.
	if !w.allowAt(start.Add(2 * rpmWindowSpan)) {
		t.Fatal("admission two windows later was denied, want the window to have drained")
	}
}

// A hot-updated RPM must apply immediately. Resetting the window is deliberate:
// admissions counted under the old limit would otherwise decide the new limit.
func TestRPMWindowHotUpdateResetsWindow(t *testing.T) {
	rl := &RateLimiter{buckets: make(map[string]*rpmWindow)}
	const key = "hot-update-key"

	w := rl.GetBucket(key, 1)
	if !w.Allow() {
		t.Fatal("first request under 1 RPM should be admitted")
	}
	if w.Allow() {
		t.Fatal("second request under 1 RPM should be throttled")
	}

	// Raise the limit: the new allowance must be usable immediately.
	raised := rl.GetBucket(key, 5)
	for i := 0; i < 5; i++ {
		if !raised.Allow() {
			t.Fatalf("request %d after raising the limit was throttled", i+1)
		}
	}
	if raised.Allow() {
		t.Fatal("6th request after raising the limit was admitted, want the 5 RPM window enforced")
	}
}

// End-to-end through the middleware: 5 admitted + 6th rejected, with the denial
// audited. (Recovery after the window is covered with the injected clock above;
// this test does not sleep for a minute.)
func TestRateLimitMiddlewareAllowsFullBurstThenThrottles(t *testing.T) {
	gin.SetMode(gin.TestMode)

	const digest = "window-test-digest"
	key := &model.APIKeyConfig{Key: digest, Status: "active", RPM: 5}

	engine := gin.New()
	engine.Use(func(c *gin.Context) {
		c.Set(ContextKeyAPIKey, digest)
		c.Set(ContextKeyAPIKeyConfig, key)
		c.Next()
	}, RateLimitMiddleware())
	engine.GET("/v1/models", func(c *gin.Context) { c.Status(http.StatusOK) })

	call := func() int {
		req := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
		rec := httptest.NewRecorder()
		engine.ServeHTTP(rec, req)
		return rec.Code
	}

	for i := 0; i < 5; i++ {
		if got := call(); got != http.StatusOK {
			t.Fatalf("request %d = %d, want 200 (a fresh window allows the full RPM burst)", i+1, got)
		}
	}
	if got := call(); got != http.StatusTooManyRequests {
		t.Fatalf("6th request = %d, want 429", got)
	}
}

// The anti-abuse limiters in internal/controlplane (registration, verification
// codes, redemption) share the same window, so they keep their intended "N per
// minute" burst semantics rather than being spaced out evenly.
func TestRPMWindowKeepsPerMinuteAllowance(t *testing.T) {
	rl := &RateLimiter{buckets: make(map[string]*rpmWindow)}
	w := rl.GetBucket("register:ip:203.0.113.7", 5)
	for i := 0; i < 5; i++ {
		if !w.Allow() {
			t.Fatalf("anti-abuse allowance %d of 5 was denied", i+1)
		}
	}
	if w.Allow() {
		t.Fatal("6th registration from the same IP within the window was allowed")
	}
	// The window is per key: another IP is unaffected.
	other := rl.GetBucket("register:ip:198.51.100.9", 5)
	if !other.Allow() {
		t.Fatal("a different IP was throttled by someone else's window")
	}
}
