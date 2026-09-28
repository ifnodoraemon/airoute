package tests

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ifnodoraemon/airoute/internal/api"
	"github.com/ifnodoraemon/airoute/internal/controlplane"
	"github.com/ifnodoraemon/airoute/internal/router"
	"github.com/ifnodoraemon/airoute/internal/storage"
)

func TestAdminAuth_LoginAndProtectedRoutes(t *testing.T) {
	db, err := storage.OpenDB(":memory:")
	if err != nil {
		t.Fatalf("failed to open in-memory db: %v", err)
	}
	repo := storage.NewRepository(db)
	dispatcher := router.NewDispatcher(nil)
	sync := controlplane.NewSynchronizer(repo, dispatcher)
	adminHandler := controlplane.NewAdminHandler(repo, sync, dispatcher)

	// Ensure default admin user is initialized
	if err := repo.EnsureDefaultAdmin("admin", "admin123"); err != nil {
		t.Fatalf("failed to ensure default admin: %v", err)
	}

	engine := api.SetupRouter(dispatcher, adminHandler)

	// 1. Attempt login with WRONG password
	badLoginPayload, _ := json.Marshal(map[string]string{
		"username": "admin",
		"password": "wrongpassword",
	})
	reqBad := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", bytes.NewReader(badLoginPayload))
	reqBad.Header.Set("Content-Type", "application/json")
	wBad := httptest.NewRecorder()
	engine.ServeHTTP(wBad, reqBad)

	if wBad.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 Unauthorized for bad password, got %d", wBad.Code)
	}

	// 2. Attempt login with CORRECT password
	goodLoginPayload, _ := json.Marshal(map[string]string{
		"username": "admin",
		"password": "admin123",
	})
	reqGood := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", bytes.NewReader(goodLoginPayload))
	reqGood.Header.Set("Content-Type", "application/json")
	wGood := httptest.NewRecorder()
	engine.ServeHTTP(wGood, reqGood)

	if wGood.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for correct password, got %d: %s", wGood.Code, wGood.Body.String())
	}

	var resp struct {
		Code int `json:"code"`
		Data struct {
			Token string `json:"token"`
			User  struct {
				Username string `json:"username"`
				Role     string `json:"role"`
			} `json:"user"`
		} `json:"data"`
	}
	if err := json.Unmarshal(wGood.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode login response: %v", err)
	}
	if resp.Data.Token == "" {
		t.Fatalf("expected non-empty auth token")
	}
	if resp.Data.User.Username != "admin" {
		t.Fatalf("expected username 'admin', got '%s'", resp.Data.User.Username)
	}

	// 3. Verify /api/v1/user/me using token
	reqMe := httptest.NewRequest(http.MethodGet, "/api/v1/user/me", nil)
	reqMe.Header.Set("Authorization", "Bearer "+resp.Data.Token)
	wMe := httptest.NewRecorder()
	engine.ServeHTTP(wMe, reqMe)

	if wMe.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for /auth/me, got %d", wMe.Code)
	}

	// 4. Verify token verification directly
	claims, err := controlplane.VerifyAdminToken(resp.Data.Token)
	if err != nil {
		t.Fatalf("token verification failed: %v", err)
	}
	if claims.Username != "admin" {
		t.Fatalf("expected claim username 'admin', got '%s'", claims.Username)
	}

	// 5. Test ListUsers
	reqUsers := httptest.NewRequest(http.MethodGet, "/api/v1/admin/users", nil)
	reqUsers.Header.Set("Authorization", "Bearer "+resp.Data.Token)
	wUsers := httptest.NewRecorder()
	engine.ServeHTTP(wUsers, reqUsers)
	if wUsers.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for /api/v1/admin/users, got %d", wUsers.Code)
	}

	// 6. Test CreateUser
	createUserPayload, _ := json.Marshal(map[string]string{
		"username": "operator1",
		"password": "operator123456",
		"role":     "operator",
	})
	reqCreateUser := httptest.NewRequest(http.MethodPost, "/api/v1/admin/users", bytes.NewReader(createUserPayload))
	reqCreateUser.Header.Set("Content-Type", "application/json")
	reqCreateUser.Header.Set("Authorization", "Bearer "+resp.Data.Token)
	wCreateUser := httptest.NewRecorder()
	engine.ServeHTTP(wCreateUser, reqCreateUser)
	if wCreateUser.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for CreateUser, got %d: %s", wCreateUser.Code, wCreateUser.Body.String())
	}

	// Verify newly created user can log in
	opLoginPayload, _ := json.Marshal(map[string]string{
		"username": "operator1",
		"password": "operator123456",
	})
	reqOpLogin := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", bytes.NewReader(opLoginPayload))
	reqOpLogin.Header.Set("Content-Type", "application/json")
	wOpLogin := httptest.NewRecorder()
	engine.ServeHTTP(wOpLogin, reqOpLogin)
	if wOpLogin.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for operator login, got %d", wOpLogin.Code)
	}

	// 7. Test ResetUserPassword
	resetPayload, _ := json.Marshal(map[string]string{
		"new_password": "operator_newpass999",
	})
	reqReset := httptest.NewRequest(http.MethodPost, "/api/v1/admin/users/operator1/password", bytes.NewReader(resetPayload))
	reqReset.Header.Set("Content-Type", "application/json")
	reqReset.Header.Set("Authorization", "Bearer "+resp.Data.Token)
	wReset := httptest.NewRecorder()
	engine.ServeHTTP(wReset, reqReset)
	if wReset.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for ResetUserPassword, got %d: %s", wReset.Code, wReset.Body.String())
	}

	// Verify login with new password
	newOpLoginPayload, _ := json.Marshal(map[string]string{
		"username": "operator1",
		"password": "operator_newpass999",
	})
	reqNewOpLogin := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", bytes.NewReader(newOpLoginPayload))
	reqNewOpLogin.Header.Set("Content-Type", "application/json")
	wNewOpLogin := httptest.NewRecorder()
	engine.ServeHTTP(wNewOpLogin, reqNewOpLogin)
	if wNewOpLogin.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for operator login with reset password, got %d", wNewOpLogin.Code)
	}

	// 8. Test DeleteUser (cannot delete self 'admin')
	reqDelAdmin := httptest.NewRequest(http.MethodDelete, "/api/v1/admin/users/admin", nil)
	reqDelAdmin.Header.Set("Authorization", "Bearer "+resp.Data.Token)
	wDelAdmin := httptest.NewRecorder()
	engine.ServeHTTP(wDelAdmin, reqDelAdmin)
	if wDelAdmin.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 Bad Request when deleting self admin, got %d", wDelAdmin.Code)
	}

	// Delete operator1
	reqDelOp := httptest.NewRequest(http.MethodDelete, "/api/v1/admin/users/operator1", nil)
	reqDelOp.Header.Set("Authorization", "Bearer "+resp.Data.Token)
	wDelOp := httptest.NewRecorder()
	engine.ServeHTTP(wDelOp, reqDelOp)
	if wDelOp.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for deleting operator1, got %d", wDelOp.Code)
	}
}
