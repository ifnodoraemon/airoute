package tests

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/ifnodoraemon/airoute/internal/api"
	"github.com/ifnodoraemon/airoute/internal/config"
	"github.com/ifnodoraemon/airoute/internal/controlplane"
	"github.com/ifnodoraemon/airoute/internal/router"
	"github.com/ifnodoraemon/airoute/internal/storage"
	"golang.org/x/crypto/bcrypt"
)

// TestP0_BatchRecordUsageLogs_NoDuplicateDeduction verifies that multiple orphan logs for a tenant
// do NOT trigger the duplicate deduction multiplier bug.
func TestP0_BatchRecordUsageLogs_NoDuplicateDeduction(t *testing.T) {
	db, err := storage.OpenDB(":memory:")
	if err != nil {
		t.Fatalf("failed to open memory db: %v", err)
	}
	defer db.Close()

	repo := storage.NewRepository(db)

	hash, _ := bcrypt.GenerateFromPassword([]byte("Password123!"), bcrypt.DefaultCost)
	user := &storage.UserRecord{
		Username:     "bob_tenant",
		Email:        "bob@example.com",
		Role:         "user",
		Balance:      100.0,
		PasswordHash: string(hash),
	}
	if err := repo.CreateUser(user); err != nil {
		t.Fatalf("failed to create user: %v", err)
	}

	// 2 orphan logs (key not in api_keys) for bob_tenant, each costing 5.0
	logs := []*storage.UsageLogRecord{
		{
			TraceID:      "tr-1",
			APIKey:       "key-orphan-1",
			TenantID:     "bob_tenant",
			Model:        "deepseek-chat",
			PromptTokens: 100,
			Cost:         5.0,
		},
		{
			TraceID:      "tr-2",
			APIKey:       "key-orphan-2",
			TenantID:     "bob_tenant",
			Model:        "deepseek-chat",
			PromptTokens: 100,
			Cost:         5.0,
		},
	}

	if err := repo.BatchRecordUsageLogs(logs); err != nil {
		t.Fatalf("BatchRecordUsageLogs failed: %v", err)
	}

	// Balance should be exactly 100.0 - 5.0 - 5.0 = 90.0
	updatedUser, err := repo.GetUserByUsername("bob_tenant")
	if err != nil {
		t.Fatalf("failed to get user: %v", err)
	}
	if updatedUser.Balance != 90.0 {
		t.Fatalf("expected balance to be 90.0, but got %f (duplicate deduction detected!)", updatedUser.Balance)
	}
}

// TestP1_UserRedeemGiftCode verifies that an authenticated user can redeem a card and get balance credited.
func TestP1_UserRedeemGiftCode(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, err := storage.OpenDB(":memory:")
	if err != nil {
		t.Fatalf("failed to open memory db: %v", err)
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

	// Create a user
	passHash, _ := bcrypt.GenerateFromPassword([]byte("UserPass123!"), bcrypt.DefaultCost)
	user := &storage.UserRecord{
		Username:     "redeem_user",
		Email:        "redeem@example.com",
		Role:         "user",
		Balance:      10.0,
		PasswordHash: string(passHash),
	}
	_ = repo.CreateUser(user)

	// Create a gift code for 66.0
	code := &storage.RedemptionCodeRecord{
		Code:   "CARD-TEST-8888-6666",
		Name:   "New Year Bonus",
		Amount: 66.0,
		Status: "active",
	}
	if err := repo.CreateRedemptionCode(code); err != nil {
		t.Fatalf("failed to create redemption code: %v", err)
	}

	// Login as redeem_user to get token
	loginBody, _ := json.Marshal(map[string]string{
		"username": "redeem_user",
		"password": "UserPass123!",
	})
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/api/v1/auth/login", bytes.NewReader(loginBody))
	req.Header.Set("Content-Type", "application/json")
	engine.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("login failed: %d %s", w.Code, w.Body.String())
	}

	var loginResp struct {
		Code int `json:"code"`
		Data struct {
			Token string `json:"token"`
		} `json:"data"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &loginResp)
	if loginResp.Data.Token == "" {
		t.Fatalf("expected login token, got empty response: %s", w.Body.String())
	}

	// Redeem code via POST /api/v1/user/wallet/redeem
	redeemBody, _ := json.Marshal(map[string]string{
		"code": "CARD-TEST-8888-6666",
	})
	w = httptest.NewRecorder()
	req, _ = http.NewRequest("POST", "/api/v1/user/wallet/redeem", bytes.NewReader(redeemBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+loginResp.Data.Token)
	engine.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("redeem failed: %d %s", w.Code, w.Body.String())
	}

	// Verify updated balance: 10 + 66 = 76.0
	updatedUser, _ := repo.GetUserByUsername("redeem_user")
	if updatedUser.Balance != 76.0 {
		t.Fatalf("expected user balance to be 76.0, got %f", updatedUser.Balance)
	}

	// Redeeming a second time should fail
	w = httptest.NewRecorder()
	req, _ = http.NewRequest("POST", "/api/v1/user/wallet/redeem", bytes.NewReader(redeemBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+loginResp.Data.Token)
	engine.ServeHTTP(w, req)

	if w.Code == http.StatusOK {
		t.Fatalf("expected double redeem to fail, but got 200 OK")
	}
}
