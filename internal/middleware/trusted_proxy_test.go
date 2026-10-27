package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestParseTrustedProxies(t *testing.T) {
	nets, err := ParseTrustedProxies("")
	if err != nil || len(nets) != 0 {
		t.Fatalf("empty spec: nets=%v err=%v", nets, err)
	}
	nets, err = ParseTrustedProxies("10.0.0.0/8, 192.168.1.1  ::1")
	if err != nil {
		t.Fatalf("valid spec: %v", err)
	}
	if len(nets) != 3 {
		t.Fatalf("expected 3 networks, got %d", len(nets))
	}
	if _, err := ParseTrustedProxies("not-an-ip"); err == nil {
		t.Fatal("expected an error for an invalid proxy entry")
	}
}

func TestResolveClientIdentityIgnoresForwardingFromUntrustedPeer(t *testing.T) {
	t.Setenv(trustedProxyEnv, "")
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodGet, "/", nil)
	c.Request.RemoteAddr = "203.0.113.50:4321"
	c.Request.Header.Set("X-Forwarded-For", "1.2.3.4, 5.6.7.8")
	c.Request.Header.Set("X-Real-IP", "1.2.3.4")

	identity := ResolveClientIdentity(c)
	if identity.IP != "203.0.113.50" {
		t.Fatalf("untrusted peer must be attributed to the TCP peer, got %q", identity.IP)
	}
	if identity.Forwarded {
		t.Fatal("forwarding headers from an untrusted peer must not be honoured")
	}
}

func TestResolveClientIdentityHonoursTrustedProxyChain(t *testing.T) {
	t.Setenv(trustedProxyEnv, "10.0.0.0/8")
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodGet, "/", nil)
	// The trusted nginx appends the real client; the leading entry is whatever
	// the client claimed and must be ignored.
	c.Request.RemoteAddr = "10.1.2.3:5555"
	c.Request.Header.Set("X-Forwarded-For", "1.2.3.4, 203.0.113.9")

	identity := ResolveClientIdentity(c)
	if identity.IP != "203.0.113.9" {
		t.Fatalf("expected the right-most untrusted hop, got %q", identity.IP)
	}
	if !identity.Forwarded {
		t.Fatal("expected Forwarded=true for a trusted proxy hop")
	}
}

func TestResolveClientIdentityStopsAtTrustedHops(t *testing.T) {
	t.Setenv(trustedProxyEnv, "10.0.0.0/8")
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodGet, "/", nil)
	c.Request.RemoteAddr = "10.1.2.3:5555"
	// Every hop is itself a trusted proxy: there is no client attribution.
	c.Request.Header.Set("X-Forwarded-For", "10.9.9.9")

	identity := ResolveClientIdentity(c)
	if identity.IP != "10.1.2.3" || identity.Forwarded {
		t.Fatalf("expected no client attribution, got %+v", identity)
	}
}

// TestSpoofedForwardedForCannotPickClientIP is the A04 regression at the engine
// level: the bundle that the login lockout and the rate limiters use must not be
// selectable by the caller. With no trusted proxies, rotating X-Forwarded-For
// must yield the same address every time; with a configured proxy, the forged
// leading entries must not shadow the address the proxy appended.
func TestSpoofedForwardedForCannotPickClientIP(t *testing.T) {
	gin.SetMode(gin.TestMode)

	var resolved string
	newProbe := func() *gin.Engine {
		engine := gin.New()
		engine.GET("/probe", func(c *gin.Context) {
			resolved = ResolveClientIP(c)
			c.Status(http.StatusOK)
		})
		return engine
	}
	clientIP := func(t *testing.T, engine *gin.Engine, remoteAddr, forwardedFor string) string {
		t.Helper()
		resolved = ""
		req := httptest.NewRequest(http.MethodGet, "/probe", nil)
		req.RemoteAddr = remoteAddr
		if forwardedFor != "" {
			req.Header.Set("X-Forwarded-For", forwardedFor)
		}
		engine.ServeHTTP(httptest.NewRecorder(), req)
		return resolved
	}

	t.Run("no trusted proxy: header rotation changes nothing", func(t *testing.T) {
		t.Setenv(trustedProxyEnv, "")
		engine := newProbe()
		if err := ConfigureTrustedProxies(engine); err != nil {
			t.Fatalf("configure: %v", err)
		}
		for _, spoofed := range []string{"1.1.1.1", "2.2.2.2", "3.3.3.3"} {
			if got := clientIP(t, engine, "172.18.0.5:1234", spoofed); got != "172.18.0.5" {
				t.Fatalf("spoofed XFF %q changed the client IP to %q", spoofed, got)
			}
		}
		// Gin's own ClientIP must agree, because other packages still use it.
		engine2 := gin.New()
		if err := ConfigureTrustedProxies(engine2); err != nil {
			t.Fatalf("configure: %v", err)
		}
		var ginClientIP string
		engine2.GET("/probe-gin", func(c *gin.Context) { ginClientIP = c.ClientIP() })
		req := httptest.NewRequest(http.MethodGet, "/probe-gin", nil)
		req.RemoteAddr = "172.18.0.5:1234"
		req.Header.Set("X-Forwarded-For", "9.9.9.9")
		engine2.ServeHTTP(httptest.NewRecorder(), req)
		if ginClientIP != "172.18.0.5" {
			t.Fatalf("gin ClientIP trusted a spoofed header: %q", ginClientIP)
		}
	})

	t.Run("trusted proxy: real client survives a forged prefix", func(t *testing.T) {
		t.Setenv(trustedProxyEnv, "10.0.0.0/8")
		engine := newProbe()
		if err := ConfigureTrustedProxies(engine); err != nil {
			t.Fatalf("configure: %v", err)
		}
		// The client sends a forged prefix; the trusted proxy appends the real
		// address it saw.
		if got := clientIP(t, engine, "10.0.0.2:443", "1.1.1.1, 203.0.113.77"); got != "203.0.113.77" {
			t.Fatalf("expected the proxy-appended client address, got %q", got)
		}
		// Rotating the forged prefix does not change the attribution.
		if got := clientIP(t, engine, "10.0.0.2:443", "4.4.4.4, 203.0.113.77"); got != "203.0.113.77" {
			t.Fatalf("forged prefix shadowed the real client: %q", got)
		}
		var ginClientIP string
		engine.GET("/probe-gin", func(c *gin.Context) { ginClientIP = c.ClientIP() })
		req := httptest.NewRequest(http.MethodGet, "/probe-gin", nil)
		req.RemoteAddr = "10.0.0.2:443"
		req.Header.Set("X-Forwarded-For", "1.1.1.1, 203.0.113.77")
		engine.ServeHTTP(httptest.NewRecorder(), req)
		if ginClientIP != "203.0.113.77" {
			t.Fatalf("gin ClientIP = %q, want the proxy-appended client", ginClientIP)
		}
	})

	t.Run("invalid specification is rejected", func(t *testing.T) {
		t.Setenv(trustedProxyEnv, "definitely-not-a-cidr")
		if err := ConfigureTrustedProxies(gin.New()); err == nil {
			t.Fatal("expected an invalid trusted proxy list to be refused instead of trusting everything")
		}
	})
}
