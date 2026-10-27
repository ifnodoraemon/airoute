package controlplane

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/ifnodoraemon/airoute/internal/storage"
)

func newSecurityTestHandler(t *testing.T) (*AdminHandler, *storage.Repository) {
	t.Helper()
	db, err := storage.OpenDB(":memory:")
	if err != nil {
		t.Fatalf("open memory db: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	repo := storage.NewRepository(db)
	return NewAdminHandler(repo, nil, nil), repo
}

func oauthCallbackRequest(t *testing.T, handler *AdminHandler, body string) *httptest.ResponseRecorder {
	t.Helper()
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/api/v1/auth/oauth/github/callback", strings.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Params = gin.Params{{Key: "provider", Value: "github"}}
	handler.OAuthCallback(c)
	return w
}

// TestSimulatedOAuthCannotTakeOverAdminViaEmail pins the escalation fix: a
// caller could pass an arbitrary non-empty code with simulated=false to reach
// the dev branch, then supply the administrator's email to be resolved to the
// admin account. The branch must be treated as simulated regardless of the code
// value, and must never authenticate an existing privileged account.
func TestSimulatedOAuthCannotTakeOverAdminViaEmail(t *testing.T) {
	t.Setenv("ENABLE_SIMULATED_OAUTH", "true")
	t.Setenv("GITHUB_CLIENT_ID", "")
	t.Setenv("GITHUB_CLIENT_SECRET", "")

	handler, repo := newSecurityTestHandler(t)
	admin := &storage.UserRecord{
		Username:     "admin",
		Email:        "admin@example.com",
		PasswordHash: "x",
		Role:         "admin",
		Status:       "active",
	}
	if err := repo.CreateUser(admin); err != nil {
		t.Fatalf("create admin: %v", err)
	}

	w := oauthCallbackRequest(t, handler, `{"code":"attacker-code","simulated":false,"username":"neo","email":"admin@example.com"}`)
	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for admin takeover, got %d: %s", w.Code, w.Body.String())
	}
	if strings.Contains(w.Body.String(), `"token"`) {
		t.Fatalf("response must not contain a token: %s", w.Body.String())
	}

	// Taking over the administrator by username is rejected too.
	w = oauthCallbackRequest(t, handler, `{"code":"attacker-code","simulated":false,"username":"admin"}`)
	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for admin username takeover, got %d: %s", w.Code, w.Body.String())
	}
}

// TestSimulatedOAuthStillAllowsFreshDemoUser ensures the hardening did not break
// the legitimate development flow for brand-new demo identities.
func TestSimulatedOAuthStillAllowsFreshDemoUser(t *testing.T) {
	t.Setenv("ENABLE_SIMULATED_OAUTH", "true")
	t.Setenv("GITHUB_CLIENT_ID", "")
	t.Setenv("GITHUB_CLIENT_SECRET", "")

	handler, _ := newSecurityTestHandler(t)
	w := oauthCallbackRequest(t, handler, `{"code":"demo","simulated":true,"username":"demo-user"}`)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 for a fresh demo user, got %d: %s", w.Code, w.Body.String())
	}
	var payload struct {
		Code int `json:"code"`
		Data struct {
			Token string `json:"token"`
			User  struct {
				Username string `json:"username"`
				Role     string `json:"role"`
			} `json:"user"`
		} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if payload.Code != 0 || payload.Data.Token == "" || payload.Data.User.Role != "user" {
		t.Fatalf("unexpected response: %s", w.Body.String())
	}
}

// TestCreateAPIKeyIgnoresClientOwnedFields pins the field-injection fix: a
// regular user could previously post used_cost (negative), user_id, tenant_id
// and even the key material itself and have it written verbatim.
func TestCreateAPIKeyIgnoresClientOwnedFields(t *testing.T) {
	handler, repo := newSecurityTestHandler(t)
	bob := &storage.UserRecord{
		Username:     "bob",
		Email:        "bob@example.com",
		PasswordHash: "x",
		Role:         "user",
		Status:       "active",
		GroupName:    "default",
	}
	if err := repo.CreateUser(bob); err != nil {
		t.Fatalf("create user: %v", err)
	}

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/api/v1/admin/keys", strings.NewReader(
		`{"tenant_id":"admin","user_id":999,"used_cost":-999999,"used_tokens":999999,"key":"sk-attacker-choice-0123456789","status":"disabled","group_name":"default","budget":999999}`,
	))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set("admin_claims", &AdminClaims{Username: "bob", Role: "user"})
	handler.CreateAPIKey(c)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	keys, err := repo.ListAPIKeysByUser(bob.ID)
	if err != nil {
		t.Fatalf("list keys: %v", err)
	}
	if len(keys) != 1 {
		t.Fatalf("expected exactly one key, got %d", len(keys))
	}
	got := keys[0]
	if got.TenantID != "bob" {
		t.Errorf("tenant_id = %q, want %q (server-owned)", got.TenantID, "bob")
	}
	if got.UserID != bob.ID {
		t.Errorf("user_id = %d, want %d", got.UserID, bob.ID)
	}
	if got.UsedCost != 0 || got.UsedTokens != 0 {
		t.Errorf("usage counters must start at zero, got cost=%v tokens=%d", got.UsedCost, got.UsedTokens)
	}
	if got.Status != "active" {
		t.Errorf("status = %q, want active for a non-admin caller", got.Status)
	}
	// Only the digest is stored (the plaintext is returned once, in the creation
	// response), so "server-side generated" now means "the stored value is a
	// digest that is not the client's literal".
	if got.Key == "sk-attacker-choice-0123456789" || !storage.IsAPIKeyHash(got.Key) {
		t.Errorf("key must be generated server-side and stored as a digest, got %q", got.Key)
	}
	if fields := strings.Fields(w.Body.String()); len(fields) == 0 {
		t.Fatalf("creation response is empty")
	}
}
