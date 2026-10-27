package middleware

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/ifnodoraemon/airoute/internal/config"
)

// TestAuthMiddlewareFailsClosedUntilDataPlaneReady is the A03 regression.
//
// The open-dev-mode bypass keys off "no active key and no key ever registered",
// which is also the state of a process whose first snapshot load failed. Without
// an explicit readiness signal the data plane then serves anonymous traffic on a
// gateway that has credentials registered in storage. Unready must be a 503, not
// an open door.
func TestAuthMiddlewareFailsClosedUntilDataPlaneReady(t *testing.T) {
	gin.SetMode(gin.TestMode)

	previous := config.GetGlobalConfig()
	defer config.SetGlobalConfig(previous)
	previousReady := DataPlaneReady()
	defer SetDataPlaneReady(previousReady)

	// Bootstrap configuration: the snapshot was never loaded, so there is no key
	// in memory and "no key was ever registered" is true.
	cfg := &config.Config{}
	cfg.UpdateAPIKeys(nil, false)
	config.SetGlobalConfig(cfg)

	engine := gin.New()
	engine.Use(AuthMiddleware())
	engine.GET("/v1/models", func(c *gin.Context) { c.Status(http.StatusOK) })

	call := func() *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		engine.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/v1/models", nil))
		return w
	}

	SetDataPlaneReady(false)
	w := call()
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("unready data plane = %d, want 503 (body: %s)", w.Code, w.Body.String())
	}
	if body := w.Body.String(); !strings.Contains(body, "data_plane_unready") {
		t.Fatalf("expected the data_plane_unready code, got %s", body)
	}
	if retryAfter := w.Header().Get("Retry-After"); retryAfter == "" {
		t.Fatal("expected a Retry-After header on the unready response")
	}

	// A successful first load re-enables the documented open-dev-mode path for a
	// genuinely fresh gateway.
	SetDataPlaneReady(true)
	if w := call(); w.Code != http.StatusOK {
		t.Fatalf("ready data plane in open dev mode = %d, want 200 (%s)", w.Code, w.Body.String())
	}

	// Once a key is registered, the ready gateway still enforces auth.
	cfg.UpdateAPIKeys(nil, true)
	if w := call(); w.Code != http.StatusUnauthorized {
		t.Fatalf("registered-key gateway = %d, want 401", w.Code)
	}
}
