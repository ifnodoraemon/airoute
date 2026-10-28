package controlplane

import (
	"strings"
	"testing"

	"github.com/ifnodoraemon/airoute/internal/storage"
)

// The call log must name a key the way the key list and the access log do
// (ak_…), while a digest that no longer resolves stays hidden.
func TestPresentLogKeysNamesRowsWithPublicID(t *testing.T) {
	db, err := storage.OpenDB(":memory:")
	if err != nil {
		t.Fatalf("open memory db: %v", err)
	}
	defer func() { _ = db.Close() }()
	repo := storage.NewRepository(db)
	rec := &storage.APIKeyRecord{Key: "sk-plaintext-for-display-test-0001", TenantID: "tenant-a", Status: "active", RPM: 60, TPM: 1000}
	if err := repo.CreateAPIKey(rec); err != nil {
		t.Fatalf("create key: %v", err)
	}
	digest := rec.Key
	unknownDigest := strings.Repeat("cd", 32)

	h := &AdminHandler{repo: repo}
	logs := []*storage.UsageLogRecord{
		{APIKey: digest},
		{APIKey: unknownDigest},
		{APIKey: "ak_alreadyresolved"},
		{APIKey: ""},
	}
	h.presentLogKeys(logs, false)

	if logs[0].APIKey != rec.PublicID {
		t.Errorf("resolved row = %q, want %q", logs[0].APIKey, rec.PublicID)
	}
	if logs[1].APIKey == unknownDigest || !strings.Contains(logs[1].APIKey, "***") {
		t.Errorf("unresolved digest must be masked, got %q", logs[1].APIKey)
	}
	if logs[2].APIKey != "ak_alreadyresolved" {
		t.Errorf("public ids must pass through untouched, got %q", logs[2].APIKey)
	}
	if logs[3].APIKey != "" {
		t.Errorf("empty keys must stay empty, got %q", logs[3].APIKey)
	}

	revealed := []*storage.UsageLogRecord{{APIKey: unknownDigest}}
	h.presentLogKeys(revealed, true)
	if revealed[0].APIKey != unknownDigest {
		t.Errorf("reveal should keep the raw digest, got %q", revealed[0].APIKey)
	}
}
