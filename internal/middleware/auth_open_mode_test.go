package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/ifnodoraemon/airoute/internal/config"
	"github.com/ifnodoraemon/airoute/internal/model"
	"github.com/ifnodoraemon/airoute/internal/storage"
)

// TestAuthMiddlewareOpenModeRequiresNoKeyEverRegistered pins the gateway's most
// consequential authorisation decision: the open dev mode bypass.
//
// AuthMiddleware skips authentication entirely when there are no active keys AND
// no key has ever been registered. The second half of that condition is what
// keeps a gateway with only disabled/revoked keys closed. It is derived from a
// database read during hot reload, so a read that silently reports "no keys were
// ever registered" turns the gateway into an open proxy. That failure mode is
// covered in internal/controlplane/sync_test.go; this test pins the middleware
// side of the contract.
func TestAuthMiddlewareOpenModeRequiresNoKeyEverRegistered(t *testing.T) {
	gin.SetMode(gin.TestMode)

	previous := config.GetGlobalConfig()
	defer config.SetGlobalConfig(previous)

	cases := []struct {
		name          string
		keys          []model.APIKeyConfig
		hasRegistered bool
		header        string
		wantStatus    int
	}{
		{
			name:          "nothing ever registered is open dev mode",
			keys:          nil,
			hasRegistered: false,
			header:        "",
			wantStatus:    http.StatusOK,
		},
		{
			// The security case: every key is disabled, but a key has existed.
			// The gateway must stay closed to unauthenticated traffic.
			name:          "a registered but disabled key still enforces auth",
			keys:          nil,
			hasRegistered: true,
			header:        "",
			wantStatus:    http.StatusUnauthorized,
		},
		{
			name:          "an unknown key is rejected",
			keys:          nil,
			hasRegistered: true,
			header:        "Bearer sk-not-issued",
			wantStatus:    http.StatusUnauthorized,
		},
		{
			// A revoked key must not be usable even though it is still listed.
			name: "a revoked key is rejected",
			keys: []model.APIKeyConfig{
				{Key: "sk-revoked", TenantID: "t", Status: "disabled"},
			},
			hasRegistered: true,
			header:        "Bearer sk-revoked",
			wantStatus:    http.StatusUnauthorized,
		},
		{
			name: "an active key is accepted",
			keys: []model.APIKeyConfig{
				// The runtime snapshot is keyed by the stored digest: reload reads
				// api_keys.key, which holds HashAPIKey(credential).
				{Key: storage.HashAPIKey("sk-live"), TenantID: "t", Status: "active"},
			},
			hasRegistered: true,
			header:        "Bearer sk-live",
			wantStatus:    http.StatusOK,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := &config.Config{}
			cfg.UpdateAPIKeys(tc.keys, tc.hasRegistered)
			config.SetGlobalConfig(cfg)

			engine := gin.New()
			engine.Use(AuthMiddleware())
			engine.GET("/v1/models", func(c *gin.Context) {
				c.Status(http.StatusOK)
			})

			req, err := http.NewRequest(http.MethodGet, "/v1/models", nil)
			if err != nil {
				t.Fatalf("build request: %v", err)
			}
			if tc.header != "" {
				req.Header.Set("Authorization", tc.header)
			}

			w := httptest.NewRecorder()
			engine.ServeHTTP(w, req)

			if w.Code != tc.wantStatus {
				t.Fatalf("status = %d, want %d (body: %s)", w.Code, tc.wantStatus, w.Body.String())
			}
		})
	}
}

// TestAuthMiddlewareOpenModeClosesOnceAKeyIsRegistered is the state transition
// that matters operationally: a fresh gateway serves traffic openly, and
// registering the first key must close it immediately.
func TestAuthMiddlewareOpenModeClosesOnceAKeyIsRegistered(t *testing.T) {
	gin.SetMode(gin.TestMode)

	previous := config.GetGlobalConfig()
	defer config.SetGlobalConfig(previous)

	engine := gin.New()
	engine.Use(AuthMiddleware())
	engine.GET("/v1/models", func(c *gin.Context) { c.Status(http.StatusOK) })

	call := func() int {
		req, _ := http.NewRequest(http.MethodGet, "/v1/models", nil)
		w := httptest.NewRecorder()
		engine.ServeHTTP(w, req)
		return w.Code
	}

	// Fresh gateway: open.
	cfg := &config.Config{}
	cfg.UpdateAPIKeys(nil, false)
	config.SetGlobalConfig(cfg)
	if got := call(); got != http.StatusOK {
		t.Fatalf("fresh gateway status = %d, want 200 (open dev mode)", got)
	}

	// First key registered: closed to unauthenticated traffic.
	cfg2 := &config.Config{}
	cfg2.UpdateAPIKeys([]model.APIKeyConfig{{Key: "sk-first", TenantID: "t", Status: "active"}}, true)
	config.SetGlobalConfig(cfg2)
	if got := call(); got != http.StatusUnauthorized {
		t.Fatalf("after registering a key status = %d, want 401", got)
	}

	// The key is later disabled, but it still existed: stays closed.
	cfg3 := &config.Config{}
	cfg3.UpdateAPIKeys(nil, true)
	config.SetGlobalConfig(cfg3)
	if got := call(); got != http.StatusUnauthorized {
		t.Fatalf("after disabling every key status = %d, want 401 (must not reopen)", got)
	}
}
