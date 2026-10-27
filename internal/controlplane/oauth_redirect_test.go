package controlplane

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/ifnodoraemon/airoute/internal/config"
)

// oauthCallbackLocation drives the browser branch of the OAuth callback (a GET
// that fails before the token exchange because no provider is configured) and
// returns the Location header the victim's browser would follow.
func oauthCallbackLocation(t *testing.T, host, forwardedHost string) string {
	t.Helper()
	handler, _ := newSecurityTestHandler(t)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/oauth/github/callback?code=abc&state=def", nil)
	req.Host = host
	if forwardedHost != "" {
		req.Header.Set("X-Forwarded-Host", forwardedHost)
	}
	c.Request = req
	c.Params = gin.Params{{Key: "provider", Value: "github"}}
	handler.OAuthCallback(c)
	return w.Header().Get("Location")
}

// TestOAuthResultRedirectCannotBePointedAtAttackerHost is the A06 regression.
//
// The post-login redirect used to be built from ResolvePublicBaseURL, which
// falls back to the request Host / X-Forwarded-Host headers — both chosen by the
// caller — so a login that completed successfully could be redirected to an
// attacker origin. Without a configured public URL the redirect must stay
// relative, and a configured URL must win over any header.
func TestOAuthResultRedirectCannotBePointedAtAttackerHost(t *testing.T) {
	gin.SetMode(gin.TestMode)
	previous := config.GetGlobalConfig()
	defer config.SetGlobalConfig(previous)
	config.SetGlobalConfig(&config.Config{})
	t.Setenv("GITHUB_CLIENT_ID", "")
	t.Setenv("GITHUB_CLIENT_SECRET", "")
	t.Setenv("ENABLE_SIMULATED_OAUTH", "")

	for _, tc := range []struct {
		name          string
		host          string
		forwardedHost string
	}{
		{name: "spoofed Host header", host: "evil.example.com"},
		{name: "spoofed X-Forwarded-Host", host: "gateway.internal", forwardedHost: "evil.example.com"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("PUBLIC_URL", "")
			t.Setenv("GATEWAY_PUBLIC_URL", "")
			location := oauthCallbackLocation(t, tc.host, tc.forwardedHost)
			if location == "" {
				t.Fatal("expected a redirect")
			}
			if strings.Contains(location, "evil.example.com") {
				t.Fatalf("attacker host ended up in the redirect: %q", location)
			}
			if !strings.HasPrefix(location, "/?") {
				t.Fatalf("expected a same-origin relative redirect, got %q", location)
			}
			if !strings.Contains(location, "oauth_complete=") && !strings.Contains(location, "oauth_error=") {
				t.Fatalf("expected an OAuth result marker in %q", location)
			}
		})
	}

	t.Run("configured public URL wins over headers", func(t *testing.T) {
		t.Setenv("PUBLIC_URL", "https://console.airoute.example")
		location := oauthCallbackLocation(t, "evil.example.com", "evil.example.com")
		want := "https://console.airoute.example/?"
		if !strings.HasPrefix(location, want) {
			t.Fatalf("redirect = %q, want prefix %q", location, want)
		}
		if strings.Contains(location, "evil.example.com") {
			t.Fatalf("attacker host leaked into a configured redirect: %q", location)
		}
	})
}
