package controlplane

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/ifnodoraemon/airoute/internal/storage"
	"github.com/ifnodoraemon/airoute/internal/telemetry"
)

// TestGetLiveStatsServesInFlightTelemetry covers GET /api/v1/admin/stats/live:
// the frozen JSON contract, in-flight gauges and the no-Redis node-local
// fallback. The handler must never touch the database, so the repository here is
// only needed to construct a realistic AdminHandler.
func TestGetLiveStatsServesInFlightTelemetry(t *testing.T) {
	db, err := storage.OpenDB(":memory:")
	if err != nil {
		t.Fatalf("open memory db: %v", err)
	}
	defer db.Close()
	handler := NewAdminHandler(storage.NewRepository(db), nil, nil)

	telemetry.GlobalMetrics.IncActiveConns()
	defer telemetry.GlobalMetrics.DecActiveConns()
	telemetry.GlobalLiveMetrics.BeginStream()
	defer telemetry.GlobalLiveMetrics.EndStream()
	telemetry.GlobalLiveMetrics.AddTokens(40)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/admin/stats/live", nil)

	handler.GetLiveStats(c)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", w.Code, w.Body.String())
	}
	t.Logf("GET /api/v1/admin/stats/live -> %s", w.Body.String())

	var payload struct {
		Code int            `json:"code"`
		Data map[string]any `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &payload); err != nil {
		t.Fatalf("unmarshal response: %v: %s", err, w.Body.String())
	}
	if payload.Code != 0 {
		t.Fatalf("expected code 0, got %d", payload.Code)
	}

	// The field names are a frozen frontend contract.
	for _, field := range []string{
		"active_requests", "active_streams",
		"tokens_per_sec_1s", "tokens_per_sec_10s", "tokens_per_sec_60s",
		"tokens_last_60s", "last_token_at", "idle_seconds", "nodes", "server_time",
	} {
		if _, ok := payload.Data[field]; !ok {
			t.Fatalf("response is missing contract field %q: %s", field, w.Body.String())
		}
	}

	if got := payload.Data["active_requests"].(float64); got < 1 {
		t.Fatalf("expected active_requests >= 1, got %v", got)
	}
	if got := payload.Data["active_streams"].(float64); got < 1 {
		t.Fatalf("expected active_streams >= 1, got %v", got)
	}
	if got := payload.Data["tokens_last_60s"].(float64); got < 40 {
		t.Fatalf("expected tokens_last_60s >= 40, got %v", got)
	}
	// No Redis in tests: the endpoint must degrade to node-local values.
	if got := payload.Data["nodes"].(float64); got != 1 {
		t.Fatalf("expected nodes 1 without redis, got %v", got)
	}
	if _, err := time.Parse(time.RFC3339, payload.Data["server_time"].(string)); err != nil {
		t.Fatalf("server_time is not RFC3339: %v", payload.Data["server_time"])
	}

	lastTokenAt, _ := payload.Data["last_token_at"].(string)
	if lastTokenAt == "" {
		t.Fatalf("expected a last_token_at timestamp after tokens were recorded")
	}
	if _, err := time.Parse(time.RFC3339, lastTokenAt); err != nil {
		t.Fatalf("last_token_at is not RFC3339: %q", lastTokenAt)
	}
	if idle := payload.Data["idle_seconds"].(float64); idle < 0 {
		t.Fatalf("expected idle_seconds >= 0 after a fresh token, got %v", idle)
	}
}

// TestGetLiveStatsRejectsTenantScope pins the scoping rule: the live gauges are
// cluster-wide and carry no API-key dimension, so a non-admin session gets a
// null payload and the console falls back to its key-scoped 60s window instead
// of reading other tenants' traffic.
func TestGetLiveStatsRejectsTenantScope(t *testing.T) {
	db, err := storage.OpenDB(":memory:")
	if err != nil {
		t.Fatalf("open memory db: %v", err)
	}
	defer db.Close()
	handler := NewAdminHandler(storage.NewRepository(db), nil, nil)

	telemetry.GlobalLiveMetrics.AddTokens(25)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/admin/stats/live", nil)
	c.Set("admin_claims", &AdminClaims{Username: "tenant-a", Role: "user"})

	handler.GetLiveStats(c)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", w.Code, w.Body.String())
	}
	var payload struct {
		Code int `json:"code"`
		Data any `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &payload); err != nil {
		t.Fatalf("unmarshal response: %v: %s", err, w.Body.String())
	}
	if payload.Code != 0 {
		t.Fatalf("expected code 0, got %d", payload.Code)
	}
	if payload.Data != nil {
		t.Fatalf("expected a null payload for a tenant session, got %v", payload.Data)
	}
}
