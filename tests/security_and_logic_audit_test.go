package tests

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/ifnodoraemon/nano-gateway/internal/api"
	"github.com/ifnodoraemon/nano-gateway/internal/config"
	"github.com/ifnodoraemon/nano-gateway/internal/controlplane"
	"github.com/ifnodoraemon/nano-gateway/internal/model"
	"github.com/ifnodoraemon/nano-gateway/internal/router"
	"github.com/ifnodoraemon/nano-gateway/internal/storage"
	"golang.org/x/crypto/bcrypt"
)

// Helper setup for router test
func setupAuditTestEnv(t *testing.T) (*storage.Repository, *gin.Engine, *controlplane.AdminHandler) {
	gin.SetMode(gin.TestMode)
	db, err := storage.OpenDB(":memory:")
	if err != nil {
		t.Fatalf("failed to open memory db: %v", err)
	}

	repo := storage.NewRepository(db)
	storage.SetGlobalRepository(repo)

	cfg := config.DefaultConfig()
	config.SetGlobalConfig(cfg)
	dispatcher := router.NewDispatcher(nil)
	sync := controlplane.NewSynchronizer(repo, dispatcher)
	adminHandler := controlplane.NewAdminHandler(repo, sync, dispatcher)
	engine := api.SetupRouter(dispatcher, adminHandler)

	return repo, engine, adminHandler
}

// 1. Locked User Control Plane Access Rejection
func TestSecurity_LockedUserCannotAccessControlPlane(t *testing.T) {
	repo, engine, _ := setupAuditTestEnv(t)

	// Create user
	hash, _ := bcrypt.GenerateFromPassword([]byte("user123"), bcrypt.DefaultCost)
	user := &storage.UserRecord{
		Username:     "activeuser",
		Email:        "active@example.com",
		PasswordHash: string(hash),
		Role:         "user",
		Status:       "active",
		Balance:      20.0,
	}
	_ = repo.CreateUser(user)

	// Generate valid token
	token, err := controlplane.GenerateAdminToken("activeuser", "user", 24*time.Hour)
	if err != nil {
		t.Fatalf("failed to generate token: %v", err)
	}

	// Request while active should succeed
	w1 := httptest.NewRecorder()
	req1, _ := http.NewRequest("GET", "/api/v1/user/wallet", nil)
	req1.Header.Set("Authorization", "Bearer "+token)
	engine.ServeHTTP(w1, req1)
	if w1.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for active user, got %d", w1.Code)
	}

	// Admin locks user
	_ = repo.UpdateUserStatus("activeuser", "locked")

	// Same token should now be rejected with 403 Forbidden
	w2 := httptest.NewRecorder()
	req2, _ := http.NewRequest("GET", "/api/v1/user/wallet", nil)
	req2.Header.Set("Authorization", "Bearer "+token)
	engine.ServeHTTP(w2, req2)
	if w2.Code != http.StatusForbidden {
		t.Fatalf("expected 403 Forbidden for locked user token, got %d", w2.Code)
	}
}

// 2. Privilege Escalation Prevention: Non-admin rejected on admin-only routes
func TestSecurity_NonAdminCannotAccessAdminEndpoints(t *testing.T) {
	repo, engine, _ := setupAuditTestEnv(t)

	hash, _ := bcrypt.GenerateFromPassword([]byte("pass123"), bcrypt.DefaultCost)
	user := &storage.UserRecord{
		Username:     "normaluser",
		Email:        "normal@example.com",
		PasswordHash: string(hash),
		Role:         "user",
		Status:       "active",
	}
	_ = repo.CreateUser(user)

	token, _ := controlplane.GenerateAdminToken("normaluser", "user", 24*time.Hour)

	adminEndpoints := []struct {
		method string
		path   string
		body   string
	}{
		{"GET", "/api/v1/admin/users", ""},
		{"POST", "/api/v1/admin/channels", `{"name":"test","base_url":"http://x","type":"openai"}`},
		{"GET", "/api/v1/admin/channels", ""},
		{"GET", "/api/v1/admin/redemptions", ""},
		{"POST", "/api/v1/admin/redemptions/generate", `{"count":1,"amount":10}`},
		{"DELETE", "/api/v1/admin/logs/1", ""},
		{"POST", "/api/v1/admin/logs/clear", ""},
		{"POST", "/api/v1/admin/pricing", `{"model":"gpt-4","prompt_price":1}`},
	}

	for _, ep := range adminEndpoints {
		w := httptest.NewRecorder()
		var bodyReader *bytes.Reader
		if ep.body != "" {
			bodyReader = bytes.NewReader([]byte(ep.body))
		} else {
			bodyReader = bytes.NewReader(nil)
		}
		req, _ := http.NewRequest(ep.method, ep.path, bodyReader)
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Content-Type", "application/json")
		engine.ServeHTTP(w, req)

		if w.Code != http.StatusForbidden {
			t.Errorf("expected 403 Forbidden for non-admin accessing %s %s, got %d", ep.method, ep.path, w.Code)
		}
	}
}

// 3. IDOR Prevention in Virtual Key Batch Operations
func TestSecurity_IDOR_VirtualKeyBatchOperations(t *testing.T) {
	repo, engine, _ := setupAuditTestEnv(t)

	// User A
	uA := &storage.UserRecord{Username: "userA", Role: "user", Status: "active"}
	_ = repo.CreateUser(uA)
	keyA := &storage.VirtualKeyRecord{
		Key:      "sk-nano-userA-key1",
		TenantID: "userA",
		UserID:   uA.ID,
		Status:   "active",
		RPM:      60,
	}
	_ = repo.CreateVirtualKey(keyA)

	// User B
	uB := &storage.UserRecord{Username: "userB", Role: "user", Status: "active"}
	_ = repo.CreateUser(uB)
	keyB := &storage.VirtualKeyRecord{
		Key:      "sk-nano-userB-key2",
		TenantID: "userB",
		UserID:   uB.ID,
		Status:   "active",
		RPM:      60,
	}
	_ = repo.CreateVirtualKey(keyB)

	tokenB, _ := controlplane.GenerateAdminToken("userB", "user", 24*time.Hour)

	// User B tries to batch-delete User A's key
	delPayload, _ := json.Marshal(map[string]interface{}{
		"ids": []int64{keyA.ID},
	})
	w1 := httptest.NewRecorder()
	req1, _ := http.NewRequest("POST", "/api/v1/admin/keys/batch-delete", bytes.NewReader(delPayload))
	req1.Header.Set("Authorization", "Bearer "+tokenB)
	req1.Header.Set("Content-Type", "application/json")
	engine.ServeHTTP(w1, req1)

	if w1.Code != http.StatusForbidden {
		t.Fatalf("expected 403 Forbidden when user B tries to delete user A's key, got %d", w1.Code)
	}

	// Verify key A is still alive
	foundA, err := repo.GetVirtualKey(keyA.ID)
	if err != nil || foundA == nil {
		t.Fatalf("key A was deleted despite IDOR attempt!")
	}

	// User B tries to batch-status toggle User A's key
	statusPayload, _ := json.Marshal(map[string]interface{}{
		"ids":    []int64{keyA.ID},
		"status": "disabled",
	})
	w2 := httptest.NewRecorder()
	req2, _ := http.NewRequest("POST", "/api/v1/admin/keys/batch-status", bytes.NewReader(statusPayload))
	req2.Header.Set("Authorization", "Bearer "+tokenB)
	req2.Header.Set("Content-Type", "application/json")
	engine.ServeHTTP(w2, req2)

	if w2.Code != http.StatusForbidden {
		t.Fatalf("expected 403 Forbidden when user B tries to modify user A's key, got %d", w2.Code)
	}

	// Verify key A status was not changed to disabled
	foundA2, _ := repo.GetVirtualKey(keyA.ID)
	if foundA2.Status != "active" {
		t.Fatalf("key A status was modified despite IDOR attempt! status=%s", foundA2.Status)
	}
}

// 4. Virtual Key RPM = 0 is preserved as unlimited
func TestLogic_VirtualKeyRPM0_Unlimited(t *testing.T) {
	repo, _, _ := setupAuditTestEnv(t)

	key := &storage.VirtualKeyRecord{
		Key:      "sk-nano-unlimited-rpm",
		TenantID: "unlimited-tenant",
		RPM:      0, // Unlimited
		TPM:      0,
		Status:   "active",
	}
	if err := repo.CreateVirtualKey(key); err != nil {
		t.Fatalf("failed to create virtual key: %v", err)
	}

	saved, err := repo.GetVirtualKeyByKey("sk-nano-unlimited-rpm")
	if err != nil {
		t.Fatalf("failed to get virtual key: %v", err)
	}

	if saved.RPM != 0 {
		t.Fatalf("expected RPM to remain 0 (unlimited), got %d", saved.RPM)
	}
}

// 5. Wallet Deduction fallback by tenant_id when user_id is 0
func TestLogic_RecordUsageLog_FallbackDeduction(t *testing.T) {
	repo, _, _ := setupAuditTestEnv(t)

	user := &storage.UserRecord{
		Username: "bobtenant",
		Email:    "bob@example.com",
		Role:     "user",
		Status:   "active",
		Balance:  10.0,
	}
	_ = repo.CreateUser(user)

	// Key with UserID == 0
	key := &storage.VirtualKeyRecord{
		Key:      "sk-nano-legacy-bob",
		TenantID: "bobtenant",
		UserID:   0,
		Status:   "active",
	}
	_ = repo.CreateVirtualKey(key)

	// Record usage log with Cost = 2.5
	log := &storage.UsageLogRecord{
		VirtualKey:  "sk-nano-legacy-bob",
		TenantID:    "bobtenant",
		Model:       "gpt-4o",
		PromptTokens: 100,
		CompletionTokens: 50,
		TotalTokens: 150,
		Cost:        2.5,
		StatusCode:  200,
	}
	if err := repo.RecordUsageLog(log); err != nil {
		t.Fatalf("failed to record usage log: %v", err)
	}

	// Check user balance
	updatedUser, err := repo.GetUserByUsername("bobtenant")
	if err != nil {
		t.Fatalf("failed to get user: %v", err)
	}

	expectedBalance := 7.5
	if fmt.Sprintf("%.2f", updatedUser.Balance) != fmt.Sprintf("%.2f", expectedBalance) {
		t.Fatalf("expected user balance to deduct to %.2f, got %.2f", expectedBalance, updatedUser.Balance)
	}
}

// 6. Self-Service Key Creation options: name, unlimited RPM, custom budget
func TestLogic_CreateUserKey_Options(t *testing.T) {
	repo, engine, _ := setupAuditTestEnv(t)

	user := &storage.UserRecord{
		Username: "charlie",
		Email:    "charlie@example.com",
		Role:     "user",
		Status:   "active",
		Balance:  50.0,
	}
	_ = repo.CreateUser(user)

	token, _ := controlplane.GenerateAdminToken("charlie", "user", 24*time.Hour)

	payload, _ := json.Marshal(map[string]interface{}{
		"name":           "production-bot",
		"rpm":            0, // unlimited
		"budget":         88.5,
		"allowed_models": []string{"gpt-4o", "claude-3-5-sonnet"},
	})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/api/v1/user/keys", bytes.NewReader(payload))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	engine.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d: %s", w.Code, w.Body.String())
	}

	var resp struct {
		Code int                      `json:"code"`
		Data storage.VirtualKeyRecord `json:"data"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &resp)

	if resp.Data.TenantID != "production-bot" {
		t.Errorf("expected tenant_id to be 'production-bot', got '%s'", resp.Data.TenantID)
	}
	if resp.Data.RPM != 0 {
		t.Errorf("expected RPM to be 0 (unlimited), got %d", resp.Data.RPM)
	}
	if resp.Data.Budget != 88.5 {
		t.Errorf("expected Budget to be 88.5, got %f", resp.Data.Budget)
	}
	if len(resp.Data.AllowedModels) != 2 {
		t.Errorf("expected 2 allowed models, got %d", len(resp.Data.AllowedModels))
	}
}

// 7. Role management & RBAC: admin can promote/demote users, cannot demote self or root admin, non-admin rejected
func TestSecurity_UpdateUserRole_RBAC(t *testing.T) {
	repo, engine, _ := setupAuditTestEnv(t)

	// Admin account
	admin := &storage.UserRecord{Username: "superadmin", Role: "admin", Status: "active"}
	_ = repo.CreateUser(admin)
	adminToken, _ := controlplane.GenerateAdminToken("superadmin", "admin", 24*time.Hour)

	// Target user
	userDave := &storage.UserRecord{Username: "dave", Role: "user", Status: "active"}
	_ = repo.CreateUser(userDave)

	// Non-admin token
	daveToken, _ := controlplane.GenerateAdminToken("dave", "user", 24*time.Hour)

	// 1. Non-admin trying to update role should be 403
	bodyUser, _ := json.Marshal(map[string]string{"role": "admin"})
	w1 := httptest.NewRecorder()
	req1, _ := http.NewRequest("POST", "/api/v1/admin/users/dave/role", bytes.NewReader(bodyUser))
	req1.Header.Set("Authorization", "Bearer "+daveToken)
	req1.Header.Set("Content-Type", "application/json")
	engine.ServeHTTP(w1, req1)
	if w1.Code != http.StatusForbidden {
		t.Fatalf("expected 403 Forbidden for non-admin role update, got %d", w1.Code)
	}

	// 2. Admin promoting Dave to admin
	w2 := httptest.NewRecorder()
	req2, _ := http.NewRequest("POST", "/api/v1/admin/users/dave/role", bytes.NewReader(bodyUser))
	req2.Header.Set("Authorization", "Bearer "+adminToken)
	req2.Header.Set("Content-Type", "application/json")
	engine.ServeHTTP(w2, req2)
	if w2.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for admin promoting Dave, got %d", w2.Code)
	}

	updatedDave, _ := repo.GetUserByUsername("dave")
	if updatedDave.Role != "admin" {
		t.Fatalf("expected Dave role to become admin, got %s", updatedDave.Role)
	}

	// 3. Admin demoting self should be rejected (400 Bad Request)
	bodyDemote, _ := json.Marshal(map[string]string{"role": "user"})
	w3 := httptest.NewRecorder()
	req3, _ := http.NewRequest("POST", "/api/v1/admin/users/superadmin/role", bytes.NewReader(bodyDemote))
	req3.Header.Set("Authorization", "Bearer "+adminToken)
	req3.Header.Set("Content-Type", "application/json")
	engine.ServeHTTP(w3, req3)
	if w3.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 Bad Request when admin tries to demote self, got %d", w3.Code)
	}

	// 4. Admin demoting Dave back to user
	w4 := httptest.NewRecorder()
	req4, _ := http.NewRequest("POST", "/api/v1/admin/users/dave/role", bytes.NewReader(bodyDemote))
	req4.Header.Set("Authorization", "Bearer "+adminToken)
	req4.Header.Set("Content-Type", "application/json")
	engine.ServeHTTP(w4, req4)
	if w4.Code != http.StatusOK {
		t.Fatalf("expected 200 OK when admin demotes Dave to user, got %d", w4.Code)
	}

	demotedDave, _ := repo.GetUserByUsername("dave")
	if demotedDave.Role != "user" {
		t.Fatalf("expected Dave role to become user, got %s", demotedDave.Role)
	}
}

// 8. Circular Fallback Prevention Test
func TestDispatcher_CircularFallback_Prevention(t *testing.T) {
	dispatcher := router.NewDispatcher(nil)
	// Set circular fallback: model-x -> model-y -> model-x
	dispatcher.SetModelFallbacks(map[string]string{
		"model-x": "model-y",
		"model-y": "model-x",
	})

	req := &model.ChatCompletionRequest{
		Model: "model-x",
		Messages: []model.ChatMessage{
			{Role: "user", Content: "Hello test"},
		},
	}

	// 1. Non-streaming dispatch should cleanly abort and return cycle error
	resp, err := dispatcher.Dispatch(context.Background(), req)
	if resp != nil {
		t.Fatalf("expected nil response on circular fallback, got %v", resp)
	}
	if err == nil {
		t.Fatalf("expected error on circular fallback, got nil")
	}
	if !strings.Contains(err.Error(), "cycle detected") {
		t.Fatalf("expected error to mention 'cycle detected', got: %v", err)
	}

	// 2. Streaming dispatch should also cleanly abort without panic or stack exhaustion
	streamChan, sErr := dispatcher.DispatchStream(context.Background(), req)
	if streamChan != nil {
		t.Fatalf("expected nil channel on circular fallback stream, got %v", streamChan)
	}
	if sErr == nil {
		t.Fatalf("expected stream error on circular fallback, got nil")
	}
	if !strings.Contains(sErr.Error(), "cycle detected") {
		t.Fatalf("expected error to mention 'cycle detected', got: %v", sErr)
	}
}

// 9. Authoritative DB Budget Check Test
func TestVirtualKey_AuthoritativeBudgetCheck(t *testing.T) {
	repo, engine, _ := setupAuditTestEnv(t)

	// Create user
	user := &storage.UserRecord{
		Username:     "budgetuser",
		Email:        "budget@example.com",
		PasswordHash: "pass",
		Role:         "user",
		Status:       "active",
		Balance:      100.0,
	}
	_ = repo.CreateUser(user)
	createdUser, _ := repo.GetUserByUsername("budgetuser")

	// Create virtual key in DB with a strict budget of 10.0 and already used 12.0
	vk := &storage.VirtualKeyRecord{
		Key:       "sk-audit-budget-test-key",
		TenantID:  "budgetuser",
		UserID:    createdUser.ID,
		Budget:    10.0,
		UsedCost:  12.0,
		Status:    "active",
		GroupName: "default",
	}
	_ = repo.CreateVirtualKey(vk)

	// In-memory config has Budget = 0 (not aware yet of the DB budget limit)
	cfg := config.DefaultConfig()
	cfg.VirtualKeys = []model.VirtualKeyConfig{
		{
			Key:      "sk-audit-budget-test-key",
			TenantID: "budgetuser",
			UserID:   createdUser.ID,
			Budget:   0.0, // Stale or missing in memory
		},
	}
	config.SetGlobalConfig(cfg)

	w := httptest.NewRecorder()
	reqBody, _ := json.Marshal(map[string]interface{}{
		"model": "gpt-4o",
		"messages": []map[string]string{
			{"role": "user", "content": "ping"},
		},
	})
	httpReq, _ := http.NewRequest("POST", "/v1/chat/completions", bytes.NewReader(reqBody))
	httpReq.Header.Set("Authorization", "Bearer sk-audit-budget-test-key")
	httpReq.Header.Set("Content-Type", "application/json")
	engine.ServeHTTP(w, httpReq)

	// Should be rejected with 402 Payment Required due to DB authoritative budget
	if w.Code != http.StatusPaymentRequired {
		t.Fatalf("expected 402 Payment Required due to authoritative DB budget check, got %d (body: %s)", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "key_budget_exceeded") {
		t.Fatalf("expected key_budget_exceeded error code in response, got %s", w.Body.String())
	}
}

// 10. Database Secondary Indexes Verification Test
func TestDB_SecondaryIndexes_Created(t *testing.T) {
	db, err := storage.OpenDB(":memory:")
	if err != nil {
		t.Fatalf("failed to open memory db: %v", err)
	}

	requiredIndexes := []string{
		"idx_vk_user_id",
		"idx_vk_tenant_id",
		"idx_usage_vk",
		"idx_usage_tenant",
		"idx_usage_model",
		"idx_users_email",
	}

	for _, idx := range requiredIndexes {
		var name string
		err := db.QueryRow("SELECT name FROM sqlite_master WHERE type = 'index' AND name = ?", idx).Scan(&name)
		if err != nil {
			t.Errorf("expected index %s to be created, but query returned error: %v", idx, err)
		}
		if name != idx {
			t.Errorf("expected index %s, got %s", idx, name)
		}
	}
}
