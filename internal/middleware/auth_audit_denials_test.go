package middleware

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/ifnodoraemon/airoute/internal/config"
	"github.com/ifnodoraemon/airoute/internal/model"
	"github.com/ifnodoraemon/airoute/internal/storage"
)

// openDenialAuditStore wires a real async logger against a throwaway SQLite file so
// a test can read the rows the middleware actually queued. The returned flush stops
// the worker deterministically (Stop is once-only) before the table is queried.
func openDenialAuditStore(t *testing.T) (*storage.DB, func()) {
	t.Helper()
	db, err := storage.OpenDB(filepath.Join(t.TempDir(), "denials.db"))
	if err != nil {
		t.Fatalf("open test db: %v", err)
	}
	previousLogger, previousRepo := storage.GlobalAsyncLogger, storage.GetGlobalRepository()
	logger := storage.InitAsyncLogger(storage.NewRepository(db), 8, 8, time.Hour)
	t.Cleanup(func() {
		logger.Stop()
		storage.GlobalAsyncLogger = previousLogger
		storage.SetGlobalRepository(previousRepo)
		db.Close()
	})
	return db, logger.Stop
}

func countUsageRows(t *testing.T, db *storage.DB, query string, args ...any) int {
	t.Helper()
	var n int
	if err := db.QueryRow(query, args...).Scan(&n); err != nil {
		t.Fatalf("count rows: %v", err)
	}
	return n
}

// A wallet-exhausted 402 is a rejection on an authenticated key, and the console's
// request log is built from usage_logs: without the row the operator cannot see why
// a customer was refused. It used to be the one denial class AuthMiddleware never
// audited, even though auditAuthFailure's contract promises 402/403/429 stay visible.
func TestAuthMiddlewareAuditsWalletRejection(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, flush := openDenialAuditStore(t)

	previousConfig := config.GetGlobalConfig()
	defer config.SetGlobalConfig(previousConfig)

	key := model.APIKeyConfig{
		Key:         storage.HashAPIKey("sk-broke-tenant"),
		TenantID:    "t",
		Status:      "active",
		UserID:      7,
		UserStatus:  "active",
		UserRole:    "user",
		UserBalance: 0,
	}
	cfg := &config.Config{}
	cfg.UpdateAPIKeys([]model.APIKeyConfig{key}, true)
	config.SetGlobalConfig(cfg)

	engine := gin.New()
	engine.Use(TraceMiddleware(), AuthMiddleware())
	engine.GET("/v1/models", func(c *gin.Context) { c.Status(http.StatusOK) })

	req := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	req.Header.Set("Authorization", "Bearer sk-broke-tenant")
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	if w.Code != http.StatusPaymentRequired {
		t.Fatalf("status = %d, want 402 (wallet exhausted)", w.Code)
	}

	// Exactly one row: a second write for the same request would double-count the
	// rejection in the console's error statistics.
	flush()
	if rows := countUsageRows(t, db, `SELECT COUNT(*) FROM usage_logs`); rows != 1 {
		t.Fatalf("usage_logs rows = %d, want exactly 1", rows)
	}
	if rows := countUsageRows(t, db, `SELECT COUNT(*) FROM usage_logs WHERE status_code = ? AND usage_source = 'gateway_auth'`, http.StatusPaymentRequired); rows != 1 {
		t.Fatalf("402 audit rows = %d, want 1", rows)
	}
	if rows := countUsageRows(t, db, `SELECT COUNT(*) FROM usage_logs WHERE api_key = ?`, key.Key); rows != 1 {
		t.Fatalf("402 audit row does not carry the key digest (rows matching digest = %d)", rows)
	}
}

// Rate limiting runs behind AuthMiddleware, so its 429 was never audited at all.
// The digest on the context is written as-is: hashing it again would store a value
// that joins to no api_keys row.
func TestRateLimitMiddlewareAuditsThrottle(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, flush := openDenialAuditStore(t)

	// A real stored digest: the context carries HashAPIKey(credential), and the
	// writer preserves values that already look like one instead of hashing again.
	digest := storage.HashAPIKey("sk-throttled-tenant")
	key := &model.APIKeyConfig{Key: digest, Status: "active", RPM: 1}
	// Consume the single token so the middleware's gate fires immediately.
	if !GlobalRateLimiter.GetBucket(digest, 1).Allow() {
		t.Fatal("test setup: the first token should have been available")
	}

	engine := gin.New()
	engine.Use(TraceMiddleware(), func(c *gin.Context) {
		c.Set(ContextKeyAPIKey, digest)
		c.Set(ContextKeyAPIKeyConfig, key)
		c.Next()
	}, RateLimitMiddleware())
	engine.GET("/v1/models", func(c *gin.Context) { c.Status(http.StatusOK) })

	req := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	if w.Code != http.StatusTooManyRequests {
		t.Fatalf("status = %d, want 429", w.Code)
	}

	flush()
	if rows := countUsageRows(t, db, `SELECT COUNT(*) FROM usage_logs WHERE status_code = ?`, http.StatusTooManyRequests); rows != 1 {
		t.Fatalf("429 audit rows = %d, want exactly 1", rows)
	}
	if rows := countUsageRows(t, db, `SELECT COUNT(*) FROM usage_logs WHERE api_key = ?`, digest); rows != 1 {
		t.Fatalf("429 audit row does not carry the resolved digest (rows matching digest = %d)", rows)
	}
}
