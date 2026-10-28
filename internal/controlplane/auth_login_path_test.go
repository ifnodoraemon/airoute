package controlplane

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

// C-04 regression: only the real login route is exempt from the control-plane
// bearer-token check.
//
// The exemption used to be strings.HasSuffix(path, "/auth/login"), which exempts
// any future route whose path happens to end that way - for example
// /api/v1/admin/users/7/auth/login. An exact comparison cannot decay like that.

func adminAuthTestEngine() *gin.Engine {
	gin.SetMode(gin.TestMode)
	h := &AdminHandler{} // no repository: the token check is the only gate exercised
	r := gin.New()
	r.Use(h.AdminAuthMiddleware())
	r.Any("/*path", func(c *gin.Context) { c.Status(http.StatusOK) })
	return r
}

func adminAuthStatus(t *testing.T, r *gin.Engine, method, path string) int {
	t.Helper()
	req := httptest.NewRequest(method, path, nil)
	req.RemoteAddr = "127.0.0.1:4000"
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	return rec.Code
}

func TestAdminAuthMiddlewareExemptsOnlyTheLoginRoute(t *testing.T) {
	r := adminAuthTestEngine()

	if status := adminAuthStatus(t, r, http.MethodPost, loginPath); status != http.StatusOK {
		t.Fatalf("POST %s = %d, want 200 (the login route must stay unauthenticated)", loginPath, status)
	}

	// Paths that merely *end* with /auth/login must not be exempt. They do not
	// exist today, which is exactly why the guard must not depend on that.
	suspicious := []string{
		"/api/v1/admin/users/7/auth/login",
		"/api/v1/admin/auth/login/extra",
		"/api/v1/adminx/auth/login",
	}
	for _, path := range suspicious {
		if status := adminAuthStatus(t, r, http.MethodPost, path); status != http.StatusUnauthorized {
			t.Fatalf("POST %s = %d, want 401: a suffix match would exempt this route from authentication", path, status)
		}
	}

	// Ordinary control-plane routes are still protected.
	if status := adminAuthStatus(t, r, http.MethodGet, "/api/v1/admin/keys"); status != http.StatusUnauthorized {
		t.Fatalf("GET /api/v1/admin/keys = %d, want 401", status)
	}
}

// TestAdminAuthMiddlewareRejectsMalformedTokens documents that a syntactically
// invalid token is rejected before any database lookup happens.
func TestAdminAuthMiddlewareRejectsMalformedTokens(t *testing.T) {
	r := adminAuthTestEngine()

	for _, token := range []string{"not-a-token", "a.b", "abc."} {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/keys", nil)
		req.Header.Set("Authorization", "Bearer "+token)
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("token %q = %d, want 401", token, rec.Code)
		}
	}
}
