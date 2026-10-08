package tests

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
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

// Helper to sign Stripe webhook payload
func signStripePayload(payload []byte, secret string, ts int64) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(fmt.Sprintf("%d.", ts)))
	mac.Write(payload)
	sig := hex.EncodeToString(mac.Sum(nil))
	return fmt.Sprintf("t=%d,v1=%s", ts, sig)
}

// 1. Test Stripe Webhook Signature Verification & Fraud Prevention
func TestAudit_StripeWebhookSignatureVerification(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, err := storage.OpenDB(":memory:")
	require.NoError(t, err)
	defer db.Close()

	repo := storage.NewRepository(db)
	dispatcher := router.NewDispatcher(nil)
	sync := controlplane.NewSynchronizer(repo, dispatcher)
	adminHandler := controlplane.NewAdminHandler(repo, sync, dispatcher)
	engine := api.SetupRouter(dispatcher, adminHandler)

	secret := "whsec_test_secret_12345"
	os.Setenv("STRIPE_WEBHOOK_SECRET", secret)
	defer os.Unsetenv("STRIPE_WEBHOOK_SECRET")

	// Create user & recharge order
	user := &storage.UserRecord{
		Username:     "alice",
		PasswordHash: "dummy",
		Role:         "user",
		Status:       "active",
		Balance:      10.0,
	}
	require.NoError(t, repo.CreateUser(user))

	orderNo := "ORD_TEST_999"
	order := &storage.RechargeOrderRecord{
		OrderNo:  orderNo,
		Username: "alice",
		Amount:   100.0,
		Status:   "pending",
	}
	require.NoError(t, repo.CreateRechargeOrder(order))

	eventPayload := []byte(fmt.Sprintf(`{
		"type": "checkout.session.completed",
		"data": {
			"object": {
				"id": "cs_test_123",
				"client_reference_id": "%s",
				"payment_status": "paid"
			}
		}
	}`, orderNo))

	// Case 1: Attack without signature header -> Rejected
	{
		req := httptest.NewRequest(http.MethodPost, "/api/v1/public/stripe/webhook", bytes.NewReader(eventPayload))
		w := httptest.NewRecorder()
		engine.ServeHTTP(w, req)
		assert.Equal(t, http.StatusBadRequest, w.Code)
		assert.Contains(t, w.Body.String(), "Stripe 签名校验失败")

		// Verify order is still pending and balance unchanged
		u, _ := repo.GetUserByUsername("alice")
		assert.Equal(t, 10.0, u.Balance)
	}

	// Case 2: Attack with fake/tampered signature -> Rejected
	{
		fakeSig := "t=1700000000,v1=0000000000000000000000000000000000000000000000000000000000000000"
		req := httptest.NewRequest(http.MethodPost, "/api/v1/public/stripe/webhook", bytes.NewReader(eventPayload))
		req.Header.Set("Stripe-Signature", fakeSig)
		w := httptest.NewRecorder()
		engine.ServeHTTP(w, req)
		assert.Equal(t, http.StatusBadRequest, w.Code)
		assert.Contains(t, w.Body.String(), "Stripe 签名校验失败")
	}

	// Case 3: Replay attack with expired timestamp (> 5 minutes) -> Rejected
	{
		expiredTime := time.Now().Add(-10 * time.Minute).Unix()
		expiredSig := signStripePayload(eventPayload, secret, expiredTime)
		req := httptest.NewRequest(http.MethodPost, "/api/v1/public/stripe/webhook", bytes.NewReader(eventPayload))
		req.Header.Set("Stripe-Signature", expiredSig)
		w := httptest.NewRecorder()
		engine.ServeHTTP(w, req)
		assert.Equal(t, http.StatusBadRequest, w.Code)
		assert.Contains(t, w.Body.String(), "重放攻击")
	}

	// Case 4: Legitimate Stripe Webhook with valid HMAC-SHA256 signature -> Accepted & Credited
	{
		validSig := signStripePayload(eventPayload, secret, time.Now().Unix())
		req := httptest.NewRequest(http.MethodPost, "/api/v1/public/stripe/webhook", bytes.NewReader(eventPayload))
		req.Header.Set("Stripe-Signature", validSig)
		w := httptest.NewRecorder()
		engine.ServeHTTP(w, req)
		assert.Equal(t, http.StatusOK, w.Code)
		assert.Contains(t, w.Body.String(), `"received":true`)

		// Verify balance increased by 100.0
		u, _ := repo.GetUserByUsername("alice")
		assert.Equal(t, 110.0, u.Balance)
	}
}

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
	_ = repo.RecordUsageLog(&storage.UsageLogRecord{
		APIKey:    "sk-alice-secret-key",
		TenantID:  "tenant-alice",
		Model:     "deepseek-chat",
		SessionID: "sess-alice",
		StatusCode: 200,
	})
	_ = repo.RecordUsageLog(&storage.UsageLogRecord{
		APIKey:    "sk-bob-secret-key",
		TenantID:  "tenant-bob",
		Model:     "gpt-4o",
		SessionID: "sess-bob",
		StatusCode: 200,
	})

	// Case 1: Unauthenticated airoute_chat is rejected when keys exist
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
		assert.Equal(t, http.StatusOK, w.Code)
		assert.Contains(t, w.Body.String(), "Authentication required for airoute_chat")
	}

	// Case 2: Unauthenticated airoute_query_logs cannot dump all logs without session_id
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
		assert.Equal(t, http.StatusOK, w.Code)
		assert.Contains(t, w.Body.String(), "Authentication required to query global logs")
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

	// Wednesday 03:00 (Weekday night off-peak)
	wednesdayNight := time.Date(2026, 10, 7, 3, 0, 0, 0, time.FixedZone("CST", 8*3600))
	costNight, savedNight, _, isOffNight, _ := engine.CalculateCostDetailed("test-offpeak-model", 1000000, 1000000, 0, wednesdayNight)
	assert.True(t, isOffNight, "Wednesday 03:00 should be detected as off-peak")
	assert.Equal(t, 15.0, costNight, "Cost should be 50% discount (10 + 20) * 0.5 = 15.0")
	assert.Equal(t, 15.0, savedNight)

	// Wednesday 14:00 (Weekday daytime peak)
	wednesdayDay := time.Date(2026, 10, 7, 14, 0, 0, 0, time.FixedZone("CST", 8*3600))
	costDay, _, _, isOffDay, _ := engine.CalculateCostDetailed("test-offpeak-model", 1000000, 1000000, 0, wednesdayDay)
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

	// Admin updates RPM to 6000 RPM (100 tokens per sec)
	bucket2 := limiter.GetBucket("test-key-dynamic", 6000)
	assert.Equal(t, float64(6000), bucket2.Capacity())
	assert.Equal(t, 100.0, bucket2.Rate())
}

// 6. Test CORS headers and Sandbox Recharge GET route
func TestAudit_CORSAndSandboxRecharge(t *testing.T) {
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

	// Verify GET /api/v1/user/wallet/recharge/sandbox works
	user := &storage.UserRecord{
		Username:     "tester",
		PasswordHash: "hash",
		Role:         "admin",
		Status:       "active",
		Balance:      0.0,
	}
	require.NoError(t, repo.CreateUser(user))
	token, _ := controlplane.GenerateAdminToken("tester", "admin", time.Hour)

	reqGet := httptest.NewRequest(http.MethodGet, "/api/v1/user/wallet/recharge/sandbox?order_no=SANDBOX123&amount=88.8", nil)
	reqGet.Header.Set("Authorization", "Bearer "+token)
	wGet := httptest.NewRecorder()
	engine.ServeHTTP(wGet, reqGet)
	assert.Equal(t, http.StatusOK, wGet.Code)
	assert.Contains(t, wGet.Body.String(), "充值成功")
}
