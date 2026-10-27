package tests

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/ifnodoraemon/airoute/internal/api"
	"github.com/ifnodoraemon/airoute/internal/config"
	"github.com/ifnodoraemon/airoute/internal/controlplane"
	"github.com/ifnodoraemon/airoute/internal/router"
	"github.com/ifnodoraemon/airoute/internal/storage"
)

// Regression: the data-plane snapshot is per replica and reloads asynchronously
// (Redis broadcast, or the periodic timer when Redis is unavailable). A key minted
// on the other replica therefore exists only in storage for a short window, and a
// request that landed on this replica was rejected with
// "Incorrect or unauthorized API key provided" even though the console had just
// handed the credential to the operator. The same gap made a key revoked since the
// last reload look like an unknown key instead of a revoked one.
func TestDataPlaneResolvesCredentialsMissingFromSnapshot(t *testing.T) {
	gin.SetMode(gin.TestMode)

	savedCfg := config.GetGlobalConfig()
	savedRepo := storage.GetGlobalRepository()
	defer func() {
		config.SetGlobalConfig(savedCfg)
		storage.SetGlobalRepository(savedRepo)
	}()

	db, err := storage.OpenDB(filepath.Join(t.TempDir(), fmt.Sprintf("auth_snapshot_miss_%d.db", time.Now().UnixMicro())))
	if err != nil {
		t.Fatalf("open test db: %v", err)
	}
	defer db.Close()

	repo := storage.NewRepository(db)
	storage.SetGlobalRepository(repo)
	dispatcher := router.NewDispatcher(nil)
	synchronizer := controlplane.NewSynchronizer(repo, dispatcher)
	adminHandler := controlplane.NewAdminHandler(repo, synchronizer, dispatcher)

	// Install a config owned by this test before reloading: ReloadFromDB writes the
	// snapshot into the *global* config, and writing into the ambient one would leak
	// a registered key into every test that runs after this one (the suite shares a
	// process-wide singleton).
	config.SetGlobalConfig(&config.Config{})

	// One registered key keeps the gateway in authenticated mode. It is the only
	// key the snapshot will contain.
	seed := &storage.APIKeyRecord{Key: "sk-airoute-seeded0000000000000000000000", TenantID: "seed", Status: "active", RPM: 60}
	if err := repo.CreateAPIKey(seed); err != nil {
		t.Fatalf("seed key: %v", err)
	}
	if err := synchronizer.ReloadFromDB(); err != nil {
		t.Fatalf("initial reload: %v", err)
	}

	// The credential the "other replica" just created: stored, not reloaded here.
	// CreateAPIKey replaces rec.Key with the stored digest, so the plaintext the
	// client holds has to be kept separately.
	const freshCredential = "sk-airoute-newsnapshotmiss0000000000000"
	fresh := &storage.APIKeyRecord{Key: freshCredential, TenantID: "tenant-fresh", Status: "active", RPM: 60}
	if err := repo.CreateAPIKey(fresh); err != nil {
		t.Fatalf("create fresh key: %v", err)
	}
	if cfg := config.GetGlobalConfig(); cfg.GetAPIKey(storage.HashAPIKey(freshCredential)) != nil {
		t.Fatal("snapshot already knows the fresh key; the test would not exercise the fallback")
	}

	engine := api.SetupRouter(dispatcher, adminHandler)
	callModels := func(credential string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
		req.Header.Set("Authorization", "Bearer "+credential)
		w := httptest.NewRecorder()
		engine.ServeHTTP(w, req)
		return w
	}

	// 1. A key that only storage knows must authenticate, not 401.
	w := callModels(freshCredential)
	if w.Code != http.StatusOK {
		t.Fatalf("fresh key rejected before it reached the snapshot: status=%d body=%s", w.Code, w.Body.String())
	}

	// 2. Revoking it (still without a reload) must say "revoked", not "incorrect".
	if err := repo.DeleteAPIKey(fresh.ID); err != nil {
		t.Fatalf("delete key: %v", err)
	}
	w = callModels(freshCredential)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("revoked key status = %d, want 401 (body=%s)", w.Code, w.Body.String())
	}
	var payload struct {
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode rejection: %v", err)
	}
	if payload.Error.Code != "api_key_revoked" {
		t.Fatalf("revoked key error code = %q, want api_key_revoked (body=%s)", payload.Error.Code, w.Body.String())
	}
	if payload.Error.Message == "" {
		t.Fatal("revoked key denial carries no message")
	}

	// 3. An unknown credential still gets the generic rejection.
	w = callModels("sk-airoute-neverexisted000000000000000")
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("unknown credential status = %d, want 401", w.Code)
	}
}
