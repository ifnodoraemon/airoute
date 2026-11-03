package storage

import (
	"strings"
	"testing"
)

// Audit rows persist the key digest, so the console has to resolve it back to the
// public identifier. These helpers are that bridge; a mismatch here would make
// every call log row display an unrelated key.
func TestAPIKeyPublicIDDigestResolution(t *testing.T) {
	db, err := OpenDB(":memory:")
	if err != nil {
		t.Fatalf("open memory db: %v", err)
	}
	defer func() { _ = db.Close() }()
	repo := NewRepository(db)

	rec := &APIKeyRecord{Key: "sk-plaintext-for-resolution-test-0001", TenantID: "tenant-a", Status: "active", RPM: 60, TPM: 1000}
	if err := repo.CreateAPIKey(rec); err != nil {
		t.Fatalf("create key: %v", err)
	}
	digest := rec.Key
	publicID := rec.PublicID
	if digest == "" || publicID == "" {
		t.Fatalf("expected digest and public id, got %q / %q", digest, publicID)
	}
	if IsAPIKeyPublicID(digest) {
		t.Fatalf("the stored credential reference must not be a public id: %q", digest)
	}
	if !IsAPIKeyPublicID(publicID) {
		t.Fatalf("public id should carry the ak_ prefix: %q", publicID)
	}

	ids, err := repo.APIKeyPublicIDsByDigest([]string{digest, digest, "", strings.Repeat("ab", 32)})
	if err != nil {
		t.Fatalf("resolve public ids: %v", err)
	}
	if len(ids) != 1 || ids[digest] != publicID {
		t.Fatalf("unexpected resolution map: %#v", ids)
	}

	back, err := repo.APIKeyDigestByPublicID(publicID)
	if err != nil || back != digest {
		t.Fatalf("digest by public id = %q, %v; want %q", back, err, digest)
	}
	if d, err := repo.APIKeyDigestByPublicID("ak_unknownidentifier"); err != nil || d != "" {
		t.Fatalf("unknown public id must resolve to empty, got %q, %v", d, err)
	}
	if d, err := repo.APIKeyDigestByPublicID("sk-not-a-public-id"); err != nil || d != "" {
		t.Fatalf("a non-public value must not be resolved, got %q, %v", d, err)
	}
	if ids, err := repo.APIKeyPublicIDsByDigest(nil); err != nil || len(ids) != 0 {
		t.Fatalf("empty input must stay empty, got %#v, %v", ids, err)
	}
}
