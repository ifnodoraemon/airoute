package controlplane

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/ifnodoraemon/airoute/internal/storage"
)

// TestAdminAuthRejectsSessionTokenInQuery is the A07 regression.
//
// A ?token= fallback made the 7-day session credential transportable in a URL,
// where it leaks through access logs, browser history and Referer headers. The
// header and cookie paths must keep working.
func TestAdminAuthRejectsSessionTokenInQuery(t *testing.T) {
	handler, repo := newSecurityTestHandler(t)
	if err := repo.CreateUser(&storage.UserRecord{
		Username: "admin", Email: "admin@example.com", PasswordHash: "hash",
		Role: "admin", Status: "active",
	}); err != nil {
		t.Fatalf("create admin: %v", err)
	}
	user, err := repo.GetUserByUsername("admin")
	if err != nil || user == nil {
		t.Fatalf("reload admin: %v", err)
	}
	token, err := GenerateAdminToken(user, time.Hour)
	if err != nil {
		t.Fatalf("token: %v", err)
	}

	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.Use(handler.AdminAuthMiddleware())
	engine.GET("/private", func(c *gin.Context) { c.Status(http.StatusOK) })

	status := func(req *http.Request) int {
		w := httptest.NewRecorder()
		engine.ServeHTTP(w, req)
		return w.Code
	}

	// Query credential is refused.
	req := httptest.NewRequest(http.MethodGet, "/private?token="+token, nil)
	if got := status(req); got != http.StatusUnauthorized {
		t.Fatalf("?token= credential = %d, want 401", got)
	}

	// Bearer still works.
	req = httptest.NewRequest(http.MethodGet, "/private", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	if got := status(req); got != http.StatusOK {
		t.Fatalf("Bearer credential = %d, want 200", got)
	}

	// Session cookie still works (safe method, so no CSRF token required).
	req = httptest.NewRequest(http.MethodGet, "/private", nil)
	req.AddCookie(&http.Cookie{Name: SessionCookieName, Value: token})
	if got := status(req); got != http.StatusOK {
		t.Fatalf("cookie credential = %d, want 200", got)
	}

	// Cookie-authenticated writes still require the CSRF token.
	engine.POST("/private", func(c *gin.Context) { c.Status(http.StatusOK) })
	req = httptest.NewRequest(http.MethodPost, "/private", nil)
	req.AddCookie(&http.Cookie{Name: SessionCookieName, Value: token})
	if got := status(req); got != http.StatusForbidden {
		t.Fatalf("cookie write without CSRF = %d, want 403", got)
	}
}
