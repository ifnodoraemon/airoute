package controlplane

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

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
