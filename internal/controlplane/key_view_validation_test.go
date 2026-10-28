package controlplane

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/ifnodoraemon/airoute/internal/storage"
)

// invokeKeyWithClaims runs a key handler with the caller's claims attached to the
// gin context, which is how the session middleware presents a logged-in caller.
func invokeKeyWithClaims(t *testing.T, handler gin.HandlerFunc, claims *AdminClaims, method, body, id string) *httptest.ResponseRecorder {
	t.Helper()
	return invokeWithClaims(t, handler, claims, method, body, id)
}

// invokeWithClaims mirrors invokeChannel but attaches the caller's claims first.
func invokeWithClaims(t *testing.T, handler gin.HandlerFunc, claims *AdminClaims, method, body, id string) *httptest.ResponseRecorder {
	t.Helper()
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request, _ = http.NewRequest(method, "/api/v1/admin/keys", strings.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	if id != "" {
		c.Params = gin.Params{{Key: "id", Value: id}}
	}
	if claims != nil {
		c.Set("admin_claims", claims)
	}
	handler(c)
	return w
}

func containsString(haystack, needle string) bool {
	return strings.Contains(haystack, needle)
}

// TestUpdateAPIKeyReturnsTheKeyView pins the serialization path of PUT
// /admin/keys/:id: the response must be apiKeyView, not the storage row.
//
// The row's JSON shape is owned by storage, so returning it made this endpoint's
// payload drift with the schema while create and list stayed pinned to the view.
func TestUpdateAPIKeyReturnsTheKeyView(t *testing.T) {
	handler, repo := newChannelTestHandler(t)

	record := &storage.APIKeyRecord{Key: "sk-view-update-0123456789", TenantID: "tenant-a", Status: "active", AllowedModels: []string{"*"}}
	if err := repo.CreateAPIKey(record); err != nil {
		t.Fatalf("create key: %v", err)
	}

	w := invokeKeyWithClaims(t, handler.UpdateAPIKey, &AdminClaims{Username: "admin", Role: "admin"},
		http.MethodPut, `{"status":"disabled"}`, int64String(record.ID))
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", w.Code, w.Body.String())
	}

	envelope := decodeEnvelope(t, w)
	data, ok := envelope["data"].(map[string]any)
	if !ok {
		t.Fatalf("data is not an object: %s", w.Body.String())
	}
	// The view names the public identifier; neither the digest nor the credential
	// has a field in it.
	if data["key_id"] != record.PublicID {
		t.Errorf("key_id = %v, want %q", data["key_id"], record.PublicID)
	}
	if _, exists := data["key"]; exists {
		t.Errorf("response carries a credential field: %s", w.Body.String())
	}
	for _, forbidden := range []string{record.Key, "sk-view-update-0123456789"} {
		if containsString(w.Body.String(), forbidden) {
			t.Errorf("response leaks %q: %s", forbidden, w.Body.String())
		}
	}
	if data["status"] != "disabled" {
		t.Errorf("status = %v, want disabled", data["status"])
	}
}

// TestAPIKeyFormatValidationRejectsUnknownLevels pins the accepted values of
// format_validation on both write paths.
//
// The field travelled from request to storage to response without validation, so
// a typo such as "stirct" looked configured while the resolver silently ignored
// it. Only the levels internal/config understands are accepted.
func TestAPIKeyFormatValidationRejectsUnknownLevels(t *testing.T) {
	handler, _ := newChannelTestHandler(t)
	admin := &AdminClaims{Username: "admin", Role: "admin"}

	t.Run("create", func(t *testing.T) {
		w := invokeKeyWithClaims(t, handler.CreateAPIKey, admin, http.MethodPost,
			`{"tenant_id":"t1","format_validation":"stirct"}`, "")
		if w.Code != http.StatusBadRequest {
			t.Fatalf("create with an unknown level = %d, want 400: %s", w.Code, w.Body.String())
		}
	})

	// A valid level must persist and come back on the view.
	created := invokeKeyWithClaims(t, handler.CreateAPIKey, admin, http.MethodPost,
		`{"tenant_id":"tenant-a","format_validation":"lenient"}`, "")
	if created.Code != http.StatusOK {
		t.Fatalf("valid create = %d: %s", created.Code, created.Body.String())
	}
	createdData, _ := decodeEnvelope(t, created)["data"].(map[string]any)
	id, _ := createdData["id"].(float64)
	if id == 0 {
		t.Fatalf("creation response has no id: %s", created.Body.String())
	}
	if createdData["format_validation"] != "lenient" {
		t.Errorf("created format_validation = %v, want lenient", createdData["format_validation"])
	}

	t.Run("update", func(t *testing.T) {
		w := invokeKeyWithClaims(t, handler.UpdateAPIKey, admin, http.MethodPut,
			`{"format_validation":"nonsense"}`, int64String(int64(id)))
		if w.Code != http.StatusBadRequest {
			t.Fatalf("update with an unknown level = %d, want 400: %s", w.Code, w.Body.String())
		}
	})

	t.Run("update accepts a known level", func(t *testing.T) {
		w := invokeKeyWithClaims(t, handler.UpdateAPIKey, admin, http.MethodPut,
			`{"format_validation":"strict"}`, int64String(int64(id)))
		if w.Code != http.StatusOK {
			t.Fatalf("update with a valid level = %d: %s", w.Code, w.Body.String())
		}
		data, _ := decodeEnvelope(t, w)["data"].(map[string]any)
		if data["format_validation"] != "strict" {
			t.Errorf("format_validation = %v, want strict", data["format_validation"])
		}
	})
}

// TestNormalizeFormatValidation pins the canonicalisation: the resolver compares
// lowercase levels, so a mixed-case write must not become a value it ignores.
func TestNormalizeFormatValidation(t *testing.T) {
	cases := []struct {
		in      string
		want    string
		wantErr bool
	}{
		{"", "", false},
		{"off", "off", false},
		{"LENIENT", "lenient", false},
		{" strict ", "strict", false},
		{"stirct", "", true},
	}
	for _, tc := range cases {
		got, err := normalizeFormatValidation(tc.in)
		if tc.wantErr {
			if err == nil {
				t.Errorf("normalizeFormatValidation(%q) = %q, want an error", tc.in, got)
			}
			continue
		}
		if err != nil {
			t.Errorf("normalizeFormatValidation(%q): %v", tc.in, err)
			continue
		}
		if got != tc.want {
			t.Errorf("normalizeFormatValidation(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}
