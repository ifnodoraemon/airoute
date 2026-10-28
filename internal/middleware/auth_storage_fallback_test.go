package middleware

import (
	"encoding/json"
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

// The snapshot is a cache: a key created on another replica, or a key revoked
// since the last reload, is only in durable storage for a short window. These
// tests pin the fallback that used to make both cases answer
// "Incorrect or unauthorized API key provided".

type authHarness struct {
	t      *testing.T
	repo   *storage.Repository
	engine *gin.Engine
}

// newAuthHarness installs a snapshot built from the supplied credentials and a
// repository holding every key the test creates, which is the same layout the
// process uses in production.
func newAuthHarness(t *testing.T, snapshotCredential string) *authHarness {
	t.Helper()
	gin.SetMode(gin.TestMode)

	db, err := storage.OpenDB(filepath.Join(t.TempDir(), "auth_fallback.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	previousRepo := storage.GetGlobalRepository()
	previousCfg := config.GetGlobalConfig()
	repo := storage.NewRepository(db)
	storage.SetGlobalRepository(repo)
	t.Cleanup(func() {
		storage.SetGlobalRepository(previousRepo)
		config.SetGlobalConfig(previousCfg)
	})

	// One registered key keeps the gateway out of open mode and gives the snapshot
	// something to answer. It is deliberately not the credential under test.
	seed := &storage.APIKeyRecord{Key: "sk-airoute-seededcredential000000000000", TenantID: "seed", Status: "active", RPM: 60}
	if err := repo.CreateAPIKey(seed); err != nil {
		t.Fatalf("create seed key: %v", err)
	}

	snapshot := []model.APIKeyConfig{seed.ToConfig()}
	if snapshotCredential != "" {
		extra := &storage.APIKeyRecord{Key: snapshotCredential, TenantID: "in-snapshot", Status: "active", RPM: 60}
		if err := repo.CreateAPIKey(extra); err != nil {
			t.Fatalf("create snapshot key: %v", err)
		}
		snapshot = append(snapshot, extra.ToConfig())
	}
	cfg := &config.Config{}
	cfg.UpdateAPIKeys(snapshot, true)
	config.SetGlobalConfig(cfg)

	engine := gin.New()
	engine.Use(AuthMiddleware())
	engine.GET("/v1/models", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"tenant": c.GetString(ContextKeyTenant),
			"key":    c.GetString(ContextKeyAPIKey),
		})
	})
	return &authHarness{t: t, repo: repo, engine: engine}
}

func (h *authHarness) get(credential string) *httptest.ResponseRecorder {
	h.t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	if credential != "" {
		req.Header.Set("Authorization", "Bearer "+credential)
	}
	w := httptest.NewRecorder()
	h.engine.ServeHTTP(w, req)
	return w
}

// errorCode reads the OpenAI-shaped error code from a rejection body.
func errorCode(t *testing.T, body []byte) string {
	t.Helper()
	var payload struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		t.Fatalf("decode rejection body %q: %v", body, err)
	}
	return payload.Error.Code
}

// A credential created on another replica is not in this replica's snapshot yet.
// It must authenticate anyway: otherwise the console hands out a key that the
// load balancer's next hop rejects.
func TestAuthResolvesCredentialMissingFromSnapshot(t *testing.T) {
	h := newAuthHarness(t, "")

	const fresh = "sk-airoute-freshkey000000000000000000000"
	rec := &storage.APIKeyRecord{Key: fresh, TenantID: "tenant-fresh", Status: "active", RPM: 120, GroupName: "vip"}
	if err := h.repo.CreateAPIKey(rec); err != nil {
		t.Fatalf("create fresh key: %v", err)
	}
	// Deliberately no config reload: the snapshot does not know this key, exactly
	// like a peer replica between the insert and its Redis reload broadcast.
	if cfg := config.GetGlobalConfig(); cfg.GetAPIKey(storage.HashAPIKey(fresh)) != nil {
		t.Fatal("snapshot unexpectedly carries the fresh key; the test would not exercise the fallback")
	}

	w := h.get(fresh)
	if w.Code != http.StatusOK {
		t.Fatalf("credential missing from the snapshot was rejected: status=%d body=%s", w.Code, w.Body.String())
	}
	var payload struct {
		Tenant string `json:"tenant"`
		Key    string `json:"key"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if payload.Tenant != "tenant-fresh" {
		t.Fatalf("tenant = %q, want the storage row's tenant-fresh", payload.Tenant)
	}
	if payload.Key != storage.HashAPIKey(fresh) {
		t.Fatalf("context key = %q, want the stored digest", payload.Key)
	}
	// The entitlements must come from the same mapping the snapshot uses.
	var cached *model.APIKeyConfig
	if cfg := config.GetGlobalConfig(); cfg != nil {
		cached = cfg.GetAPIKey(storage.HashAPIKey(fresh))
	}
	if cached != nil {
		t.Fatal("fallback must not mutate the snapshot")
	}
}

// A revoked key is a real credential whose snapshot entry is gone, so the
// fallback must answer "revoked or disabled", not "incorrect API key".
func TestAuthReportsRevokedCredentialMissingFromSnapshot(t *testing.T) {
	h := newAuthHarness(t, "")

	const revoked = "sk-airoute-revokedkey00000000000000000"
	rec := &storage.APIKeyRecord{Key: revoked, TenantID: "tenant-revoked", Status: "active", RPM: 60}
	if err := h.repo.CreateAPIKey(rec); err != nil {
		t.Fatalf("create key: %v", err)
	}
	if err := h.repo.DeleteAPIKey(rec.ID); err != nil {
		t.Fatalf("delete key: %v", err)
	}

	w := h.get(revoked)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("revoked key status = %d, want 401 (body=%s)", w.Code, w.Body.String())
	}
	if code := errorCode(t, w.Body.Bytes()); code != "api_key_revoked" {
		t.Fatalf("revoked key error code = %q, want api_key_revoked (body=%s)", code, w.Body.String())
	}
}

// A credential storage has never seen stays a plain invalid key.
func TestAuthRejectsUnknownCredentialStill(t *testing.T) {
	h := newAuthHarness(t, "")

	w := h.get("sk-airoute-neverexisted0000000000000000")
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("unknown credential status = %d, want 401", w.Code)
	}
	if code := errorCode(t, w.Body.Bytes()); code != "invalid_api_key" {
		t.Fatalf("unknown credential error code = %q, want invalid_api_key", code)
	}
}

// An empty credential keeps its dedicated message rather than turning into a
// storage lookup.
func TestAuthStillRejectsMissingCredential(t *testing.T) {
	h := newAuthHarness(t, "")

	w := h.get("")
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("missing credential status = %d, want 401", w.Code)
	}
	if code := errorCode(t, w.Body.Bytes()); code != "missing_api_key" {
		t.Fatalf("missing credential error code = %q, want missing_api_key", code)
	}
}

// The snapshot hit path must keep working and must not depend on the fallback.
func TestAuthSnapshotHitUnaffectedByFallback(t *testing.T) {
	const inSnapshot = "sk-airoute-insnapshot000000000000000000"
	h := newAuthHarness(t, inSnapshot)

	w := h.get(inSnapshot)
	if w.Code != http.StatusOK {
		t.Fatalf("snapshot credential status = %d, want 200 (body=%s)", w.Code, w.Body.String())
	}
}

func TestCredentialCouldExist(t *testing.T) {
	cases := []struct {
		name string
		key  string
		want bool
	}{
		{"console key", "sk-airoute-0123456789abcdef0123456789abcdef", true},
		{"short legacy shape", "sk-short", false},
		{"too long", "sk-" + string(make([]byte, 400)), false},
		{"embedded space", "sk-airoute-0123456789 abcdef0123456789", false},
		{"embedded newline", "sk-airoute-0123456789\nabcdef0123456789", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := credentialCouldExist(tc.key); got != tc.want {
				t.Fatalf("credentialCouldExist(%q) = %v, want %v", tc.key, got, tc.want)
			}
		})
	}
}

func TestRejectedCredentialCacheExpiresAndStaysBounded(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	cache := newRejectedCredentialCache()
	cache.now = func() time.Time { return now }

	const digest = "digest-a"
	if cache.recent(digest) {
		t.Fatal("fresh cache reported a rejection")
	}
	cache.remember(digest)
	if !cache.recent(digest) {
		t.Fatal("remembered rejection was not reported")
	}

	now = now.Add(rejectedCredentialTTL + time.Second)
	if cache.recent(digest) {
		t.Fatal("rejection outlived its TTL")
	}

	// Filling past capacity must evict rather than grow without bound, and must not
	// panic or wedge the cache.
	for i := 0; i < rejectedCredentialCapacity*2; i++ {
		cache.remember(string(rune('a'+i%26)) + "-" + time.Duration(i).String())
	}
	if len(cache.entries) > rejectedCredentialCapacity {
		t.Fatalf("cache grew to %d entries, want <= %d", len(cache.entries), rejectedCredentialCapacity)
	}
}
