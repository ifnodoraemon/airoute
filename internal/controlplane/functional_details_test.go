package controlplane

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/ifnodoraemon/airoute/internal/billing"
	"github.com/ifnodoraemon/airoute/internal/model"
	"github.com/ifnodoraemon/airoute/internal/provider"
	"github.com/ifnodoraemon/airoute/internal/storage"
	"golang.org/x/crypto/bcrypt"
)

// Regression coverage for the functional details review.
func TestAuditReviewGoogleDisplayNameDoesNotIdentifyAccount(t *testing.T) {
	gin.SetMode(gin.TestMode)
	t.Setenv("GOOGLE_CLIENT_ID", "audit-client")
	t.Setenv("GOOGLE_CLIENT_SECRET", "audit-secret")
	h, repo := newSecurityTestHandler(t)
	victim := &storage.UserRecord{Username: "gg_Common Name", Email: "victim@example.invalid", PasswordHash: "unused", Role: "admin", Status: "active"}
	if err := repo.CreateUser(victim); err != nil {
		t.Fatal(err)
	}
	previousTransport := http.DefaultTransport
	t.Cleanup(func() { http.DefaultTransport = previousTransport })
	http.DefaultTransport = regressionTransport(func(r *http.Request) (*http.Response, error) {
		body := `{"access_token":"synthetic-provider-token"}`
		if r.URL.Path == "/oauth2/v2/userinfo" {
			body = `{"id":"different-google-subject","name":"Common Name","email":"different-person@example.invalid","verified_email":true}`
		}
		return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body))}, nil
	})
	state, err := newOAuthState("google")
	if err != nil {
		t.Fatal(err)
	}
	body, _ := json.Marshal(map[string]string{"code": "valid-synthetic-code", "state": state})
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "provider", Value: "google"}}
	c.Request = httptest.NewRequest("POST", "/api/v1/auth/oauth/google/callback", strings.NewReader(string(body)))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Request.AddCookie(&http.Cookie{Name: OAuthStateCookieName, Value: state})
	h.OAuthCallback(c)
	var result struct {
		Data struct {
			User struct {
				Username string `json:"username"`
				Role     string `json:"role"`
			} `json:"user"`
		} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if w.Code == 200 && result.Data.User.Username == victim.Username {
		t.Fatalf("a distinct provider subject/email was authenticated as existing account %q with role %q", result.Data.User.Username, result.Data.User.Role)
	}
}

func TestAuditReviewPasswordChangeRevokesOldSession(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h, repo := newSecurityTestHandler(t)
	hash, _ := bcrypt.GenerateFromPassword([]byte("old-password"), bcrypt.MinCost)
	if err := repo.CreateUser(&storage.UserRecord{Username: "audit-user", Email: "audit@example.invalid", PasswordHash: string(hash), Role: "user", Status: "active"}); err != nil {
		t.Fatal(err)
	}
	user, _ := repo.GetUserByUsername("audit-user")
	token, _ := GenerateAdminToken(user, time.Hour)
	router := gin.New()
	router.Use(h.AdminAuthMiddleware())
	router.POST("/password", h.ChangePassword)
	router.GET("/me", h.GetMe)
	w := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/password", strings.NewReader(`{"old_password":"old-password","new_password":"new-password"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	router.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatalf("password fixture failed: %d %s", w.Code, w.Body.String())
	}
	w = httptest.NewRecorder()
	req = httptest.NewRequest("GET", "/me", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	router.ServeHTTP(w, req)
	if w.Code == 200 {
		t.Fatal("session issued before password change still authorizes /me with HTTP 200")
	}
}

func TestAuditReviewPartialPriceUpdatePreservesCurrency(t *testing.T) {
	store := newFakeRouteStore()
	store.seedPrice(&storage.ModelPriceRecord{Model: "audit-model", GroupName: "default", Currency: "USD", PromptPrice: 2, CompletionPrice: 8})
	service := NewRouteService(store, nil, nil)
	if err := service.UpdateModelRoute(context.Background(), &UpdateModelRouteRequest{Model: "audit-model", PromptPrice: priceFloat(3)}); err == nil {
		t.Fatal("unsupported existing currency silently accepted")
	}
	if store.applyCalls != 0 {
		t.Fatal("foreign price was rewritten")
	}
	if err := service.UpdateModelRoute(context.Background(), &UpdateModelRouteRequest{Model: "audit-model", Currency: "CNY", PromptPrice: priceFloat(3)}); err != nil {
		t.Fatal(err)
	}
	if store.lastBundle.Price.Currency != "CNY" || store.lastBundle.Price.CompletionPrice != 8 {
		t.Fatal("explicit correction lost other fields")
	}

}

func TestAuditReviewPriceReadFailureDoesNotOverwriteRates(t *testing.T) {
	store := newFakeRouteStore()
	store.getPriceErr = errors.New("temporary database read failure")
	service := NewRouteService(store, nil, nil)
	err := service.UpdateModelRoute(context.Background(), &UpdateModelRouteRequest{Model: "audit-model", FixedPrice: priceFloat(1)})
	if err == nil || store.applyCalls != 0 {
		t.Fatalf("read error ignored: applyCalls=%d; written prompt=%v completion=%v", store.applyCalls, store.lastBundle.Price.PromptPrice, store.lastBundle.Price.CompletionPrice)
	}
}

func TestAuditReviewChannelTestRespectsTextProtocol(t *testing.T) {
	h, repo := newSecurityTestHandler(t)
	ch := &storage.ChannelRecord{Name: "text-only", Type: model.ProviderOpenAI, BaseURL: "https://audit.invalid/v1", Models: []string{"text-model"}, Protocols: []string{"openai_text"}, Status: "active"}
	if err := repo.CreateChannel(ch); err != nil {
		t.Fatal(err)
	}
	previousClient := provider.SharedDefaultHTTPClient
	t.Cleanup(func() { provider.SharedDefaultHTTPClient = previousClient })
	var calledPath string
	provider.SharedDefaultHTTPClient = &http.Client{Transport: regressionTransport(func(r *http.Request) (*http.Response, error) {
		calledPath = r.URL.Path
		body, status := `{"choices":[{"text":"pong","finish_reason":"stop"}]}`, 200
		if calledPath != "/v1/completions" {
			body, status = `{"error":{"message":"endpoint not supported"}}`, 404
		}
		return &http.Response{StatusCode: status, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body))}, nil
	})}
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "id", Value: strconv.FormatInt(ch.ID, 10)}}
	c.Request = httptest.NewRequest("POST", "/channels/test", nil)
	h.TestChannel(c)
	if calledPath != "/v1/completions" {
		t.Fatalf("text-only configured channel tested with %s; response=%s", calledPath, w.Body.String())
	}
}

func TestAuditReviewForeignCurrencyDoesNotDebitCNYAtFaceValue(t *testing.T) {
	_, repo := newSecurityTestHandler(t)
	u := &storage.UserRecord{Username: "currency-user", Email: "currency@example.invalid", PasswordHash: "unused", Role: "user", Status: "active", Balance: 10}
	if err := repo.CreateUser(u); err != nil {
		t.Fatal(err)
	}
	key := &storage.APIKeyRecord{Key: "sk-currency-audit", TenantID: u.Username, UserID: u.ID, Status: "active"}
	if err := repo.CreateAPIKey(key); err != nil {
		t.Fatal(err)
	}
	svc := NewPricingService(repo, nil, nil, nil)
	err := svc.SaveRate(context.Background(), &storage.ModelPriceRecord{Model: "currency-model", GroupName: "default", Currency: "USD", PromptPrice: 1, OffPeakEnabled: false})
	if err != nil {
		return
	} // A single-currency wallet may correctly reject USD.
	engine := billing.NewEngine(repo)
	cost, _, _, _, _, snapshot := engine.CalculateCostForUnits("currency-model", "default", 1_000_000, 0, 0, 1, time.Now())
	rec := &storage.UsageLogRecord{APIKey: key.Key, TenantID: u.Username, Model: "currency-model", PromptTokens: 1_000_000, TotalTokens: 1_000_000, Cost: cost, PricingSnapshot: snapshot, StatusCode: 200}
	if err := repo.BatchRecordUsageLogsContext(context.Background(), []*storage.UsageLogRecord{rec}); err != nil {
		t.Fatal(err)
	}
	after, err := repo.GetUserByUsername(u.Username)
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.Currency == "USD" && after.Balance == 9 {
		t.Fatalf("USD 1 pricing was accepted and charged as CNY 1 (CNY wallet 10 -> %v)", after.Balance)
	}
}

func TestOAuthStableSubjectAndCrossReplicaCookie(t *testing.T) {
	t.Setenv("GOOGLE_CLIENT_ID", "synthetic-client")
	t.Setenv("GOOGLE_CLIENT_SECRET", "synthetic-secret")
	h, repo := newSecurityTestHandler(t)
	originalTransport := http.DefaultTransport
	t.Cleanup(func() { http.DefaultTransport = originalTransport })
	subject, displayName, email := "subject-1", "Same Name", "first@example.invalid"
	http.DefaultTransport = regressionTransport(func(r *http.Request) (*http.Response, error) {
		body := []byte(`{"access_token":"synthetic"}`)
		if r.URL.Path == "/oauth2/v2/userinfo" {
			body, _ = json.Marshal(map[string]any{"id": subject, "name": displayName, "email": email, "verified_email": true})
		}
		return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(string(body)))}, nil
	})
	callback := func(method string) *httptest.ResponseRecorder {
		state, err := newOAuthState("google")
		if err != nil {
			t.Fatal(err)
		}
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Params = gin.Params{{Key: "provider", Value: "google"}}
		c.Request = httptest.NewRequest(method, "/api/v1/auth/oauth/google/callback?code=synthetic&state="+state, nil)
		c.Request.AddCookie(&http.Cookie{Name: OAuthStateCookieName, Value: state})
		h.OAuthCallback(c)
		return w
	}
	first := callback("POST")
	if first.Code != 200 {
		t.Fatalf("initial login: %d %s", first.Code, first.Body.String())
	}
	user, err := repo.GetUserByOAuthIdentity("google", subject)
	if err != nil {
		t.Fatal(err)
	}
	displayName, email = "Renamed Person", "changed@example.invalid"
	second := callback("POST")
	if second.Code != 200 {
		t.Fatal(second.Body.String())
	}
	same, err := repo.GetUserByOAuthIdentity("google", subject)
	if err != nil || same.ID != user.ID {
		t.Fatalf("subject identity changed: %+v %v", same, err)
	}
	subject = "subject-2"
	email = "second@example.invalid"
	other := callback("POST")
	if other.Code != 200 {
		t.Fatal(other.Body.String())
	}
	otherUser, err := repo.GetUserByOAuthIdentity("google", subject)
	if err != nil || otherUser.ID == user.ID {
		t.Fatal("same display name merged subjects")
	}
	// An existing local account email must never be implicitly linked.
	local := &storage.UserRecord{Username: "local-admin", Email: "local@example.invalid", PasswordHash: "test", Role: "admin"}
	if err := repo.CreateUser(local); err != nil {
		t.Fatal(err)
	}
	subject, email = "unlinked-subject", local.Email
	if w := callback("POST"); w.Code != http.StatusConflict {
		t.Fatalf("local account email linked: %d %s", w.Code, w.Body.String())
	}
	subject, email = "subject-1", "changed@example.invalid"
	redirect := callback("GET")
	if redirect.Code != 302 || !strings.Contains(redirect.Header().Get("Location"), "oauth_complete=1") || strings.Contains(redirect.Header().Get("Location"), "oauth_code") {
		t.Fatalf("unexpected redirect: %d %s", redirect.Code, redirect.Header().Get("Location"))
	}
	// A second handler uses the shared repository and verifies the HttpOnly cookie.
	otherNode := NewAdminHandler(storage.NewRepository(repo.GetDB()), nil, nil)
	server := gin.New()
	server.GET("/me", otherNode.AdminAuthMiddleware(), otherNode.GetMe)
	request := httptest.NewRequest("GET", "/me", nil)
	for _, cookie := range redirect.Result().Cookies() {
		request.AddCookie(cookie)
	}
	w := httptest.NewRecorder()
	server.ServeHTTP(w, request)
	if w.Code != 200 || !strings.Contains(w.Body.String(), user.Username) {
		t.Fatalf("cross-replica restore failed: %d %s", w.Code, w.Body.String())
	}
}

func TestPasswordResetRevokesSessionsAcrossHandlers(t *testing.T) {
	h, repo := newSecurityTestHandler(t)
	user := &storage.UserRecord{Username: "reset-user", PasswordHash: "original-hash", Role: "user", Status: "active"}
	if err := repo.CreateUser(user); err != nil {
		t.Fatal(err)
	}
	old, err := GenerateAdminToken(user, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if err := repo.UpdateUserPassword(user.Username, "new-hash"); err != nil {
		t.Fatal(err)
	}
	otherNode := NewAdminHandler(storage.NewRepository(repo.GetDB()), nil, nil)
	for _, node := range []*AdminHandler{h, otherNode} {
		server := gin.New()
		server.GET("/me", node.AdminAuthMiddleware(), node.GetMe)
		req := httptest.NewRequest("GET", "/me", nil)
		req.Header.Set("Authorization", "Bearer "+old)
		w := httptest.NewRecorder()
		server.ServeHTTP(w, req)
		if w.Code != 401 {
			t.Fatalf("reset token still valid: %d", w.Code)
		}
		current, _ := repo.GetUserByUsername(user.Username)
		fresh, _ := GenerateAdminToken(current, time.Hour)
		req = httptest.NewRequest("GET", "/me", nil)
		req.Header.Set("Authorization", "Bearer "+fresh)
		w = httptest.NewRecorder()
		server.ServeHTTP(w, req)
		if w.Code != 200 {
			t.Fatalf("fresh token rejected: %d %s", w.Code, w.Body.String())
		}
	}
}
