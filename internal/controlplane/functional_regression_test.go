package controlplane

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/ifnodoraemon/airoute/internal/config"
	"github.com/ifnodoraemon/airoute/internal/middleware"
	"github.com/ifnodoraemon/airoute/internal/storage"
)

type regressionTransport func(*http.Request) (*http.Response, error)

func (f regressionTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func regressionCall(fn gin.HandlerFunc, method, path, body string) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(method, path, strings.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set("admin_claims", &AdminClaims{Username: "regression-client", Role: "user"})
	fn(c)
	return w
}
func regressionHandler(t *testing.T) (*AdminHandler, *storage.Repository) {
	t.Helper()
	db, err := storage.OpenDB(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	repo := storage.NewRepository(db)
	return &AdminHandler{repo: repo}, repo
}

// regressionCredential is the caller-side credential for regressionUsage; the
// repository stores only its digest.
const regressionCredential = "sk-regression-synthetic-only"

func regressionUsage(t *testing.T, repo *storage.Repository) *storage.APIKeyRecord {
	t.Helper()
	u := &storage.UserRecord{Username: "regression-client", Email: "regression@example.invalid", PasswordHash: "synthetic", Role: "user", Status: "active", Balance: 100, GroupName: "default"}
	if err := repo.CreateUser(u); err != nil {
		t.Fatal(err)
	}
	k := &storage.APIKeyRecord{Key: regressionCredential, TenantID: u.Username, UserID: u.ID, AllowedModels: []string{"*"}, Status: "active"}
	if err := repo.CreateAPIKey(k); err != nil {
		t.Fatal(err)
	}
	if err := repo.RecordUsageLogContext(context.Background(), &storage.UsageLogRecord{APIKey: k.Key, TenantID: u.Username, Model: "regression-chat", Channel: "regression-provider", Cost: 1, TotalTokens: 10, StatusCode: 200}); err != nil {
		t.Fatal(err)
	}
	return k
}
func TestCustomerKeyIDDrilldown(t *testing.T) {
	h, r := regressionHandler(t)
	k := regressionUsage(t, r)
	exact := regressionCall(h.ListLogs, "GET", "/api/v1/admin/logs?key="+url.QueryEscape(k.Key), "")
	if decodeEnvelope(t, exact)["total"] != float64(1) {
		t.Fatalf("fixture: %s", exact.Body.String())
	}
	keyword := regressionCall(h.ListLogs, "GET", fmt.Sprintf("/api/v1/admin/logs?key_id=%d", k.ID), "")
	if decodeEnvelope(t, keyword)["total"] != float64(1) {
		t.Fatalf("key_id filter did not return the owned record: %s", keyword.Body.String())
	}
}
func TestTraceAndSessionAreDistinctFilters(t *testing.T) {
	h, r := regressionHandler(t)
	regressionUsage(t, r)
	logs, _, err := r.ListUsageLogsWithFilterAndCount(storage.LogFilter{Limit: 10, Offset: 0})
	if err != nil || len(logs) != 1 {
		t.Fatal("fixture")
	}
	trace := logs[0].TraceID
	exact := regressionCall(h.ListLogs, "GET", "/api/v1/admin/logs?trace_id="+url.QueryEscape(trace), "")
	if decodeEnvelope(t, exact)["total"] != float64(1) {
		t.Fatal("fixture")
	}
	wrong := regressionCall(h.ListLogs, "GET", "/api/v1/admin/logs?session_id="+url.QueryEscape(trace), "")
	if decodeEnvelope(t, wrong)["total"] != float64(0) {
		t.Fatalf("trace sent as session_id returns total=%v", decodeEnvelope(t, wrong)["total"])
	}
}
func TestDeletedKeyPreservesCustomerHistory(t *testing.T) {
	h, r := regressionHandler(t)
	k := regressionUsage(t, r)
	before := regressionCall(h.ListLogs, "GET", "/api/v1/admin/logs", "")
	if decodeEnvelope(t, before)["total"] != float64(1) {
		t.Fatal("fixture")
	}
	if err := r.DeleteAPIKey(k.ID); err != nil {
		t.Fatal(err)
	}
	after := regressionCall(h.ListLogs, "GET", "/api/v1/admin/logs", "")
	if decodeEnvelope(t, after)["total"] != float64(1) {
		t.Fatalf("customer historical total after key deletion: %v", decodeEnvelope(t, after)["total"])
	}
	if _, err := r.GetAPIKeyByKey(k.Key); err == nil {
		t.Fatal("deleted credential is still available for authentication")
	}
	if keys, err := r.ListAPIKeysByUser(k.UserID); err != nil || len(keys) != 0 {
		t.Fatalf("deleted key remains in management list: %v %v", keys, err)
	}
	if _, err := r.BatchUpdateAPIKeyStatus([]int64{k.ID}, "active"); err != nil {
		t.Fatal(err)
	}
	if _, err := r.GetAPIKeyByKey(k.Key); err == nil {
		t.Fatal("batch update resurrected a deleted credential")
	}
	stats := regressionCall(h.GetStatsOverview, "GET", "/stats", "")
	if stats.Code != 200 || decodeEnvelope(t, stats)["data"].(map[string]any)["total_requests"] != float64(1) {
		t.Fatalf("historical overview was lost: %s", stats.Body.String())
	}
}

func TestDeletingLastKeyKeepsAuthenticationEnforced(t *testing.T) {
	for _, operation := range []string{"single", "batch", "account"} {
		t.Run(operation, func(t *testing.T) {
			_, repo := regressionHandler(t)
			key := regressionUsage(t, repo)
			previousConfig := config.GetGlobalConfig()
			previousRepo := storage.GetGlobalRepository()
			t.Cleanup(func() {
				config.SetGlobalConfig(previousConfig)
				storage.SetGlobalRepository(previousRepo)
			})
			cfg := &config.Config{}
			config.SetGlobalConfig(cfg)
			storage.SetGlobalRepository(repo)
			sync := newTestSynchronizer(repo, nil, cfg, nil)
			if err := sync.ReloadFromDB(); err != nil {
				t.Fatal(err)
			}
			engine := gin.New()
			engine.GET("/protected", middleware.AuthMiddleware(), func(c *gin.Context) { c.Status(http.StatusOK) })
			request := func(credential string, want int) {
				t.Helper()
				r := httptest.NewRequest("GET", "/protected", nil)
				if credential != "" {
					r.Header.Set("Authorization", "Bearer "+credential)
				}
				w := httptest.NewRecorder()
				engine.ServeHTTP(w, r)
				if w.Code != want {
					t.Fatalf("auth status = %d, want %d: %s", w.Code, want, w.Body.String())
				}
			}
			// Clients hold the plaintext credential; api_keys.key is its digest.
			request(regressionCredential, http.StatusOK)
			var err error
			if operation == "batch" {
				_, err = repo.BatchDeleteAPIKeys([]int64{key.ID})
			} else if operation == "account" {
				err = repo.DeleteUser("regression-client")
			} else {
				err = repo.DeleteAPIKey(key.ID)
			}
			if err != nil {
				t.Fatal(err)
			}
			// Revocation must also take effect while the runtime snapshot is stale.
			request(key.Key, http.StatusUnauthorized)
			if err := sync.ReloadFromDB(); err != nil {
				t.Fatal(err)
			}
			if !cfg.HasRegisteredAPIKeys() || cfg.ActiveAPIKeyCount() != 0 {
				t.Fatal("last-key deletion did not preserve the authentication gate")
			}
			request("", http.StatusUnauthorized)
			request(key.Key, http.StatusUnauthorized)
		})
	}
}

func regressionStripe(t *testing.T, f regressionTransport) {
	t.Helper()
	t.Setenv("STRIPE_API_KEY", "synthetic-regression-key")
	t.Setenv("STRIPE_WEBHOOK_SECRET", "synthetic-webhook-secret")
	old := http.DefaultClient
	http.DefaultClient = &http.Client{Transport: f}
	t.Cleanup(func() { http.DefaultClient = old })
}
func TestStripeFailureDoesNotCreateCheckout(t *testing.T) {
	h, _ := regressionHandler(t)
	regressionStripe(t, func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 401, Body: io.NopCloser(strings.NewReader(`{"error":{"message":"synthetic failure"}}`)), Header: make(http.Header)}, nil
	})
	w := regressionCall(h.CreateStripeRechargeSession, "POST", "/stripe", `{"amount":50,"currency":"CNY"}`)
	if w.Code != 502 {
		t.Fatalf("want 502, got %d %s", w.Code, w.Body.String())
	}
}
func TestCheckoutRequiresPersistedOrder(t *testing.T) {
	h, r := regressionHandler(t)
	regressionStripe(t, func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"id":"cs_regression","url":"https://example.invalid/regression"}`)), Header: make(http.Header)}, nil
	})
	_ = r.GetDB().Close()
	w := regressionCall(h.CreateStripeRechargeSession, "POST", "/stripe", `{"amount":50,"currency":"CNY"}`)
	if w.Code != 500 {
		t.Fatalf("want 500, got %d %s", w.Code, w.Body.String())
	}
}
func TestRechargeCentsMatchOrderAmount(t *testing.T) {
	h, _ := regressionHandler(t)
	cents := ""
	regressionStripe(t, func(r *http.Request) (*http.Response, error) {
		_ = r.ParseForm()
		cents = r.Form.Get("line_items[0][price_data][unit_amount]")
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"id":"cs_regression","url":"https://example.invalid/regression"}`)), Header: make(http.Header)}, nil
	})
	regressionCall(h.CreateStripeRechargeSession, "POST", "/stripe", `{"amount":19.99,"currency":"CNY"}`)
	if cents != "1999" {
		t.Fatalf("request 19.99 CNY should charge 1999 cents, observed %s", cents)
	}
}

func TestRechargeRejectsInvalidCurrencyAndPrecision(t *testing.T) {
	h, _ := regressionHandler(t)
	for _, body := range []string{`{"amount":0.001}`, `{"amount":10.999}`, `{"amount":1000000}`, `{"amount":50,"currency":"USD"}`} {
		w := regressionCall(h.CreateStripeRechargeSession, "POST", "/stripe", body)
		if w.Code != 400 {
			t.Fatalf("invalid amount accepted: %s -> %d", body, w.Code)
		}
	}
}

func TestCustomerCannotReadAnotherKeyHistory(t *testing.T) {
	h, r := regressionHandler(t)
	regressionUsage(t, r)
	other := &storage.UserRecord{Username: "another-user", PasswordHash: "synthetic", Role: "user"}
	if err := r.CreateUser(other); err != nil {
		t.Fatal(err)
	}
	key := &storage.APIKeyRecord{Key: "sk-other", UserID: other.ID}
	if err := r.CreateAPIKey(key); err != nil {
		t.Fatal(err)
	}
	if err := r.RecordUsageLogContext(context.Background(), &storage.UsageLogRecord{APIKey: key.Key, Model: "private", StatusCode: 200}); err != nil {
		t.Fatal(err)
	}
	w := regressionCall(h.ListLogs, "GET", fmt.Sprintf("/logs?key_id=%d", key.ID), "")
	if decodeEnvelope(t, w)["total"] != float64(0) {
		t.Fatalf("other customer's logs exposed: %s", w.Body.String())
	}
}

func signedRechargeEvent(t *testing.T, h *AdminHandler, kind, status, session, currency string, cents int64) *httptest.ResponseRecorder {
	t.Helper()
	const secret = "synthetic-webhook-test"
	t.Setenv("STRIPE_WEBHOOK_SECRET", secret)
	body, _ := json.Marshal(map[string]any{"type": kind, "data": map[string]any{"object": map[string]any{
		"id": session, "client_reference_id": "REC-regression", "payment_status": status, "currency": currency, "amount_total": cents,
	}}})
	timestamp := fmt.Sprint(time.Now().Unix())
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(timestamp + "." + string(body)))
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("POST", "/stripe/webhook", strings.NewReader(string(body)))
	c.Request.Header.Set("Stripe-Signature", fmt.Sprintf("t=%s,v1=%x", timestamp, mac.Sum(nil)))
	h.StripeWebhook(c)
	return w
}

func TestStripeOrderLifecycleAndIdempotency(t *testing.T) {
	h, r := regressionHandler(t)
	regressionUsage(t, r) // balance is 99 after the fixture request
	if err := r.CreateRechargeOrder(&storage.RechargeOrderRecord{OrderNo: "REC-regression", Username: "regression-client", Amount: 19.99, Currency: "CNY", Channel: "stripe", StripeSessionID: "cs_regression"}); err != nil {
		t.Fatal(err)
	}
	for _, input := range []struct {
		session, currency string
		cents             int64
	}{{"cs_wrong", "cny", 1999}, {"cs_regression", "usd", 1999}, {"cs_regression", "cny", 1998}} {
		if w := signedRechargeEvent(t, h, "checkout.session.completed", "paid", input.session, input.currency, input.cents); w.Code != 400 {
			t.Fatalf("mismatch accepted: %s", w.Body.String())
		}
	}
	for _, status := range []string{"", "unpaid"} {
		if w := signedRechargeEvent(t, h, "checkout.session.completed", status, "cs_regression", "cny", 1999); w.Code != 200 {
			t.Fatal(w.Body.String())
		}
	}
	order, _ := r.GetRechargeOrder("REC-regression")
	if order.Status != "pending" {
		t.Fatal("unpaid checkout credited")
	}
	for _, kind := range []string{"checkout.session.async_payment_failed", "checkout.session.async_payment_succeeded", "checkout.session.completed", "checkout.session.expired"} {
		status := "unpaid"
		if kind == "checkout.session.async_payment_succeeded" || kind == "checkout.session.completed" {
			status = "paid"
		}
		if w := signedRechargeEvent(t, h, kind, status, "cs_regression", "cny", 1999); w.Code != 200 {
			t.Fatal(w.Body.String())
		}
	}
	order, _ = r.GetRechargeOrder("REC-regression")
	user, _ := r.GetUserByUsername("regression-client")
	if order.Status != "paid" || storage.ToUnits(user.Balance) != storage.ToUnits(118.99) {
		t.Fatalf("duplicate or out-of-order event changed balance: %s %.9f", order.Status, user.Balance)
	}
}

func TestRechargeOrderPollingIsOwnerScoped(t *testing.T) {
	h, r := regressionHandler(t)
	if err := r.CreateRechargeOrder(&storage.RechargeOrderRecord{OrderNo: "REC-private", Username: "another-user", Amount: 10}); err != nil {
		t.Fatal(err)
	}
	w := regressionCall(func(c *gin.Context) {
		c.Params = gin.Params{{Key: "order_no", Value: "REC-private"}}
		h.GetUserRechargeOrder(c)
	}, "GET", "/orders/REC-private", "")
	if w.Code != 404 {
		t.Fatalf("another user's order exposed: %s", w.Body.String())
	}
}

func TestAdminCanCreateZeroBalanceUser(t *testing.T) {
	h, r := regressionHandler(t)
	w := regressionCall(func(c *gin.Context) {
		c.Set("admin_claims", &AdminClaims{Username: "admin", Role: "admin"})
		h.CreateUser(c)
	}, "POST", "/users", `{"username":"zero-balance","password":"synthetic-password","balance":0}`)
	if w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	u, err := r.GetUserByUsername("zero-balance")
	if err != nil || u.Balance != 0 {
		t.Fatalf("zero balance replaced with grant: %+v %v", u, err)
	}
	keys, err := r.ListAPIKeysByUser(u.ID)
	if err != nil || len(keys) != 1 {
		t.Fatalf("initial key missing: %v", err)
	}
}
