package tests

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/ifnodoraemon/nano-gateway/internal/api"
	"github.com/ifnodoraemon/nano-gateway/internal/billing"
	"github.com/ifnodoraemon/nano-gateway/internal/config"
	"github.com/ifnodoraemon/nano-gateway/internal/controlplane"
	"github.com/ifnodoraemon/nano-gateway/internal/model"
	"github.com/ifnodoraemon/nano-gateway/internal/router"
	"github.com/ifnodoraemon/nano-gateway/internal/storage"
	"golang.org/x/crypto/bcrypt"
)

func TestUserWallet_RegistrationAndVerification(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, err := storage.OpenDB(":memory:")
	if err != nil {
		t.Fatalf("failed to open db: %v", err)
	}
	defer db.Close()

	repo := storage.NewRepository(db)
	storage.SetGlobalRepository(repo)

	cfg := config.DefaultConfig()
	config.SetGlobalConfig(cfg)
	dispatcher := router.NewDispatcher(nil)
	sync := controlplane.NewSynchronizer(repo, dispatcher)
	adminHandler := controlplane.NewAdminHandler(repo, sync, dispatcher)
	engine := api.SetupRouter(dispatcher, adminHandler)

	// 1. Send verification code
	sendBody, _ := json.Marshal(map[string]string{
		"email":   "alice@example.com",
		"purpose": "register",
	})
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/api/v1/auth/send-verification-code", bytes.NewReader(sendBody))
	req.Header.Set("Content-Type", "application/json")
	engine.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("send verification code failed: %d %s", w.Code, w.Body.String())
	}

	var sendResp struct {
		Code    int    `json:"code"`
		DevCode string `json:"dev_code"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &sendResp)
	if sendResp.DevCode == "" {
		t.Fatalf("expected dev_code in response for test")
	}

	// 2. Register user
	regBody, _ := json.Marshal(map[string]string{
		"username": "alice",
		"email":    "alice@example.com",
		"password": "password123",
		"code":     sendResp.DevCode,
	})
	w = httptest.NewRecorder()
	req, _ = http.NewRequest("POST", "/api/v1/auth/register", bytes.NewReader(regBody))
	req.Header.Set("Content-Type", "application/json")
	engine.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("register failed: %d %s", w.Code, w.Body.String())
	}

	var regRes struct {
		Code int `json:"code"`
		Data struct {
			Token  string `json:"token"`
			APIKey string `json:"api_key"`
			User   struct {
				Username string  `json:"username"`
				Balance  float64 `json:"balance"`
				Role     string  `json:"role"`
			} `json:"user"`
		} `json:"data"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &regRes)
	if regRes.Data.User.Username != "alice" {
		t.Errorf("expected username alice, got %s", regRes.Data.User.Username)
	}
	if regRes.Data.User.Balance <= 0 {
		t.Errorf("expected initial trial balance > 0, got %f", regRes.Data.User.Balance)
	}
	if regRes.Data.APIKey == "" {
		t.Errorf("expected auto-generated API key")
	}
}

func TestUserWallet_RedemptionAndRecharge(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, err := storage.OpenDB(":memory:")
	if err != nil {
		t.Fatalf("failed to open db: %v", err)
	}
	defer db.Close()

	repo := storage.NewRepository(db)
	storage.SetGlobalRepository(repo)

	cfg := config.DefaultConfig()
	config.SetGlobalConfig(cfg)
	dispatcher := router.NewDispatcher(nil)
	sync := controlplane.NewSynchronizer(repo, dispatcher)
	adminHandler := controlplane.NewAdminHandler(repo, sync, dispatcher)
	engine := api.SetupRouter(dispatcher, adminHandler)

	// Create user
	_ = repo.CreateUser(&storage.UserRecord{
		Username:     "bob",
		Email:        "bob@test.com",
		PasswordHash: "hash",
		Role:         "user",
		Status:       "active",
		Balance:      10.0,
		GroupName:    "default",
	})

	token, _ := controlplane.GenerateAdminToken("bob", "user", time.Hour)

	// 1. Admin generates redemption cards
	genBody, _ := json.Marshal(map[string]interface{}{
		"count":  2,
		"amount": 25.5,
		"name":   "测试VIP充值卡",
	})
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/api/v1/admin/redemptions/generate", bytes.NewReader(genBody))
	req.Header.Set("Content-Type", "application/json")
	engine.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("generate redemptions failed: %d %s", w.Code, w.Body.String())
	}

	var genRes struct {
		Data []string `json:"data"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &genRes)
	if len(genRes.Data) != 2 {
		t.Fatalf("expected 2 codes, got %d", len(genRes.Data))
	}
	codeToRedeem := genRes.Data[0]

	// 2. User redeems card
	redeemBody, _ := json.Marshal(map[string]string{
		"code": codeToRedeem,
	})
	w = httptest.NewRecorder()
	req, _ = http.NewRequest("POST", "/api/v1/user/wallet/redeem", bytes.NewReader(redeemBody))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	engine.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("redeem code failed: %d %s", w.Code, w.Body.String())
	}

	// Check updated user balance
	user, _ := repo.GetUserByUsername("bob")
	if user.Balance != 35.5 { // 10.0 + 25.5
		t.Errorf("expected balance 35.5, got %f", user.Balance)
	}

	// 3. User performs sandbox top-up
	rechargeBody, _ := json.Marshal(map[string]interface{}{
		"amount": 50.0,
	})
	w = httptest.NewRecorder()
	req, _ = http.NewRequest("POST", "/api/v1/user/wallet/recharge/sandbox", bytes.NewReader(rechargeBody))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	engine.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("sandbox recharge failed: %d %s", w.Code, w.Body.String())
	}

	user, _ = repo.GetUserByUsername("bob")
	if user.Balance != 85.5 { // 35.5 + 50.0
		t.Errorf("expected balance 85.5, got %f", user.Balance)
	}

	// 4. Verify wallet endpoint
	w = httptest.NewRecorder()
	req, _ = http.NewRequest("GET", "/api/v1/user/wallet", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	engine.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("get wallet failed: %d %s", w.Code, w.Body.String())
	}
}

func TestModelGroupPricing(t *testing.T) {
	db, err := storage.OpenDB(":memory:")
	if err != nil {
		t.Fatalf("failed to open db: %v", err)
	}
	defer db.Close()

	repo := storage.NewRepository(db)
	engine := billing.NewEngine(repo)

	// Save default price for gpt-4o: prompt 18.0, completion 72.0
	_ = repo.SaveModelPrice(&storage.ModelPriceRecord{
		Model:           "gpt-4o",
		GroupName:       "default",
		PromptPrice:     18.0,
		CompletionPrice: 72.0,
		CacheReadPrice:  9.0,
		Currency:        "CNY",
		OffPeakEnabled:  false,
	})

	// Save VIP price for gpt-4o: prompt 10.0, completion 40.0
	_ = repo.SaveModelPrice(&storage.ModelPriceRecord{
		Model:           "gpt-4o",
		GroupName:       "vip",
		PromptPrice:     10.0,
		CompletionPrice: 40.0,
		CacheReadPrice:  5.0,
		Currency:        "CNY",
		OffPeakEnabled:  false,
	})

	_ = engine.ReloadPrices()

	// Calculate cost for 1,000,000 prompt tokens and 0 completion
	costDefault, _ := engine.CalculateCostWithGroup("gpt-4o", "default", 1000000, 0, 0)
	costVIP, _ := engine.CalculateCostWithGroup("gpt-4o", "vip", 1000000, 0, 0)

	if costDefault != 18.0 {
		t.Errorf("expected default cost 18.0, got %f", costDefault)
	}
	if costVIP != 10.0 {
		t.Errorf("expected VIP cost 10.0, got %f", costVIP)
	}
}

func TestUserLockAndQuotaExhaustion(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, err := storage.OpenDB(":memory:")
	if err != nil {
		t.Fatalf("failed to open db: %v", err)
	}
	defer db.Close()

	repo := storage.NewRepository(db)
	storage.SetGlobalRepository(repo)

	// Create user with 0 balance
	_ = repo.CreateUser(&storage.UserRecord{
		Username:     "broke_user",
		Email:        "broke@test.com",
		PasswordHash: "hash",
		Role:         "user",
		Status:       "active",
		Balance:      0.0, // Exhausted quota
		GroupName:    "default",
	})
	u, _ := repo.GetUserByUsername("broke_user")

	_ = repo.CreateVirtualKey(&storage.VirtualKeyRecord{
		Key:           "sk-broke-key",
		TenantID:      "broke_user",
		UserID:        u.ID,
		AllowedModels: []string{"*"},
		Status:        "active",
	})

	// Setup Config
	cfg := config.DefaultConfig()
	cfg.VirtualKeys = []model.VirtualKeyConfig{
		{
			Key:      "sk-broke-key",
			TenantID: "broke_user",
			UserID:   u.ID,
		},
	}
	cfg.HasConfiguredKeys = true
	config.SetGlobalConfig(cfg)

	dispatcher := router.NewDispatcher(nil)
	adminHandler := controlplane.NewAdminHandler(repo, nil, dispatcher)
	engine := api.SetupRouter(dispatcher, adminHandler)

	// 1. Request with broke user's key should return 402 Payment Required (insufficient_quota)
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/v1/chat/completions", bytes.NewReader([]byte(`{"model":"gpt-4o","messages":[{"role":"user","content":"hi"}]}`)))
	req.Header.Set("Authorization", "Bearer sk-broke-key")
	req.Header.Set("Content-Type", "application/json")
	engine.ServeHTTP(w, req)

	if w.Code != http.StatusPaymentRequired {
		t.Fatalf("expected 402 Payment Required, got %d %s", w.Code, w.Body.String())
	}

	var errResp struct {
		Error struct {
			Code    string `json:"code"`
			Type    string `json:"type"`
			Message string `json:"message"`
		} `json:"error"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &errResp)
	if errResp.Error.Code != "insufficient_quota" {
		t.Errorf("expected error code insufficient_quota, got %s", errResp.Error.Code)
	}

	// 2. Lock user and give positive balance -> should return 403 Forbidden (account_locked)
	_ = repo.UpdateUserBalance("broke_user", 100.0)
	_ = repo.UpdateUserStatus("broke_user", "locked")

	w = httptest.NewRecorder()
	req, _ = http.NewRequest("POST", "/v1/chat/completions", bytes.NewReader([]byte(`{"model":"gpt-4o","messages":[{"role":"user","content":"hi"}]}`)))
	req.Header.Set("Authorization", "Bearer sk-broke-key")
	req.Header.Set("Content-Type", "application/json")
	engine.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403 Forbidden for locked user, got %d %s", w.Code, w.Body.String())
	}
	_ = json.Unmarshal(w.Body.Bytes(), &errResp)
	if errResp.Error.Code != "account_locked" {
		t.Errorf("expected error code account_locked, got %s", errResp.Error.Code)
	}
}

func TestUserKeyScopingAndBatchPriceDelete(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, err := storage.OpenDB(":memory:")
	if err != nil {
		t.Fatalf("failed to open db: %v", err)
	}
	defer db.Close()

	repo := storage.NewRepository(db)
	storage.SetGlobalRepository(repo)

	// 1. Test BatchDeleteModelPriceKeys
	_ = repo.SaveModelPrice(&storage.ModelPriceRecord{
		Model:           "test-m1",
		GroupName:       "default",
		PromptPrice:     1.0,
		CompletionPrice: 2.0,
	})
	_ = repo.SaveModelPrice(&storage.ModelPriceRecord{
		Model:           "test-m1",
		GroupName:       "vip",
		PromptPrice:     0.8,
		CompletionPrice: 1.6,
	})
	_ = repo.SaveModelPrice(&storage.ModelPriceRecord{
		Model:           "test-m2",
		GroupName:       "default",
		PromptPrice:     3.0,
		CompletionPrice: 6.0,
	})

	// Delete test-m1 only in vip group
	deletedCount, err := repo.BatchDeleteModelPriceKeys([]storage.ModelPriceKey{
		{Model: "test-m1", Group: "vip"},
	})
	if err != nil || deletedCount != 1 {
		t.Fatalf("expected 1 deleted item, got %d, err: %v", deletedCount, err)
	}

	// test-m1 in default group should still exist
	pDef, err := repo.GetModelPriceWithGroup("test-m1", "default")
	if err != nil || pDef == nil {
		t.Fatalf("expected test-m1 in default group to still exist")
	}

	// test-m1 in vip group should be gone from exact records
	pVip, err := repo.GetModelPriceExact("test-m1", "vip")
	if err == nil && pVip != nil {
		t.Fatalf("expected test-m1 in vip group to be deleted")
	}

	// But calling GetModelPriceWithGroup should now fall back to default group!
	pFallback, err := repo.GetModelPriceWithGroup("test-m1", "vip")
	if err != nil || pFallback == nil || pFallback.GroupName != "default" {
		t.Fatalf("expected fallback to default group rate when vip rate deleted")
	}

	// 2. Test User Key Scoping
	dispatcher := router.NewDispatcher(nil)
	sync := controlplane.NewSynchronizer(repo, dispatcher)
	adminHandler := controlplane.NewAdminHandler(repo, sync, dispatcher)
	engine := api.SetupRouter(dispatcher, adminHandler)

	// Create user Bob
	_ = repo.CreateUser(&storage.UserRecord{
		Username:     "bob",
		Email:        "bob@test.com",
		PasswordHash: "hash",
		Role:         "user",
		Status:       "active",
		Balance:      50.0,
		GroupName:    "vip",
	})
	bob, _ := repo.GetUserByUsername("bob")
	bobToken, _ := controlplane.GenerateAdminToken("bob", "user", 24*time.Hour)

	// Bob creates a key via /api/v1/admin/keys
	keyBody, _ := json.Marshal(map[string]interface{}{
		"tenant_id": "bob-app",
	})
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/api/v1/admin/keys", bytes.NewReader(keyBody))
	req.Header.Set("Authorization", "Bearer "+bobToken)
	req.Header.Set("Content-Type", "application/json")
	engine.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("bob create key failed: %d %s", w.Code, w.Body.String())
	}
	var createdKeyResp struct {
		Code int                      `json:"code"`
		Data storage.VirtualKeyRecord `json:"data"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &createdKeyResp)
	if createdKeyResp.Data.UserID != bob.ID {
		t.Errorf("expected key user_id to be bob's ID %d, got %d", bob.ID, createdKeyResp.Data.UserID)
	}
	if createdKeyResp.Data.GroupName != "vip" {
		t.Errorf("expected key group_name to inherit bob's group vip, got %s", createdKeyResp.Data.GroupName)
	}

	// Bob lists keys -> should see 1 key
	w = httptest.NewRecorder()
	req, _ = http.NewRequest("GET", "/api/v1/admin/keys", nil)
	req.Header.Set("Authorization", "Bearer "+bobToken)
	engine.ServeHTTP(w, req)

	var listResp struct {
		Code int                         `json:"code"`
		Data []*storage.VirtualKeyRecord `json:"data"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &listResp)
	if len(listResp.Data) != 1 {
		t.Fatalf("expected bob to see 1 key, got %d", len(listResp.Data))
	}
}

func TestVirtualKey_BudgetLimitEnforcement(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, err := storage.OpenDB(":memory:")
	if err != nil {
		t.Fatalf("failed to open db: %v", err)
	}
	defer db.Close()

	repo := storage.NewRepository(db)
	storage.SetGlobalRepository(repo)

	cfg := config.DefaultConfig()
	config.SetGlobalConfig(cfg)
	dispatcher := router.NewDispatcher(nil)
	sync := controlplane.NewSynchronizer(repo, dispatcher)
	adminHandler := controlplane.NewAdminHandler(repo, sync, dispatcher)
	engine := api.SetupRouter(dispatcher, adminHandler)

	// Create user with high balance
	_ = repo.CreateUser(&storage.UserRecord{
		Username: "budget-user",
		Balance:  100.0,
		Role:     "user",
		Status:   "active",
	})
	user, _ := repo.GetUserByUsername("budget-user")

	// Create key with Budget: 10.0
	vkKey := "sk-nano-budget-test"
	_ = repo.CreateVirtualKey(&storage.VirtualKeyRecord{
		Key:      vkKey,
		TenantID: "budget-user",
		UserID:   user.ID,
		Budget:   10.0,
		Status:   "active",
	})
	_ = sync.ReloadFromDB()

	// 1. Under budget (used_cost = 2.0) -> request allowed to pass auth
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/v1/models", nil)
	req.Header.Set("Authorization", "Bearer "+vkKey)
	engine.ServeHTTP(w, req)
	if w.Code == http.StatusPaymentRequired {
		t.Fatalf("unexpected payment required when under budget: %d %s", w.Code, w.Body.String())
	}

	// 2. Simulate exceeding key budget: used_cost = 10.5
	_, _ = db.Exec(`UPDATE virtual_keys SET used_cost = 10.5 WHERE key = ?`, vkKey)

	w = httptest.NewRecorder()
	req, _ = http.NewRequest("GET", "/v1/models", nil)
	req.Header.Set("Authorization", "Bearer "+vkKey)
	engine.ServeHTTP(w, req)

	if w.Code != http.StatusPaymentRequired {
		t.Fatalf("expected 402 Payment Required for exceeded key budget, got %d %s", w.Code, w.Body.String())
	}
	var errResp struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &errResp)
	if errResp.Error.Code != "key_budget_exceeded" {
		t.Errorf("expected error code key_budget_exceeded, got %s", errResp.Error.Code)
	}
}

func TestModelsEndpoint_AllowedModelsFiltering(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, err := storage.OpenDB(":memory:")
	if err != nil {
		t.Fatalf("failed to open db: %v", err)
	}
	defer db.Close()

	repo := storage.NewRepository(db)
	storage.SetGlobalRepository(repo)

	cfg := config.DefaultConfig()
	config.SetGlobalConfig(cfg)

	// Add channel for two models in repository
	_ = repo.CreateChannel(&storage.ChannelRecord{
		Name:    "chan-1",
		Type:    "openai",
		BaseURL: "http://mock-upstream",
		Models:  []string{"gpt-4o", "claude-3-5-sonnet"},
		Status:  "active",
	})

	dispatcher := router.NewDispatcher(nil)
	sync := controlplane.NewSynchronizer(repo, dispatcher)
	adminHandler := controlplane.NewAdminHandler(repo, sync, dispatcher)
	engine := api.SetupRouter(dispatcher, adminHandler)

	// Create user
	_ = repo.CreateUser(&storage.UserRecord{
		Username: "model-user",
		Balance:  50.0,
		Role:     "user",
		Status:   "active",
	})
	user, _ := repo.GetUserByUsername("model-user")

	// Create key restricted to ONLY gpt-4o
	vkKey := "sk-nano-model-filter"
	_ = repo.CreateVirtualKey(&storage.VirtualKeyRecord{
		Key:           vkKey,
		TenantID:      "model-user",
		UserID:        user.ID,
		AllowedModels: []string{"gpt-4o"},
		Status:        "active",
	})
	_ = sync.ReloadFromDB()

	// 1. GET /v1/models should only return gpt-4o
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/v1/models", nil)
	req.Header.Set("Authorization", "Bearer "+vkKey)
	engine.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("GET /v1/models failed: %d %s", w.Code, w.Body.String())
	}
	var modelList model.ModelListResponse
	_ = json.Unmarshal(w.Body.Bytes(), &modelList)
	if len(modelList.Data) != 1 || modelList.Data[0].ID != "gpt-4o" {
		t.Fatalf("expected exactly [gpt-4o], got: %+v", modelList.Data)
	}

	// 2. GET /v1/models/gpt-4o should succeed
	w = httptest.NewRecorder()
	req, _ = http.NewRequest("GET", "/v1/models/gpt-4o", nil)
	req.Header.Set("Authorization", "Bearer "+vkKey)
	engine.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 for allowed model, got %d", w.Code)
	}

	// 3. GET /v1/models/claude-3-5-sonnet should be forbidden 403
	w = httptest.NewRecorder()
	req, _ = http.NewRequest("GET", "/v1/models/claude-3-5-sonnet", nil)
	req.Header.Set("Authorization", "Bearer "+vkKey)
	engine.ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403 Forbidden for restricted model, got %d %s", w.Code, w.Body.String())
	}
}

func TestUser_SelfServiceChangePassword(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, err := storage.OpenDB(":memory:")
	if err != nil {
		t.Fatalf("failed to open db: %v", err)
	}
	defer db.Close()

	repo := storage.NewRepository(db)
	storage.SetGlobalRepository(repo)

	cfg := config.DefaultConfig()
	config.SetGlobalConfig(cfg)
	dispatcher := router.NewDispatcher(nil)
	sync := controlplane.NewSynchronizer(repo, dispatcher)
	adminHandler := controlplane.NewAdminHandler(repo, sync, dispatcher)
	engine := api.SetupRouter(dispatcher, adminHandler)

	// Register user
	pwHash, _ := bcrypt.GenerateFromPassword([]byte("secret123"), bcrypt.DefaultCost)
	_ = repo.CreateUser(&storage.UserRecord{
		Username:     "pw-user",
		PasswordHash: string(pwHash),
		Role:         "user",
		Status:       "active",
		Balance:      10.0,
	})
	user, _ := repo.GetUserByUsername("pw-user")

	token, _ := controlplane.GenerateAdminToken(user.Username, user.Role, 24*time.Hour)

	// Call POST /api/v1/user/password
	pwBody, _ := json.Marshal(map[string]string{
		"old_password": "secret123",
		"new_password": "newsecret456",
	})
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/api/v1/user/password", bytes.NewReader(pwBody))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	engine.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("change password failed: %d %s", w.Code, w.Body.String())
	}
}

