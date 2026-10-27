package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/ifnodoraemon/airoute/internal/middleware"
	"github.com/ifnodoraemon/airoute/internal/router"
)

type readinessPayload struct {
	Status       string            `json:"status"`
	Dependencies map[string]string `json:"dependencies"`
}

func decodeReadiness(t *testing.T, body []byte) readinessPayload {
	t.Helper()
	var payload readinessPayload
	if err := json.Unmarshal(body, &payload); err != nil {
		t.Fatalf("decode readiness response %q: %v", string(body), err)
	}
	return payload
}

// /health must stay a cheap 200 liveness answer while /health/ready is wired and
// reflects dependencies. At HEAD /health/ready did not exist at all (404), which
// is why compose, the Helm chart and the rolling-rebuild script could only gate
// on a probe that never checked anything.
func TestReadinessRouteIsWiredAndLivenessStaysCheap(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ResetReadinessChecks()
	t.Cleanup(ResetReadinessChecks)
	middleware.SetDataPlaneReady(false)
	t.Cleanup(func() { middleware.SetDataPlaneReady(false) })

	engine := SetupRouter(router.NewDispatcher(nil), nil)

	rec := httptest.NewRecorder()
	engine.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/health", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("/health = %d, want 200", rec.Code)
	}

	rec = httptest.NewRecorder()
	engine.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/health/ready", nil))
	if rec.Code == http.StatusNotFound {
		t.Fatal("/health/ready is not registered: readiness probes cannot check dependencies")
	}
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("/health/ready = %d before the data plane loaded a snapshot, want 503", rec.Code)
	}
	payload := decodeReadiness(t, rec.Body.Bytes())
	if payload.Status != "not_ready" {
		t.Fatalf("status = %q, want not_ready", payload.Status)
	}
	if payload.Dependencies["data_plane"] != "not_loaded" {
		t.Fatalf("data_plane = %q, want not_loaded", payload.Dependencies["data_plane"])
	}
}

// Dependency results are reported per check, a failing dependency makes the
// endpoint 503, and the raw error text never reaches the (unauthenticated) body.
func TestReadinessReportsDependencies(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ResetReadinessChecks()
	t.Cleanup(ResetReadinessChecks)

	middleware.SetDataPlaneReady(true)
	t.Cleanup(func() { middleware.SetDataPlaneReady(false) })

	RegisterReadinessCheck("database", func(context.Context) error { return nil })
	RegisterReadinessCheck("redis", func(context.Context) error {
		return errors.New("dial tcp 10.0.0.5:6379: connect: connection refused; password=hunter2")
	})

	handler := NewHandler(router.NewDispatcher(nil))
	engine := gin.New()
	engine.GET("/health/ready", handler.HandleReadiness)

	rec := httptest.NewRecorder()
	engine.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/health/ready", nil))
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d with a failing dependency, want 503", rec.Code)
	}
	body := rec.Body.String()
	if strings.Contains(body, "hunter2") || strings.Contains(body, "connection refused") {
		t.Fatalf("readiness body leaks the raw dependency error: %s", body)
	}
	payload := decodeReadiness(t, rec.Body.Bytes())
	if payload.Dependencies["database"] != "ok" {
		t.Fatalf("database = %q, want ok", payload.Dependencies["database"])
	}
	if payload.Dependencies["redis"] != "unavailable" {
		t.Fatalf("redis = %q, want unavailable", payload.Dependencies["redis"])
	}

	// All dependencies healthy => ready.
	ResetReadinessChecks()
	RegisterReadinessCheck("database", func(context.Context) error { return nil })
	RegisterReadinessCheck("redis", func(context.Context) error { return nil })
	rec = httptest.NewRecorder()
	engine.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/health/ready", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d with healthy dependencies, want 200 (body %s)", rec.Code, rec.Body.String())
	}
}

// A hung dependency must not hang the probe.
func TestReadinessRespectsDeadline(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ResetReadinessChecks()
	t.Cleanup(ResetReadinessChecks)
	middleware.SetDataPlaneReady(true)
	t.Cleanup(func() { middleware.SetDataPlaneReady(false) })

	RegisterReadinessCheck("database", func(ctx context.Context) error {
		<-ctx.Done()
		return ctx.Err()
	})

	handler := NewHandler(router.NewDispatcher(nil))
	engine := gin.New()
	engine.GET("/health/ready", handler.HandleReadiness)

	rec := httptest.NewRecorder()
	engine.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/health/ready", nil))
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d for a dependency that outlives the probe deadline, want 503", rec.Code)
	}
}
