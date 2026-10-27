package middleware

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/ifnodoraemon/airoute/internal/config"
	"github.com/ifnodoraemon/airoute/internal/storage"
)

// Regression (A12): the runtime snapshot is keyed by the stored digest, and a
// client still authenticates with the cleartext credential. This exercises the
// real key lifecycle: repository insert -> snapshot load -> middleware lookup.
func TestAuthAcceptsCleartextCredentialAgainstDigestSnapshot(t *testing.T) {
	gin.SetMode(gin.TestMode)

	db, err := storage.OpenDB(filepath.Join(t.TempDir(), "auth.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	repo := storage.NewRepository(db)
	storage.SetGlobalRepository(repo)
	t.Cleanup(func() { storage.SetGlobalRepository(nil) })

	const credential = "sk-airoute-authdigest000000000000000000"
	rec := &storage.APIKeyRecord{Key: credential, TenantID: "tenant-a", Status: "active", RPM: 60, TPM: 1000}
	if err := repo.CreateAPIKey(rec); err != nil {
		t.Fatalf("create key: %v", err)
	}

	// The data-plane snapshot is built from api_keys.key (digests).
	keys, err := repo.ToModelAPIKeys()
	if err != nil {
		t.Fatalf("build key snapshot: %v", err)
	}
	if len(keys) != 1 {
		t.Fatalf("snapshot carries %d keys, want 1", len(keys))
	}
	if keys[0].Key != storage.HashAPIKey(credential) {
		t.Fatalf("snapshot key = %q, want the stored digest", keys[0].Key)
	}

	cfg := &config.Config{}
	cfg.UpdateAPIKeys(keys, true)
	config.SetGlobalConfig(cfg)

	engine := gin.New()
	engine.Use(AuthMiddleware())
	engine.GET("/v1/models", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"tenant": c.GetString(ContextKeyTenant),
			"key":    c.GetString(ContextKeyAPIKey),
			"key_id": c.GetString(ContextKeyAPIKeyID),
		})
	})

	req := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	req.Header.Set("Authorization", "Bearer "+credential)
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("credential rejected: status=%d body=%s", w.Code, w.Body.String())
	}
	body := w.Body.String()
	// The request context must carry the digest (audit rows / ledger joins use it)
	// and never the cleartext credential.
	if contains(body, credential) {
		t.Fatalf("response context exposes the cleartext credential: %s", body)
	}
	if !contains(body, storage.HashAPIKey(credential)) {
		t.Fatalf("request context does not carry the key digest: %s", body)
	}
	if contains(body, rec.PublicID) == false {
		t.Fatalf("request context does not carry the key's public id: %s", body)
	}
	if contains(body, "kfp_") {
		t.Fatalf("request context still carries the removed fingerprint: %s", body)
	}

	// The stored digest must not work as a bearer credential: hashing it again can
	// never match the stored digest.
	req2 := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	req2.Header.Set("Authorization", "Bearer "+storage.HashAPIKey(credential))
	w2 := httptest.NewRecorder()
	engine.ServeHTTP(w2, req2)
	if w2.Code != http.StatusUnauthorized {
		t.Fatalf("the stored digest was accepted as a credential: status=%d", w2.Code)
	}
}

func contains(haystack, needle string) bool {
	return len(needle) == 0 || (len(haystack) >= len(needle) && indexOf(haystack, needle) >= 0)
}

func indexOf(haystack, needle string) int {
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if haystack[i:i+len(needle)] == needle {
			return i
		}
	}
	return -1
}
