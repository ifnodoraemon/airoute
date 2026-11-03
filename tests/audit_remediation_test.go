package tests

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/ifnodoraemon/airoute/internal/api"
	"github.com/ifnodoraemon/airoute/internal/billing"
	"github.com/ifnodoraemon/airoute/internal/controlplane"
	"github.com/ifnodoraemon/airoute/internal/middleware"
	"github.com/ifnodoraemon/airoute/internal/router"
	"github.com/ifnodoraemon/airoute/internal/storage"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// 2. Test MCP Authentication & Tenant Log Isolation
func TestAudit_MCPAuthAndTenantLogIsolation(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, err := storage.OpenDB(":memory:")
	require.NoError(t, err)
	defer db.Close()

	repo := storage.NewRepository(db)
	dispatcher := router.NewDispatcher(nil)
	sync := controlplane.NewSynchronizer(repo, dispatcher)
	adminHandler := controlplane.NewAdminHandler(repo, sync, dispatcher)
	engine := api.SetupRouter(dispatcher, adminHandler)

	// Create a user and active API key
	user := &storage.UserRecord{
		Username: "alice",
		Role:     "user",
		Status:   "active",
		Balance:  50.0,
	}
	require.NoError(t, repo.CreateUser(user))

	apiKey := &storage.APIKeyRecord{
		Key:      "sk-alice-secret-key",
		UserID:   user.ID,
		TenantID: "tenant-alice",
		Status:   "active",
	}
	require.NoError(t, repo.CreateAPIKey(apiKey))

	// Record logs for two different tenants
	_ = repo.RecordUsageLogContext(context.Background(), &storage.UsageLogRecord{
		APIKey:     "sk-alice-secret-key",
		TenantID:   "tenant-alice",
		Model:      "deepseek-chat",
		SessionID:  "sess-alice",
		StatusCode: 200,
	})
	_ = repo.RecordUsageLogContext(context.Background(), &storage.UsageLogRecord{
		APIKey:     "sk-bob-secret-key",
		TenantID:   "tenant-bob",
		Model:      "gpt-4o",
		SessionID:  "sess-bob",
		StatusCode: 200,
	})

	// Case 1: Unauthenticated airoute_chat is rejected when keys exist.
	// The endpoint itself is fail-closed now, so no tool ever executes.
	{
		rpcBody := map[string]interface{}{
			"jsonrpc": "2.0",
			"id":      1,
			"method":  "tools/call",
			"params": map[string]interface{}{
				"name": "airoute_chat",
				"arguments": map[string]interface{}{
					"model":   "deepseek-chat",
					"message": "hello",
				},
			},
		}
		data, _ := json.Marshal(rpcBody)
		req := httptest.NewRequest(http.MethodPost, "/mcp/messages", bytes.NewReader(data))
		w := httptest.NewRecorder()
		engine.ServeHTTP(w, req)
		assert.Equal(t, http.StatusUnauthorized, w.Code)
		assert.NotContains(t, w.Body.String(), "choices")
	}

	// Case 2: Unauthenticated airoute_query_logs cannot dump global logs either.
	{
		rpcBody := map[string]interface{}{
			"jsonrpc": "2.0",
			"id":      2,
			"method":  "tools/call",
			"params": map[string]interface{}{
				"name": "airoute_query_logs",
				"arguments": map[string]interface{}{
					"limit": 10,
				},
			},
		}
		data, _ := json.Marshal(rpcBody)
		req := httptest.NewRequest(http.MethodPost, "/mcp/messages", bytes.NewReader(data))
		w := httptest.NewRecorder()
		engine.ServeHTTP(w, req)
		assert.Equal(t, http.StatusUnauthorized, w.Code)
		assert.NotContains(t, w.Body.String(), "tenant-bob")
	}

	// Case 3: Authenticated airoute_query_logs with Alice's API key is tenant-isolated
	{
		rpcBody := map[string]interface{}{
			"jsonrpc": "2.0",
			"id":      3,
			"method":  "tools/call",
			"params": map[string]interface{}{
				"name": "airoute_query_logs",
				"arguments": map[string]interface{}{
					"limit": 10,
				},
			},
		}
		data, _ := json.Marshal(rpcBody)
		req := httptest.NewRequest(http.MethodPost, "/mcp/messages", bytes.NewReader(data))
		req.Header.Set("Authorization", "Bearer sk-alice-secret-key")
		w := httptest.NewRecorder()
		engine.ServeHTTP(w, req)
		assert.Equal(t, http.StatusOK, w.Code)
		// Should contain Alice's model and tenant, but NOT Bob's
		assert.Contains(t, w.Body.String(), "deepseek-chat")
		assert.NotContains(t, w.Body.String(), "tenant-bob")
	}
}

// 3. Test Database InsertGetID Dialect Adaptation
func TestAudit_DatabaseInsertGetID(t *testing.T) {
	db, err := storage.OpenDB(":memory:")
	require.NoError(t, err)
	defer db.Close()

	repo := storage.NewRepository(db)

	user := &storage.UserRecord{
		Username:     "carol",
		PasswordHash: "pass",
		Role:         "user",
		Status:       "active",
	}
	err = repo.CreateUser(user)
	require.NoError(t, err)
	assert.Greater(t, user.ID, int64(0), "user.ID should be populated with auto-increment ID")

	key := &storage.APIKeyRecord{
		Key:      "sk-carol-123",
		UserID:   user.ID,
		TenantID: "tenant-carol",
		Status:   "active",
	}
	err = repo.CreateAPIKey(key)
	require.NoError(t, err)
	assert.Greater(t, key.ID, int64(0), "key.ID should be populated with auto-increment ID")
	assert.Equal(t, user.ID, key.UserID, "key.UserID should correctly match user.ID")
}

// 4. Test Weekday Off-Peak Window in Default Custom Mode
func TestAudit_WeekdayCustomOffPeakDiscount(t *testing.T) {
	db, err := storage.OpenDB(":memory:")
	require.NoError(t, err)
	defer db.Close()

	repo := storage.NewRepository(db)
	engine := billing.NewEngine(repo)

	// Price record with default "custom" mode and OffPeakStart/End set to 00:00 - 08:30 (50% discount)
	price := &storage.ModelPriceRecord{
		Model:           "test-offpeak-model",
		PromptPrice:     10.0,
		CompletionPrice: 20.0,
		OffPeakEnabled:  true,
		OffPeakMode:     "custom",
		OffPeakDiscount: 0.5,
		OffPeakStart:    "00:00",
		OffPeakEnd:      "08:30",
		WeekendAllDay:   true,
	}
	require.NoError(t, repo.SaveModelPrice(price))
	require.NoError(t, engine.ReloadPrices())

	// Wednesday 03:00 (Weekday night off-peak). The 5-variant CalculateCost* API was
	// retired (C-01); the single funnel callers use is the units+cache-creation one.
	wednesdayNight := time.Date(2026, 10, 7, 3, 0, 0, 0, time.FixedZone("CST", 8*3600))
	costNight, savedNight, _, isOffNight, _, _ := engine.CalculateCostForUnitsWithCacheCreation("test-offpeak-model", "default", 1000000, 1000000, 0, 0, 1, wednesdayNight)
	assert.True(t, isOffNight, "Wednesday 03:00 should be detected as off-peak")
	assert.Equal(t, 15.0, costNight, "Cost should be 50% discount (10 + 20) * 0.5 = 15.0")
	assert.Equal(t, 15.0, savedNight)

	// Wednesday 14:00 (Weekday daytime peak)
	wednesdayDay := time.Date(2026, 10, 7, 14, 0, 0, 0, time.FixedZone("CST", 8*3600))
	costDay, _, _, isOffDay, _, _ := engine.CalculateCostForUnitsWithCacheCreation("test-offpeak-model", "default", 1000000, 1000000, 0, 0, 1, wednesdayDay)
	assert.False(t, isOffDay, "Wednesday 14:00 should NOT be off-peak")
	assert.Equal(t, 30.0, costDay, "Cost should be normal (10 + 20) = 30.0")
}

// 5. Test Dynamic RPM Rate Limit Hot Update
func TestAudit_RateLimiterDynamicRPMUpdate(t *testing.T) {
	limiter := middleware.GlobalRateLimiter

	// Initial low limit: 1 RPM
	bucket1 := limiter.GetBucket("test-key-dynamic", 1)
	assert.True(t, bucket1.Allow())
	assert.False(t, bucket1.Allow(), "Second request within minute should be blocked with 1 RPM")

	// Admin updates RPM to 6000 RPM (100 tokens per sec). Assert the hot update
	// behaviourally: the retired Capacity()/Rate() accessors exposed internal state
	// that nothing read, so the test now proves the refill rate changed instead.
	bucket2 := limiter.GetBucket("test-key-dynamic", 6000)
	time.Sleep(20 * time.Millisecond) // at 100 tokens/s that is ~2 tokens; at 1 RPM it is ~0
	assert.True(t, bucket2.Allow(), "a bucket raised to 6000 RPM must refill at the new rate immediately")
}

// 6. Test CORS headers and Sandbox Recharge GET route
func TestAudit_CORSAndRemovedPaymentSimulator(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, err := storage.OpenDB(":memory:")
	require.NoError(t, err)
	defer db.Close()

	repo := storage.NewRepository(db)
	dispatcher := router.NewDispatcher(nil)
	sync := controlplane.NewSynchronizer(repo, dispatcher)
	adminHandler := controlplane.NewAdminHandler(repo, sync, dispatcher)
	engine := api.SetupRouter(dispatcher, adminHandler)

	// Verify CORS preflight allows x-goog-api-key and x-admin-token
	reqOpt := httptest.NewRequest(http.MethodOptions, "/v1/models", nil)
	wOpt := httptest.NewRecorder()
	engine.ServeHTTP(wOpt, reqOpt)
	assert.Equal(t, 204, wOpt.Code)
	corsHeaders := wOpt.Header().Get("Access-Control-Allow-Headers")
	assert.Contains(t, corsHeaders, "x-goog-api-key")
	assert.Contains(t, corsHeaders, "x-admin-token")
	assert.Contains(t, corsHeaders, "Stripe-Signature")

	// The removed payment simulator has no route for either method.
	for _, method := range []string{http.MethodGet, http.MethodPost} {
		req := httptest.NewRequest(method, "/api/v1/user/wallet/recharge/sandbox", nil)
		w := httptest.NewRecorder()
		engine.ServeHTTP(w, req)
		assert.Equal(t, http.StatusNotFound, w.Code)
	}
}
