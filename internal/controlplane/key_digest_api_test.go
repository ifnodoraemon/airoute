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

// Regression (A12): the key creation response returns the credential exactly once,
// while list responses expose only a mask and the fingerprint. Neither response may
// contain the stored digest (it must never be presented as a usable key).
func TestKeyCreationReturnsPlaintextOnceAndListsMasked(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h, repo := regressionHandler(t)

	user := &storage.UserRecord{Username: "keyowner", Email: "keyowner@example.invalid", PasswordHash: "x", Role: "user", Status: "active", Balance: 10, GroupName: "default"}
	if err := repo.CreateUser(user); err != nil {
		t.Fatalf("create user: %v", err)
	}

	// Creation through the user-facing handler with a session for that user.
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/api/v1/keys", strings.NewReader(`{"tenant_id":"keyowner","rpm":60}`))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set("admin_claims", &AdminClaims{Username: "keyowner", Role: "user"})
	h.CreateUserKey(c)
	if w.Code != http.StatusOK {
		t.Fatalf("create key status=%d body=%s", w.Code, w.Body.String())
	}

	var created struct {
		Data struct {
			Key         string `json:"key"`
			Fingerprint string `json:"key_fingerprint"`
		} `json:"data"`
		KeyReturnedOnce bool `json:"key_returned_once"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode creation response: %v", err)
	}
	plaintext := created.Data.Key
	if plaintext == "" {
		t.Fatalf("creation response did not return a credential: %s", w.Body.String())
	}
	if !created.KeyReturnedOnce {
		t.Fatalf("creation response is not marked as one-time: %s", w.Body.String())
	}
	if !strings.HasPrefix(plaintext, "sk-airoute-") {
		t.Fatalf("returned credential %q is not a generated key", plaintext)
	}

	// The credential is usable and stored only as a digest.
	digest := storage.HashAPIKey(plaintext)
	var stored string
	if err := repo.GetDB().QueryRow(`SELECT key FROM api_keys WHERE tenant_id = 'keyowner'`).Scan(&stored); err != nil {
		t.Fatalf("read stored key: %v", err)
	}
	if stored != digest {
		t.Fatalf("stored key = %q, want the digest", stored)
	}
	if _, err := repo.GetAPIKeyBySecret(t.Context(), plaintext); err != nil {
		t.Fatalf("returned credential cannot authenticate: %v", err)
	}

	// Listing must not disclose the credential or the digest.
	listW := httptest.NewRecorder()
	listC, _ := gin.CreateTestContext(listW)
	listC.Request = httptest.NewRequest(http.MethodGet, "/api/v1/user/keys", nil)
	listC.Set("admin_claims", &AdminClaims{Username: "keyowner", Role: "user"})
	h.ListUserKeys(listC)
	if listW.Code != http.StatusOK {
		t.Fatalf("list keys status=%d body=%s", listW.Code, listW.Body.String())
	}
	body := listW.Body.String()
	if strings.Contains(body, plaintext) {
		t.Fatalf("list response discloses the credential: %s", body)
	}
	if strings.Contains(body, digest) {
		t.Fatalf("list response discloses the stored digest as a key: %s", body)
	}
	if !strings.Contains(body, "***") {
		t.Fatalf("list response does not carry a masked key: %s", body)
	}
}
