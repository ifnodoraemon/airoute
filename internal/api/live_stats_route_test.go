package api

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ifnodoraemon/airoute/internal/controlplane"
	"github.com/ifnodoraemon/airoute/internal/router"
	"github.com/ifnodoraemon/airoute/internal/storage"
)

// TestLiveStatsRouteRegistration pins the frozen endpoint path and its placement
// in the protected admin group: an unauthenticated call must be rejected by the
// admin auth middleware (401) instead of falling through as an unknown route
// (404). The handler itself is covered by the controlplane package.
func TestLiveStatsRouteRegistration(t *testing.T) {
	db, err := storage.OpenDB(":memory:")
	if err != nil {
		t.Fatalf("open memory db: %v", err)
	}
	defer db.Close()
	adminHandler := controlplane.NewAdminHandler(storage.NewRepository(db), nil, nil)
	engine := SetupRouter(router.NewDispatcher(nil), adminHandler)

	w := httptest.NewRecorder()
	engine.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/v1/admin/stats/live", nil))

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 from the admin auth middleware for GET /api/v1/admin/stats/live, got %d: %s", w.Code, w.Body.String())
	}
}
