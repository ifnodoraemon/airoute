package tests

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/ifnodoraemon/airoute/internal/api"
	"github.com/ifnodoraemon/airoute/internal/config"
	"github.com/ifnodoraemon/airoute/internal/controlplane"
	"github.com/ifnodoraemon/airoute/internal/middleware"
	"github.com/ifnodoraemon/airoute/internal/model"
	"github.com/ifnodoraemon/airoute/internal/router"
	"github.com/ifnodoraemon/airoute/internal/storage"
)

// TestAPIKey_ActiveVsDisabled verifies that disabled keys are immediately rejected on the hot path
// and that toggling status via the Admin API dynamically evicts/re-enables them in Data Plane memory.
func TestAPIKey_ActiveVsDisabled(t *testing.T) {
	gin.SetMode(gin.TestMode)

	savedCfg := *config.GetGlobalConfig()
	defer func() {
		config.SetGlobalConfig(&savedCfg)
	}()

	tempDB := filepath.Join(t.TempDir(), fmt.Sprintf("test_key_gov_%d.db", time.Now().UnixNano()))

	db, err := storage.OpenDB(tempDB)
	if err != nil {
		t.Fatalf("failed to open test db: %v", err)
	}
	defer db.Close()

	repo := storage.NewRepository(db)
	dispatcher := router.NewDispatcher(nil)
	synchronizer := controlplane.NewSynchronizer(repo, dispatcher)
	adminHandler := controlplane.NewAdminHandler(repo, synchronizer, dispatcher)

	// Create test API key
	keyRec := &storage.APIKeyRecord{
		Key:           "sk-nano-test-toggle-key",
		TenantID:      "qa-department",
		AllowedModels: []string{"gpt-4o"},
		RPM:           100,
		Status:        "active",
	}
	if err := repo.CreateAPIKey(keyRec); err != nil {
		t.Fatalf("CreateAPIKey failed: %v", err)
	}

	// Hot reload into memory
	if err := synchronizer.ReloadFromDB(); err != nil {
		t.Fatalf("ReloadFromDB failed: %v", err)
	}

	// Setup API router
	engine := api.SetupRouter(dispatcher, adminHandler)

	// 1. Initial State: Key is active -> Auth should succeed
	req := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	req.Header.Set("Authorization", "Bearer "+keyRec.Key)
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for active key, got %d: %s", w.Code, w.Body.String())
	}

	// 2. Admin disables the key via PUT /api/v1/admin/keys/:id
	disabledStatus := "disabled"
	bodyBytes, _ := json.Marshal(map[string]any{
		"status": disabledStatus,
	})
	adminToken, _ := controlplane.GenerateAdminToken("admin", "admin", time.Hour)
	updateReq := httptest.NewRequest(http.MethodPut, fmt.Sprintf("/api/v1/admin/keys/%d", keyRec.ID), bytes.NewReader(bodyBytes))
	updateReq.Header.Set("Authorization", "Bearer "+adminToken)
	updateReq.Header.Set("Content-Type", "application/json")
	updateW := httptest.NewRecorder()
	engine.ServeHTTP(updateW, updateReq)
	if updateW.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for key update, got %d: %s", updateW.Code, updateW.Body.String())
	}

	// Verify DB record status
	fetched, err := repo.GetAPIKey(keyRec.ID)
	if err != nil || fetched.Status != "disabled" {
		t.Fatalf("expected key status in DB to be 'disabled', got %v (err: %v)", fetched.Status, err)
	}

	// 3. Hot Path Check: The disabled key MUST be rejected with 401 Unauthorized
	reqDisabled := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	reqDisabled.Header.Set("Authorization", "Bearer "+keyRec.Key)
	wDisabled := httptest.NewRecorder()
	engine.ServeHTTP(wDisabled, reqDisabled)
	if wDisabled.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 Unauthorized for disabled key, got %d: %s", wDisabled.Code, wDisabled.Body.String())
	}

	// 4. Admin re-enables the key
	activeStatus := "active"
	bodyActiveBytes, _ := json.Marshal(map[string]any{
		"status": activeStatus,
	})
	reEnableReq := httptest.NewRequest(http.MethodPut, fmt.Sprintf("/api/v1/admin/keys/%d", keyRec.ID), bytes.NewReader(bodyActiveBytes))
	reEnableReq.Header.Set("Authorization", "Bearer "+adminToken)
	reEnableReq.Header.Set("Content-Type", "application/json")
	reEnableW := httptest.NewRecorder()
	engine.ServeHTTP(reEnableW, reEnableReq)
	if reEnableW.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for key re-enable, got %d: %s", reEnableW.Code, reEnableW.Body.String())
	}

	// 5. Hot Path Check: The re-enabled key MUST succeed again
	reqReEnabled := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	reqReEnabled.Header.Set("Authorization", "Bearer "+keyRec.Key)
	wReEnabled := httptest.NewRecorder()
	engine.ServeHTTP(wReEnabled, reqReEnabled)
	if wReEnabled.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for re-enabled key, got %d: %s", wReEnabled.Code, wReEnabled.Body.String())
	}
}

// TestAPIKeysMap_O1_LookupSpeed verifies nanosecond O(1) hash map lookup performance
// for per-request authentication checks across thousands of API keys.
func TestAPIKeysMap_O1_LookupSpeed(t *testing.T) {
	savedCfg := *config.GetGlobalConfig()
	defer func() {
		config.SetGlobalConfig(&savedCfg)
	}()

	keyCount := 10000
	keys := make([]model.APIKeyConfig, keyCount)
	for i := 0; i < keyCount; i++ {
		keys[i] = model.APIKeyConfig{
			Key:      fmt.Sprintf("sk-nano-perf-test-key-%06d", i),
			TenantID: fmt.Sprintf("tenant-%d", i),
			RPM:      1000,
		}
	}

	cfg := &config.Config{
		APIKeys: keys,
	}
	config.SetGlobalConfig(cfg)

	// Perform 100,000 lookups
	targetKey := "sk-nano-perf-test-key-009999" // last key in slice
	start := time.Now()
	iterations := 100000
	for i := 0; i < iterations; i++ {
		k := cfg.GetAPIKey(targetKey)
		if k == nil || k.Key != targetKey {
			t.Fatalf("failed to lookup key")
		}
	}
	elapsed := time.Since(start)
	avgNsPerOp := elapsed.Nanoseconds() / int64(iterations)

	t.Logf("100,000 lookups across 10,000 keys took %v (average: %d ns/op)", elapsed, avgNsPerOp)
	if avgNsPerOp > 5000 { // O(1) map lookup is ~50-150ns normally, and < 2000ns under -race on shared CI runners
		t.Errorf("O(1) lookup took %d ns/op, expected < 5000 ns/op", avgNsPerOp)
	}
}

// TestRateLimitMiddleware_RedisGracefulDegradation verifies that when Redis is nil or unreachable,
// the gateway seamlessly and safely falls back to local token buckets without dropping traffic.
func TestRateLimitMiddleware_RedisGracefulDegradation(t *testing.T) {
	gin.SetMode(gin.TestMode)

	savedCfg := *config.GetGlobalConfig()
	defer func() {
		config.SetGlobalConfig(&savedCfg)
	}()

	testKey := "sk-nano-degrade-test"
	cfg := &config.Config{
		APIKeys: []model.APIKeyConfig{
			{
				Key:      testKey,
				TenantID: "tenant-degrade",
				RPM:      1, // 1 request allowed per minute
			},
		},
	}
	config.SetGlobalConfig(cfg)

	r := gin.New()
	r.Use(middleware.AuthMiddleware())
	r.Use(middleware.RateLimitMiddleware())
	r.GET("/ping", func(c *gin.Context) {
		c.String(http.StatusOK, "pong")
	})

	// 1st request succeeds via local token bucket fallback
	req1 := httptest.NewRequest(http.MethodGet, "/ping", nil)
	req1.Header.Set("Authorization", "Bearer "+testKey)
	w1 := httptest.NewRecorder()
	r.ServeHTTP(w1, req1)
	if w1.Code != http.StatusOK {
		t.Fatalf("1st request expected 200, got %d: %s", w1.Code, w1.Body.String())
	}

	// 2nd request is blocked with 429
	req2 := httptest.NewRequest(http.MethodGet, "/ping", nil)
	req2.Header.Set("Authorization", "Bearer "+testKey)
	w2 := httptest.NewRecorder()
	r.ServeHTTP(w2, req2)
	if w2.Code != http.StatusTooManyRequests {
		t.Fatalf("2nd request expected 429, got %d", w2.Code)
	}
}
