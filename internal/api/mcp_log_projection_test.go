package api

import (
	"strings"
	"testing"

	"github.com/ifnodoraemon/airoute/internal/storage"
)

// D-03: the audit rows carry the key digest, so the agent-facing tool must name
// keys the way the console and the access log do. A digest that no longer
// resolves is masked rather than returned raw.
func TestPresentMCPLogKeys(t *testing.T) {
	h, repo := newMCPTestRepo(t)

	key := &storage.APIKeyRecord{Key: "sk-mcp-projection-0001", TenantID: "tenant-a", Status: "active", GroupName: "default"}
	if err := repo.CreateAPIKey(key); err != nil {
		t.Fatalf("create key: %v", err)
	}
	digest := key.Key // CreateAPIKey stores and returns the digest
	unknownDigest := strings.Repeat("cd", 32)

	logs := []*storage.UsageLogRecord{
		{APIKey: digest},
		{APIKey: unknownDigest},
		{APIKey: key.PublicID},
		{APIKey: ""},
		{},
	}
	logs[4] = nil

	h.presentMCPLogKeys(logs)

	if logs[0].APIKey != key.PublicID {
		t.Errorf("resolved digest = %q, want public id %q", logs[0].APIKey, key.PublicID)
	}
	if !strings.Contains(logs[1].APIKey, "***") {
		t.Errorf("unresolved digest must be masked, got %q", logs[1].APIKey)
	}
	if logs[2].APIKey != key.PublicID {
		t.Errorf("public id must pass through untouched, got %q", logs[2].APIKey)
	}
	if logs[3].APIKey != "" {
		t.Errorf("empty key must stay empty, got %q", logs[3].APIKey)
	}
}

// The projection must never leak the raw digest when the key row is gone.
func TestPresentMCPLogKeysMasksWithoutRepository(t *testing.T) {
	h := NewMCPHandler(nil, nil)
	digest := strings.Repeat("ab", 32)
	logs := []*storage.UsageLogRecord{{APIKey: digest}}
	h.presentMCPLogKeys(logs)
	if logs[0].APIKey == digest {
		t.Fatal("a missing repository must leave the digest masked, not raw")
	}
}
