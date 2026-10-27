package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func adminGuardEngine() *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(AdminAccessGuard())
	ok := func(c *gin.Context) { c.Status(http.StatusOK) }
	r.GET("/api/v1/admin", ok)
	r.GET("/api/v1/admin/keys", ok)
	r.GET("/app/", ok)
	r.GET("/api/v1/adminx", ok)
	r.GET("/v1/models", ok)
	return r
}

func adminGuardRequest(r *gin.Engine, path, remoteAddr, host string, xff string) int {
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.RemoteAddr = remoteAddr
	if host != "" {
		req.Host = host
	}
	if xff != "" {
		req.Header.Set("X-Forwarded-For", xff)
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w.Code
}

// The guard is the D15 fix: the catch-all nginx vhost exposes /api/v1/admin and
// /app, so the gateway itself must refuse public clients by default.
func TestAdminAccessGuardDefaultsToLoopbackAndPrivate(t *testing.T) {
	t.Setenv(adminAllowedIPsEnv, "")
	t.Setenv(adminAllowedHostsEnv, "")
	t.Setenv(adminAllowPublicEnv, "")
	t.Setenv(trustedProxyEnv, "")
	engine := adminGuardEngine()

	cases := []struct {
		name string
		path string
		addr string
		want int
	}{
		{"loopback v4", "/api/v1/admin/keys", "127.0.0.1:5000", http.StatusOK},
		{"loopback v6", "/api/v1/admin", "[::1]:5000", http.StatusOK},
		{"private 10", "/api/v1/admin/keys", "10.1.2.3:5000", http.StatusOK},
		{"private 192.168", "/app/", "192.168.1.9:5000", http.StatusOK},
		{"public admin", "/api/v1/admin/keys", "8.8.8.8:5000", http.StatusForbidden},
		{"public console", "/app/", "203.0.113.9:5000", http.StatusForbidden},
		{"public data plane unaffected", "/v1/models", "8.8.8.8:5000", http.StatusOK},
		{"similar prefix is not control plane", "/api/v1/adminx", "8.8.8.8:5000", http.StatusOK},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := adminGuardRequest(engine, tc.path, tc.addr, "127.0.0.1:8080", ""); got != tc.want {
				t.Fatalf("%s %s from %s = %d, want %d", tc.path, tc.addr, tc.name, got, tc.want)
			}
		})
	}
}

func TestAdminAccessGuardUsesTrustedProxyClientIP(t *testing.T) {
	t.Setenv(adminAllowedIPsEnv, "")
	t.Setenv(adminAllowedHostsEnv, "")
	t.Setenv(adminAllowPublicEnv, "")
	t.Setenv(trustedProxyEnv, "172.22.0.0/16")
	engine := adminGuardEngine()

	// The LB is a trusted proxy; the last untrusted XFF hop is the real client.
	if got := adminGuardRequest(engine, "/api/v1/admin/keys", "172.22.0.1:5000", "127.0.0.1:8080", "8.8.8.8"); got != http.StatusForbidden {
		t.Fatalf("public client behind trusted proxy = %d, want 403", got)
	}
	// A spoofed left-most hop must not help: the right-most untrusted one wins.
	if got := adminGuardRequest(engine, "/api/v1/admin/keys", "172.22.0.1:5000", "127.0.0.1:8080", "192.168.1.5, 8.8.8.8"); got != http.StatusForbidden {
		t.Fatalf("spoofed XFF chain = %d, want 403", got)
	}
	if got := adminGuardRequest(engine, "/api/v1/admin/keys", "172.22.0.1:5000", "127.0.0.1:8080", "192.168.1.5"); got != http.StatusOK {
		t.Fatalf("private client behind trusted proxy = %d, want 200", got)
	}
	if got := adminGuardRequest(engine, "/api/v1/admin/keys", "172.22.0.1:5000", "127.0.0.1:8080", ""); got != http.StatusOK {
		t.Fatalf("trusted proxy peer itself = %d, want 200", got)
	}
}

func TestAdminAccessGuardExplicitAllowlistOverridesDefault(t *testing.T) {
	t.Setenv(adminAllowedIPsEnv, "203.0.113.0/24, 2001:db8::/32")
	t.Setenv(adminAllowedHostsEnv, "")
	t.Setenv(adminAllowPublicEnv, "")
	t.Setenv(trustedProxyEnv, "172.22.0.0/16")
	engine := adminGuardEngine()

	if got := adminGuardRequest(engine, "/api/v1/admin/keys", "172.22.0.1:5000", "127.0.0.1:8080", "203.0.113.7"); got != http.StatusOK {
		t.Fatalf("allowlisted public client = %d, want 200", got)
	}
	if got := adminGuardRequest(engine, "/api/v1/admin/keys", "172.22.0.1:5000", "127.0.0.1:8080", "2001:db8::5"); got != http.StatusOK {
		t.Fatalf("allowlisted IPv6 client = %d, want 200", got)
	}
	if got := adminGuardRequest(engine, "/api/v1/admin/keys", "172.22.0.1:5000", "127.0.0.1:8080", "198.51.100.4"); got != http.StatusForbidden {
		t.Fatalf("non-allowlisted client = %d, want 403", got)
	}
	// An explicit list replaces the private-network default (fail closed).
	if got := adminGuardRequest(engine, "/api/v1/admin/keys", "127.0.0.1:5000", "127.0.0.1:8080", ""); got != http.StatusForbidden {
		t.Fatalf("loopback with explicit allowlist = %d, want 403", got)
	}
}

func TestAdminAccessGuardAllowPublicOptOut(t *testing.T) {
	t.Setenv(adminAllowedIPsEnv, "")
	t.Setenv(adminAllowedHostsEnv, "")
	t.Setenv(adminAllowPublicEnv, "true")
	t.Setenv(trustedProxyEnv, "")
	engine := adminGuardEngine()
	if got := adminGuardRequest(engine, "/api/v1/admin/keys", "8.8.8.8:5000", "127.0.0.1:8080", ""); got != http.StatusOK {
		t.Fatalf("allow-public opt-out = %d, want 200", got)
	}
}

func TestAdminAccessGuardHostAllowlist(t *testing.T) {
	t.Setenv(adminAllowedIPsEnv, "")
	t.Setenv(adminAllowedHostsEnv, "console.example.com, *.internal")
	t.Setenv(adminAllowPublicEnv, "")
	t.Setenv(trustedProxyEnv, "")
	engine := adminGuardEngine()

	if got := adminGuardRequest(engine, "/api/v1/admin/keys", "127.0.0.1:5000", "evil.example.com", ""); got != http.StatusForbidden {
		t.Fatalf("unlisted host = %d, want 403", got)
	}
	if got := adminGuardRequest(engine, "/api/v1/admin/keys", "127.0.0.1:5000", "console.example.com:8080", ""); got != http.StatusOK {
		t.Fatalf("listed host with port = %d, want 200", got)
	}
	if got := adminGuardRequest(engine, "/app/", "127.0.0.1:5000", "airoute.internal", ""); got != http.StatusOK {
		t.Fatalf("wildcard subdomain = %d, want 200", got)
	}
	if got := adminGuardRequest(engine, "/app/", "127.0.0.1:5000", "internal", ""); got != http.StatusForbidden {
		t.Fatalf("wildcard must not match the bare suffix = %d, want 403", got)
	}
}

func TestAdminAccessGuardMisconfiguredAllowlistFailsClosed(t *testing.T) {
	t.Setenv(adminAllowedIPsEnv, "not-a-cidr")
	t.Setenv(adminAllowedHostsEnv, "")
	t.Setenv(adminAllowPublicEnv, "")
	t.Setenv(trustedProxyEnv, "")
	engine := adminGuardEngine()
	if got := adminGuardRequest(engine, "/api/v1/admin/keys", "127.0.0.1:5000", "127.0.0.1:8080", ""); got != http.StatusServiceUnavailable {
		t.Fatalf("misconfigured allowlist = %d, want 503 (fail closed)", got)
	}
}
