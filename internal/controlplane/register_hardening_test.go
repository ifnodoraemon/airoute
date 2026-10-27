package controlplane

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

// registrationRequest executes the real Register handler for one attempt.
func registrationRequest(t *testing.T, handler *AdminHandler, remoteAddr, username, email, code string) *httptest.ResponseRecorder {
	t.Helper()
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	body := fmt.Sprintf(`{"username":%q,"email":%q,"password":"password123","code":%q}`, username, email, code)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/register", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.RemoteAddr = remoteAddr
	if remoteAddr == "" {
		req.RemoteAddr = "203.0.113.10:1234"
	}
	c.Request = req
	handler.Register(c)
	return w
}

// TestRegistrationGrantOnlyForVerifiedOrOpenMode pins the credit/key decision:
// a registration that has not verified its address must never be granted the
// trial balance or an initial API key.
func TestRegistrationGrantOnlyForVerifiedOrOpenMode(t *testing.T) {
	if registrationGrant(true, false) {
		t.Fatal("unverified registration under a verification-required deployment must not be granted")
	}
	if !registrationGrant(true, true) {
		t.Fatal("verified registration must be granted")
	}
	if !registrationGrant(false, false) {
		t.Fatal("a deployment that disabled verification accepts unverified signups")
	}
}

// TestRegisterRequiresValidCodeWhenVerificationEnabled ensures no account, trial
// credit or API key is minted before the address is verified.
func TestRegisterRequiresValidCodeWhenVerificationEnabled(t *testing.T) {
	t.Setenv("ALLOW_REGISTRATION", "true")
	t.Setenv("REQUIRE_EMAIL_VERIFICATION", "true")
	t.Setenv("INITIAL_USER_BALANCE", "5")

	handler, repo := newSecurityTestHandler(t)
	w := registrationRequest(t, handler, "203.0.113.11:1000", "unverified-user", "unverified@example.com", "")
	if w.Code != http.StatusBadRequest {
		t.Fatalf("registration without a code = %d, want 400 (%s)", w.Code, w.Body.String())
	}
	if user, _ := repo.GetUserByUsername("unverified-user"); user != nil {
		t.Fatal("an unverified registration must not create the account")
	}

	w = registrationRequest(t, handler, "203.0.113.11:1000", "unverified-user", "unverified@example.com", "000000")
	if w.Code != http.StatusBadRequest {
		t.Fatalf("registration with an invalid code = %d, want 400 (%s)", w.Code, w.Body.String())
	}
	if user, _ := repo.GetUserByUsername("unverified-user"); user != nil {
		t.Fatal("an invalid code must not create the account")
	}
}

// TestRegisterRejectsInvalidCodeWhenVerificationOptional pins the new behaviour:
// a code the caller supplied is verified instead of being silently discarded.
func TestRegisterRejectsInvalidCodeWhenVerificationOptional(t *testing.T) {
	t.Setenv("ALLOW_REGISTRATION", "true")
	t.Setenv("REQUIRE_EMAIL_VERIFICATION", "false")
	t.Setenv("INITIAL_USER_BALANCE", "0")

	handler, repo := newSecurityTestHandler(t)
	w := registrationRequest(t, handler, "203.0.113.12:1000", "optional-code-user", "optional@example.com", "123456")
	if w.Code != http.StatusBadRequest {
		t.Fatalf("invalid supplied code = %d, want 400 (%s)", w.Code, w.Body.String())
	}
	if user, _ := repo.GetUserByUsername("optional-code-user"); user != nil {
		t.Fatal("a rejected code must not create the account")
	}
}

// TestRegisterIsRateLimitedPerClient is the A05 regression: registration is the
// only unauthenticated endpoint that mints spendable credit and an API key, so it
// must not be loopable. The address comes from the resolved client, not from a
// caller-supplied header.
func TestRegisterIsRateLimitedPerClient(t *testing.T) {
	t.Setenv("ALLOW_REGISTRATION", "true")
	t.Setenv("REQUIRE_EMAIL_VERIFICATION", "false")
	t.Setenv("INITIAL_USER_BALANCE", "0")

	handler, repo := newSecurityTestHandler(t)
	remoteAddr := "203.0.113.20:1000"

	allowed := 0
	for i := 0; i < registerRatePerIPPerMinute+3; i++ {
		username := fmt.Sprintf("farm-%d", i)
		email := fmt.Sprintf("farm-%d@example.com", i)
		w := registrationRequest(t, handler, remoteAddr, username, email, "")
		switch w.Code {
		case http.StatusOK:
			allowed++
		case http.StatusTooManyRequests:
			if retryAfter := w.Header().Get("Retry-After"); retryAfter == "" {
				t.Fatal("expected Retry-After on the throttled registration")
			}
			if allowed != registerRatePerIPPerMinute {
				t.Fatalf("throttled after %d successful registrations, want %d", allowed, registerRatePerIPPerMinute)
			}
			if user, _ := repo.GetUserByUsername(username); user != nil {
				t.Fatalf("throttled registration still created %q", username)
			}
			return
		default:
			t.Fatalf("registration %d = %d: %s", i, w.Code, w.Body.String())
		}
	}
	t.Fatalf("expected registration to be throttled after %d attempts", registerRatePerIPPerMinute)
}

// TestRegisterIsRateLimitedPerUsername ensures one username cannot be spammed
// from many addresses (the per-address bucket alone would allow that).
func TestRegisterIsRateLimitedPerUsername(t *testing.T) {
	t.Setenv("ALLOW_REGISTRATION", "true")
	t.Setenv("REQUIRE_EMAIL_VERIFICATION", "false")
	t.Setenv("INITIAL_USER_BALANCE", "0")

	handler, _ := newSecurityTestHandler(t)
	throttled := false
	for i := 0; i < registerRatePerUsernamePerMinute+3; i++ {
		w := registrationRequest(t, handler,
			fmt.Sprintf("203.0.113.30:1000"), "repeated-user", fmt.Sprintf("repeated-%d@example.com", i), "")
		if w.Code == http.StatusTooManyRequests {
			throttled = true
			break
		}
		if w.Code != http.StatusBadRequest && w.Code != http.StatusOK {
			t.Fatalf("unexpected status %d: %s", w.Code, w.Body.String())
		}
	}
	if !throttled {
		t.Fatal("repeated registrations for one username were never throttled")
	}
}

// TestRegisterRateLimitIgnoresSpoofedForwardedFor is the A05/A04 join: rotating
// X-Forwarded-For from an untrusted peer must not mint a fresh registration
// bucket.
func TestRegisterRateLimitIgnoresSpoofedForwardedFor(t *testing.T) {
	t.Setenv("ALLOW_REGISTRATION", "true")
	t.Setenv("REQUIRE_EMAIL_VERIFICATION", "false")
	t.Setenv("INITIAL_USER_BALANCE", "0")
	t.Setenv("GATEWAY_TRUSTED_PROXIES", "")

	handler, _ := newSecurityTestHandler(t)
	// Every attempt claims a different forwarded address, but the TCP peer is
	// the same; the bucket key must come from the peer.
	allowed := 0
	for i := 0; i < registerRatePerIPPerMinute+2; i++ {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		body := fmt.Sprintf(`{"username":"xff-farm-%d","email":"xff-farm-%d@example.com","password":"password123"}`, i, i)
		req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/register", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.RemoteAddr = "203.0.113.40:1000"
		req.Header.Set("X-Forwarded-For", fmt.Sprintf("198.51.100.%d", i+1))
		c.Request = req
		handler.Register(c)
		switch w.Code {
		case http.StatusOK:
			allowed++
		case http.StatusTooManyRequests:
			if allowed != registerRatePerIPPerMinute {
				t.Fatalf("spoofed header changed the bucket: %d allowed before throttle", allowed)
			}
			return
		default:
			t.Fatalf("unexpected status %d: %s", w.Code, w.Body.String())
		}
	}
	t.Fatal("spoofed X-Forwarded-For bypassed the registration rate limit")
}

// TestRegisterHonoursTrustedProxyClientAddress makes sure the fix did not break
// the correctly configured deployment: a client identified through a trusted
// proxy is throttled on its own address, not on the proxy address.
func TestRegisterHonoursTrustedProxyClientAddress(t *testing.T) {
	t.Setenv("ALLOW_REGISTRATION", "true")
	t.Setenv("REQUIRE_EMAIL_VERIFICATION", "false")
	t.Setenv("INITIAL_USER_BALANCE", "0")
	t.Setenv("GATEWAY_TRUSTED_PROXIES", "10.0.0.0/8")

	handler, _ := newSecurityTestHandler(t)
	registerAsClient := func(clientIP string, i int) int {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		body := fmt.Sprintf(`{"username":"proxy-farm-%d","email":"proxy-farm-%d@example.com","password":"password123"}`, i, i)
		req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/register", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.RemoteAddr = "10.0.0.2:1000"
		req.Header.Set("X-Forwarded-For", clientIP)
		c.Request = req
		handler.Register(c)
		return w.Code
	}
	for i := 0; i < registerRatePerIPPerMinute; i++ {
		if code := registerAsClient("203.0.113.50", i); code != http.StatusOK {
			t.Fatalf("attempt %d = %d, want 200", i, code)
		}
	}
	if code := registerAsClient("203.0.113.50", 99); code != http.StatusTooManyRequests {
		t.Fatalf("same proxied client = %d, want 429", code)
	}
	// A different client behind the same proxy still has its own bucket.
	if code := registerAsClient("203.0.113.51", 100); code != http.StatusOK {
		t.Fatalf("different proxied client = %d, want 200", code)
	}
}
