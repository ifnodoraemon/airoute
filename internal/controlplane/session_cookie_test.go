package controlplane

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

// B-04 regression suite: the Secure flag of the session cookies is decided from
// something the gateway can trust (TLS, or a scheme forwarded by a configured
// trusted proxy), never from a header an arbitrary caller wrote.

func sessionTestContext(t *testing.T, target string, peer string, forwardedProto string) (*gin.Context, *httptest.ResponseRecorder) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	req := httptest.NewRequest(http.MethodPost, target, nil)
	req.RemoteAddr = peer
	if forwardedProto != "" {
		req.Header.Set("X-Forwarded-Proto", forwardedProto)
	}
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = req
	return c, rec
}

func cookieAttrs(setCookie string) []string {
	parts := strings.Split(setCookie, ";")
	attrs := make([]string, 0, len(parts))
	for _, part := range parts {
		if attr := strings.TrimSpace(part); attr != "" {
			attrs = append(attrs, attr)
		}
	}
	return attrs
}

func cookieHasSecure(t *testing.T, rec *httptest.ResponseRecorder) bool {
	t.Helper()
	cookies := rec.Header().Values("Set-Cookie")
	if len(cookies) == 0 {
		t.Fatal("no Set-Cookie headers were written")
	}
	// Every Set-Cookie header written by SetSessionCookies/ClearSessionCookies must
	// agree on the flag: a session and CSRF cookie pair that disagree is a bug in
	// itself, and browsers would treat the pair inconsistently.
	secure := false
	for _, setCookie := range cookies {
		hasSecure := false
		for _, attr := range cookieAttrs(setCookie) {
			if strings.EqualFold(attr, "Secure") {
				hasSecure = true
			}
		}
		if !hasSecure {
			return false
		}
		secure = true
	}
	return secure
}

func TestSessionCookieSecureFollowsTrustNotClientHeader(t *testing.T) {
	// The shipped default: no trusted proxy configured. A caller can send any
	// X-Forwarded-Proto it likes; the gateway must not act on it.
	t.Setenv("GATEWAY_TRUSTED_PROXIES", "")
	t.Setenv("GATEWAY_ADMIN_SECRET", "")

	t.Run("untrusted peer cannot claim https", func(t *testing.T) {
		c, rec := sessionTestContext(t, "http://gateway.local/api/v1/auth/login", "203.0.113.9:5000", "https")
		if cookieSecure(c) {
			t.Fatal("an untrusted peer's X-Forwarded-Proto: https was believed")
		}
		SetSessionCookies(c, "token")
		if cookieHasSecure(t, rec) {
			t.Fatal("session cookie was issued with Secure based on an untrusted header")
		}
	})

	t.Run("untrusted peer cannot downgrade either", func(t *testing.T) {
		c, _ := sessionTestContext(t, "http://gateway.local/api/v1/auth/login", "203.0.113.9:5000", "http")
		if cookieSecure(c) {
			t.Fatal("unexpected Secure decision for an untrusted plain-http peer")
		}
	})

	t.Run("no TLS and no trusted proxy is not secure", func(t *testing.T) {
		c, _ := sessionTestContext(t, "http://gateway.local/api/v1/auth/login", "198.51.100.4:5000", "")
		if cookieSecure(c) {
			t.Fatal("cookie marked Secure without TLS or a trusted proxy")
		}
	})

	t.Run("direct TLS is secure", func(t *testing.T) {
		c, rec := sessionTestContext(t, "https://gateway.local/api/v1/auth/login", "203.0.113.9:5000", "")
		if !cookieSecure(c) {
			t.Fatal("TLS connection did not mark the cookie Secure")
		}
		SetSessionCookies(c, "token")
		if !cookieHasSecure(t, rec) {
			t.Fatal("session cookie issued over TLS is missing Secure")
		}
	})
}

func TestSessionCookieSecureHonoursTrustedProxy(t *testing.T) {
	t.Setenv("GATEWAY_TRUSTED_PROXIES", "203.0.113.0/24")
	t.Setenv("GATEWAY_ADMIN_SECRET", "")

	t.Run("trusted proxy forwarding https is secure", func(t *testing.T) {
		c, rec := sessionTestContext(t, "http://gateway.local/api/v1/auth/login", "203.0.113.5:40000", "https")
		if !cookieSecure(c) {
			t.Fatal("a scheme forwarded by a trusted proxy was ignored")
		}
		SetSessionCookies(c, "token")
		if !cookieHasSecure(t, rec) {
			t.Fatal("session cookie is missing Secure although the trusted proxy terminated TLS")
		}
	})

	t.Run("trusted proxy forwarding http is honestly not secure", func(t *testing.T) {
		c, _ := sessionTestContext(t, "http://gateway.local/api/v1/auth/login", "203.0.113.5:40000", "http")
		if cookieSecure(c) {
			t.Fatal("cookie marked Secure although the trusted proxy reported http")
		}
	})

	t.Run("trusted proxy without the header falls back to TLS state", func(t *testing.T) {
		c, _ := sessionTestContext(t, "http://gateway.local/api/v1/auth/login", "203.0.113.5:40000", "")
		if cookieSecure(c) {
			t.Fatal("cookie marked Secure without any scheme evidence")
		}
	})

	t.Run("peer outside the trusted list is ignored", func(t *testing.T) {
		c, _ := sessionTestContext(t, "http://gateway.local/api/v1/auth/login", "198.51.100.5:40000", "https")
		if cookieSecure(c) {
			t.Fatal("a peer outside GATEWAY_TRUSTED_PROXIES was treated as a proxy")
		}
	})
}

// TestClearSessionCookiesMatchesSecureFlag keeps the logout path symmetric with
// the login path: an expiring cookie must carry the same attributes, otherwise
// browsers can treat it as a different cookie.
func TestClearSessionCookiesMatchesSecureFlag(t *testing.T) {
	t.Setenv("GATEWAY_TRUSTED_PROXIES", "")
	c, rec := sessionTestContext(t, "https://gateway.local/api/v1/auth/logout", "203.0.113.9:5000", "")
	ClearSessionCookies(c)
	if !cookieHasSecure(t, rec) {
		t.Fatal("an expiring cookie over TLS lost the Secure flag")
	}
}
