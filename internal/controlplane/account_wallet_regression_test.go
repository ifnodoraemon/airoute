package controlplane

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/ifnodoraemon/airoute/internal/config"
	"github.com/ifnodoraemon/airoute/internal/storage"
	"golang.org/x/crypto/bcrypt"
)

func accountAdminCall(fn gin.HandlerFunc, body string, params ...gin.Param) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("POST", "/test", strings.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Params = params
	c.Set("admin_claims", &AdminClaims{Username: "operator", Role: "admin"})
	fn(c)
	return w
}

func TestAccountMutationsRejectMissingUsersAndInvalidBalances(t *testing.T) {
	h, r := regressionHandler(t)
	regressionUsage(t, r)
	before, err := r.GetUserByUsername("regression-client")
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name string
		fn   gin.HandlerFunc
		body string
	}{
		{"balance", h.UpdateUserBalance, `{"balance":50}`},
		{"delta", h.UpdateUserBalance, `{"delta":50}`},
		{"status", h.UpdateUserStatus, `{"status":"locked"}`},
		{"group", h.UpdateUserGroup, `{"group_name":"vip"}`},
		{"role", h.UpdateUserRole, `{"role":"admin"}`},
		{"password", h.ResetUserPassword, `{"new_password":"new-secret"}`},
		{"delete", h.DeleteUser, `{}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w := accountAdminCall(tc.fn, tc.body, gin.Param{Key: "username", Value: "missing"})
			if w.Code != 404 {
				t.Fatalf("missing account returned %d: %s", w.Code, w.Body.String())
			}
		})
	}
	for _, body := range []string{`{}`, `{"balance":1,"delta":2}`, `{"balance":1e15}`, `{"delta":-1e15}`} {
		w := accountAdminCall(h.UpdateUserBalance, body, gin.Param{Key: "username", Value: "regression-client"})
		if w.Code != 400 {
			t.Fatalf("invalid adjustment %s: %d %s", body, w.Code, w.Body.String())
		}
	}
	u, _ := r.GetUserByUsername("regression-client")
	if u.Balance != before.Balance {
		t.Fatalf("invalid request changed balance: %v", u.Balance)
	}
}

func TestAccountCreationRejectsUnsupportedRoleAndStatus(t *testing.T) {
	h, _ := regressionHandler(t)
	for _, body := range []string{
		`{"username":"invalid-role","password":"synthetic-password","role":"operator"}`,
		`{"username":"invalid-status","password":"synthetic-password","status":"deleted"}`,
	} {
		w := accountAdminCall(h.CreateUser, body)
		if w.Code != 400 {
			t.Fatalf("unsupported account attributes accepted: %d %s", w.Code, w.Body.String())
		}
	}
}

func TestGiftCardGenerationReportsFailuresAndValidatesAmounts(t *testing.T) {
	h, r := regressionHandler(t)
	for _, body := range []string{`{"amount":0.001}`, `{"amount":1e15}`, `{"amount":20,"count":101}`, `{"amount":20,"count":-1}`} {
		if w := accountAdminCall(h.GenerateRedemptions, body); w.Code != 400 {
			t.Fatalf("invalid batch accepted: %s %s", body, w.Body.String())
		}
	}
	w := accountAdminCall(h.GenerateRedemptions, `{"amount":19.99,"count":3}`)
	if w.Code != 200 {
		t.Fatalf("valid batch rejected: %s", w.Body.String())
	}
	if cards, err := r.ListRedemptionCodes(); err != nil || len(cards) != 3 {
		t.Fatalf("batch size: %d %v", len(cards), err)
	}
	if _, err := r.GetDB().Exec(`CREATE TRIGGER fail_redemptions BEFORE INSERT ON redemption_codes BEGIN SELECT RAISE(ABORT, 'injected failure'); END`); err != nil {
		t.Fatal(err)
	}
	w = accountAdminCall(h.GenerateRedemptions, `{"amount":20,"count":3}`)
	if w.Code != 500 {
		t.Fatalf("failed batch reported success: %d %s", w.Code, w.Body.String())
	}
	if cards, err := r.ListRedemptionCodes(); err != nil || len(cards) != 3 {
		t.Fatalf("failed batch altered existing cards: %d %v", len(cards), err)
	}
}

func TestLoginByEmailAndDeletedAccount(t *testing.T) {
	h, r := regressionHandler(t)
	hash, err := bcrypt.GenerateFromPassword([]byte("synthetic-password"), bcrypt.MinCost)
	if err != nil {
		t.Fatal(err)
	}
	u := &storage.UserRecord{Username: "email-login", Email: "email@example.invalid", PasswordHash: string(hash)}
	if err := r.CreateUser(u); err != nil {
		t.Fatal(err)
	}
	for _, login := range []string{u.Username, u.Email} {
		w := accountAdminCall(h.Login, fmt.Sprintf(`{"username":%q,"password":"synthetic-password"}`, login))
		if w.Code != 200 {
			t.Fatalf("login %s failed: %s", login, w.Body.String())
		}
	}
	if err := r.DeleteUser(u.Username); err != nil {
		t.Fatal(err)
	}
	w := accountAdminCall(h.Login, `{"username":"email@example.invalid","password":"synthetic-password"}`)
	if w.Code != 401 {
		t.Fatalf("deleted account login: %d %s", w.Code, w.Body.String())
	}
}

func TestOAuthOnboardingHonorsPolicyAndRollsBack(t *testing.T) {
	previous := config.GetGlobalConfig()
	t.Cleanup(func() { config.SetGlobalConfig(previous) })
	allow := false
	config.SetGlobalConfig(&config.Config{Auth: config.AuthConfig{AllowRegistration: &allow, InitialUserBalance: 12.34}})
	h, r := regressionHandler(t)
	call := func(name, email string) *httptest.ResponseRecorder {
		return accountAdminCall(h.OAuthCallback, fmt.Sprintf(`{"simulated":true,"username":%q,"email":%q}`, name, email), gin.Param{Key: "provider", Value: "github"})
	}
	if w := call("oauth-closed", "closed@example.invalid"); w.Code != 403 {
		t.Fatalf("closed registration bypassed: %s", w.Body.String())
	}
	allow = true
	if _, err := r.GetDB().Exec(`CREATE TRIGGER fail_oauth_key BEFORE INSERT ON api_keys BEGIN SELECT RAISE(ABORT, 'injected failure'); END`); err != nil {
		t.Fatal(err)
	}
	if w := call("oauth-atomic", "atomic@example.invalid"); w.Code != 500 {
		t.Fatalf("partial onboarding reported success: %s", w.Body.String())
	}
	if u, _ := r.GetUserByUsername("oauth-atomic"); u != nil {
		t.Fatal("OAuth left a partial account")
	}
	if _, err := r.GetDB().Exec(`DROP TRIGGER fail_oauth_key`); err != nil {
		t.Fatal(err)
	}
	if w := call("oauth-atomic", "atomic@example.invalid"); w.Code != 200 {
		t.Fatalf("onboarding failed: %s", w.Body.String())
	}
	u, err := r.GetUserByUsername("oauth-atomic")
	if err != nil || u.Balance != 12.34 {
		t.Fatalf("configured initial balance ignored: %+v %v", u, err)
	}
	if keys, err := r.ListAPIKeysByUser(u.ID); err != nil || len(keys) != 1 {
		t.Fatalf("missing initial key: %d %v", len(keys), err)
	}
	if w := call("another-identity", u.Email); w.Code != http.StatusForbidden {
		t.Fatalf("simulated OAuth took over an email owner: %s", w.Body.String())
	}
}
