package controlplane

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/ifnodoraemon/airoute/internal/storage"
)

// Literal budgets keep these tests runnable against the pre-fix revision (where
// the constants had different names); they mirror the production values.
const (
	testLoginAttemptBudget = 5
	testLoginSprayBudget   = 20
)

// newLoginTestServer wires the real Login handler behind a Gin engine.
func newLoginTestServer(t *testing.T) (*AdminHandler, *storage.Repository, *gin.Engine) {
	t.Helper()
	handler, repo := newSecurityTestHandler(t)
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.POST("/api/v1/auth/login", handler.Login)
	return handler, repo, engine
}

func attemptLogin(engine *gin.Engine, remoteAddr, forwardedFor, username, password string) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	body := fmt.Sprintf(`{"username":%q,"password":%q}`, username, password)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.RemoteAddr = remoteAddr
	if forwardedFor != "" {
		req.Header.Set("X-Forwarded-For", forwardedFor)
	}
	engine.ServeHTTP(w, req)
	return w
}

// TestSpoofedForwardedForCannotBypassLoginSprayThrottle is the A04 regression.
//
// Behind the reverse proxy the real client address is appended to
// X-Forwarded-For by the proxy. A caller can prepend anything, so the gateway
// must attribute the right-most untrusted hop. Before the fix the first entry
// was believed, which let a password-spraying client present a fresh address on
// every attempt and never hit the per-address budget.
func TestSpoofedForwardedForCannotBypassLoginSprayThrottle(t *testing.T) {
	t.Setenv("GATEWAY_TRUSTED_PROXIES", "10.0.0.0/8")
	_, repo, engine := newLoginTestServer(t)

	if err := repo.CreateUser(&storage.UserRecord{
		Username: "spray-target", Email: "spray-target@example.com",
		PasswordHash: bcryptHash(t, "correct-password"), Role: "user", Status: "active",
	}); err != nil {
		t.Fatalf("create user: %v", err)
	}

	const (
		proxyPeer = "10.0.0.2:443"
		realIP    = "203.0.113.99"
	)
	// Spray distinct accounts; the forged leading entry rotates, the address the
	// proxy appended does not.
	for i := 0; i < testLoginSprayBudget; i++ {
		forwarded := fmt.Sprintf("10.9.%d.%d, %s", i/250, i%250, realIP)
		w := attemptLogin(engine, proxyPeer, forwarded, fmt.Sprintf("spray-%d", i), "wrong-password")
		if w.Code != http.StatusUnauthorized {
			t.Fatalf("spray attempt %d = %d, want 401 (%s)", i+1, w.Code, w.Body.String())
		}
	}

	// The client address budget is now spent: a valid credential from the same
	// client must be throttled...
	if w := attemptLogin(engine, proxyPeer, "10.9.9.9, "+realIP, "spray-target", "correct-password"); w.Code != http.StatusTooManyRequests {
		t.Fatalf("spraying client = %d, want 429 (%s)", w.Code, w.Body.String())
	} else if retryAfter := w.Header().Get("Retry-After"); retryAfter == "" {
		t.Fatal("expected Retry-After on the throttled login")
	}
	// ...while an unrelated client behind the same proxy is unaffected.
	if w := attemptLogin(engine, proxyPeer, "10.9.9.9, 203.0.113.100", "spray-target", "correct-password"); w.Code != http.StatusOK {
		t.Fatalf("innocent client = %d, want 200 (%s)", w.Code, w.Body.String())
	}
}

// TestLoginLockoutStaysAccountScoped ensures the per-account budget still stops
// password guessing for one account from many addresses (the address budget
// alone would not).
func TestLoginLockoutStaysAccountScoped(t *testing.T) {
	t.Setenv("GATEWAY_TRUSTED_PROXIES", "10.0.0.0/8")
	_, repo, engine := newLoginTestServer(t)

	if err := repo.CreateUser(&storage.UserRecord{
		Username: "account-scoped", Email: "account-scoped@example.com",
		PasswordHash: bcryptHash(t, "correct-password"), Role: "user", Status: "active",
	}); err != nil {
		t.Fatalf("create user: %v", err)
	}

	// Distinct client addresses, same account.
	for i := 0; i < testLoginAttemptBudget; i++ {
		forwarded := fmt.Sprintf("10.9.9.9, 203.0.113.%d", 110+i)
		if w := attemptLogin(engine, "10.0.0.2:443", forwarded, "account-scoped", "wrong-password"); w.Code != http.StatusUnauthorized {
			t.Fatalf("attempt %d = %d, want 401 (%s)", i+1, w.Code, w.Body.String())
		}
	}
	if w := attemptLogin(engine, "10.0.0.2:443", "10.9.9.9, 203.0.113.200", "account-scoped", "correct-password"); w.Code != http.StatusTooManyRequests {
		t.Fatalf("locked account from a fresh address = %d, want 429 (%s)", w.Code, w.Body.String())
	}
}

// TestLoginLockoutIgnoresForwardedForWithoutTrustedProxy pins the safe default:
// without a configured proxy the forwarding header is self-asserted, so it must
// not be able to create fresh buckets.
func TestLoginLockoutIgnoresForwardedForWithoutTrustedProxy(t *testing.T) {
	t.Setenv("GATEWAY_TRUSTED_PROXIES", "")
	_, repo, engine := newLoginTestServer(t)

	if err := repo.CreateUser(&storage.UserRecord{
		Username: "no-proxy-account", Email: "no-proxy@example.com",
		PasswordHash: bcryptHash(t, "correct-password"), Role: "user", Status: "active",
	}); err != nil {
		t.Fatalf("create user: %v", err)
	}

	for i := 0; i < testLoginAttemptBudget; i++ {
		forwarded := fmt.Sprintf("198.51.100.%d", i+1)
		if w := attemptLogin(engine, "172.18.0.5:1234", forwarded, "no-proxy-account", "wrong-password"); w.Code != http.StatusUnauthorized {
			t.Fatalf("attempt %d = %d, want 401", i+1, w.Code)
		}
	}
	// The spoofed header must not rescue the attacker from the account budget.
	if w := attemptLogin(engine, "172.18.0.5:1234", "198.51.100.250", "no-proxy-account", "correct-password"); w.Code != http.StatusTooManyRequests {
		t.Fatalf("spoofed header bypassed the lockout: %d (%s)", w.Code, w.Body.String())
	}
}
