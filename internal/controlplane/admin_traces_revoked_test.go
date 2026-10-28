package controlplane

import (
	"net/http"
	"testing"

	"github.com/ifnodoraemon/airoute/internal/storage"
)

// A revoked key's traces must stay visible to the account that owns them.
//
// Deleting a key is a soft delete, so the key list hides it — but the traces it
// produced are still the owner's audit history. Scoping with the live key list
// made a user's own trace history disappear as soon as the key was revoked
// (admin_logs.go already scoped with the historical list for the same reason).
func TestRequestTracesIncludeRevokedKeys(t *testing.T) {
	handler, repo := newChannelTestHandler(t)

	user := &storage.UserRecord{
		Username: "trace-owner", Email: "trace-owner@example.invalid", PasswordHash: "synthetic",
		Role: "user", Status: "active", Balance: 10, GroupName: "default",
	}
	if err := repo.CreateUser(user); err != nil {
		t.Fatalf("create user: %v", err)
	}

	live := &storage.APIKeyRecord{Key: "sk-trace-live-0123456789", TenantID: user.Username, UserID: user.ID, Status: "active"}
	if err := repo.CreateAPIKey(live); err != nil {
		t.Fatalf("create live key: %v", err)
	}
	revoked := &storage.APIKeyRecord{Key: "sk-trace-revoked-012345", TenantID: user.Username, UserID: user.ID, Status: "active"}
	if err := repo.CreateAPIKey(revoked); err != nil {
		t.Fatalf("create revoked key: %v", err)
	}
	if err := repo.DeleteAPIKey(revoked.ID); err != nil {
		t.Fatalf("revoke key: %v", err)
	}

	for _, rec := range []struct {
		traceID string
		key     *storage.APIKeyRecord
	}{
		{"tr-live-request", live},
		{"tr-revoked-request", revoked},
	} {
		if err := repo.RecordRequestTrace(&storage.RequestTraceRecord{
			TraceID: rec.traceID, APIKey: rec.key.Key, TenantID: user.Username,
			Model: "m1", StatusCode: http.StatusOK,
		}); err != nil {
			t.Fatalf("record trace %s: %v", rec.traceID, err)
		}
	}

	w := invokeKeyWithClaims(t, handler.ListRequestTraces, &AdminClaims{Username: user.Username, Role: "user"},
		http.MethodGet, "", "")
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", w.Code, w.Body.String())
	}

	envelope := decodeEnvelope(t, w)
	traces, _ := envelope["data"].([]any)
	if len(traces) != 2 {
		t.Fatalf("traces = %d, want both the live and the revoked key's trace: %s", len(traces), w.Body.String())
	}
	seen := map[string]bool{}
	for _, entry := range traces {
		asMap, _ := entry.(map[string]any)
		if traceID, ok := asMap["trace_id"].(string); ok {
			seen[traceID] = true
		}
	}
	if !seen["tr-live-request"] || !seen["tr-revoked-request"] {
		t.Errorf("trace ids = %v, want both tr-live-request and tr-revoked-request", seen)
	}
}
