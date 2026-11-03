package middleware

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/ifnodoraemon/airoute/internal/config"
	"github.com/ifnodoraemon/airoute/internal/storage"
)

const freeReadVideoTaskCredential = "sk-airoute-free-read-video-task-000001"

// videoReadAuthFixture wires a real repository + snapshot so AuthMiddleware
// admits or refuses a key exactly like production.
type videoReadAuthFixture struct {
	engine    *gin.Engine
	repo      *storage.Repository
	keyRecord *storage.APIKeyRecord
	user      *storage.UserRecord
}

func newVideoReadAuthFixture(t *testing.T, userStatus string, userBalance float64, tweakKey func(*storage.APIKeyRecord)) *videoReadAuthFixture {
	t.Helper()
	gin.SetMode(gin.TestMode)

	db, err := storage.OpenDB(filepath.Join(t.TempDir(), "freeread.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	prevRepo, prevCfg := storage.GetGlobalRepository(), config.GetGlobalConfig()
	repo := storage.NewRepository(db)
	storage.SetGlobalRepository(repo)
	t.Cleanup(func() {
		storage.SetGlobalRepository(prevRepo)
		config.SetGlobalConfig(prevCfg)
		_ = db.Close()
	})

	user := &storage.UserRecord{Username: "free-read-video-user", Role: "user", Status: userStatus, Balance: userBalance}
	if err := repo.CreateUser(user); err != nil {
		t.Fatalf("create user: %v", err)
	}
	key := &storage.APIKeyRecord{
		Key: freeReadVideoTaskCredential, TenantID: user.Username, UserID: user.ID,
		Status: "active", GroupName: "default",
	}
	if tweakKey != nil {
		tweakKey(key)
	}
	if err := repo.CreateAPIKey(key); err != nil {
		t.Fatalf("create key: %v", err)
	}

	keys, err := repo.ToModelAPIKeys()
	if err != nil {
		t.Fatalf("build key snapshot: %v", err)
	}
	cfg := &config.Config{}
	cfg.UpdateAPIKeys(keys, true)
	config.SetGlobalConfig(cfg)

	engine := gin.New()
	engine.Use(AuthMiddleware())
	engine.GET("/v1/videos/tasks/:id", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"id": c.Param("id")})
	})
	engine.GET("/videos/tasks/:id", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"id": c.Param("id")})
	})
	engine.POST("/v1/videos/generations", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})
	engine.GET("/v1/models", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})

	return &videoReadAuthFixture{engine: engine, repo: repo, keyRecord: key, user: user}
}

func (f *videoReadAuthFixture) do(t *testing.T, method, target, credential string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, target, nil)
	if credential != "" {
		req.Header.Set("Authorization", "Bearer "+credential)
	}
	w := httptest.NewRecorder()
	f.engine.ServeHTTP(w, req)
	return w
}

func requireAuthErrorCode(t *testing.T, w *httptest.ResponseRecorder, wantStatus int, wantCode string) {
	t.Helper()
	if w.Code != wantStatus {
		t.Fatalf("status = %d, want %d (body=%s)", w.Code, wantStatus, w.Body.String())
	}
	if wantCode == "" {
		return
	}
	var payload struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode error body: %v (body=%s)", err, w.Body.String())
	}
	if payload.Error.Code != wantCode {
		t.Fatalf("error code = %q, want %q (body=%s)", payload.Error.Code, wantCode, w.Body.String())
	}
}

// A video-task GET is a read of a job that was already billed on creation.
// An exhausted wallet used to deny it with 402, so a customer could not poll
// the result they had paid for.
func TestVideoTaskReadAllowedWithExhaustedWallet(t *testing.T) {
	f := newVideoReadAuthFixture(t, "active", 0, nil)

	for _, target := range []string{"/v1/videos/tasks/vid_123", "/videos/tasks/vid_123"} {
		w := f.do(t, http.MethodGet, target, freeReadVideoTaskCredential)
		requireAuthErrorCode(t, w, http.StatusOK, "")
	}

	// The exemption is limited to GET task reads: every other data-plane call
	// from the same key is still blocked by the empty wallet.
	requireAuthErrorCode(t, f.do(t, http.MethodGet, "/v1/models", freeReadVideoTaskCredential), http.StatusPaymentRequired, "insufficient_quota")
	requireAuthErrorCode(t, f.do(t, http.MethodPost, "/v1/videos/generations", freeReadVideoTaskCredential), http.StatusPaymentRequired, "insufficient_quota")
}

// The per-key budget is another spending gate that must not block a free read.
func TestVideoTaskReadAllowedWithExhaustedKeyBudget(t *testing.T) {
	f := newVideoReadAuthFixture(t, "active", 100, func(key *storage.APIKeyRecord) {
		key.Budget = 10
		key.UsedCost = 10
	})

	requireAuthErrorCode(t, f.do(t, http.MethodGet, "/v1/videos/tasks/vid_budget", freeReadVideoTaskCredential), http.StatusOK, "")
	requireAuthErrorCode(t, f.do(t, http.MethodGet, "/v1/models", freeReadVideoTaskCredential), http.StatusPaymentRequired, "key_budget_exceeded")
}

// The exemption waives spending gates only: authentication, key status and
// locked/deleted accounts still reject the request.
func TestVideoTaskReadStillBlockedByAuthAndAccountState(t *testing.T) {
	t.Run("missing credential", func(t *testing.T) {
		f := newVideoReadAuthFixture(t, "active", 0, nil)
		requireAuthErrorCode(t, f.do(t, http.MethodGet, "/v1/videos/tasks/vid_1", ""), http.StatusUnauthorized, "missing_api_key")
	})

	t.Run("disabled key", func(t *testing.T) {
		f := newVideoReadAuthFixture(t, "active", 0, nil)
		// Disable the key after the snapshot was built: the authoritative storage
		// overlay must still revoke it on the free-read path.
		f.keyRecord.Status = "disabled"
		if err := f.repo.UpdateAPIKey(f.keyRecord); err != nil {
			t.Fatalf("disable key: %v", err)
		}
		requireAuthErrorCode(t, f.do(t, http.MethodGet, "/v1/videos/tasks/vid_1", freeReadVideoTaskCredential), http.StatusUnauthorized, "api_key_revoked")
	})

	t.Run("locked account", func(t *testing.T) {
		f := newVideoReadAuthFixture(t, "locked", 0, nil)
		requireAuthErrorCode(t, f.do(t, http.MethodGet, "/v1/videos/tasks/vid_1", freeReadVideoTaskCredential), http.StatusForbidden, "account_locked")
	})
}
