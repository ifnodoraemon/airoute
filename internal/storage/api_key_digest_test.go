package storage

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
)

func newDigestRepo(t *testing.T) *Repository {
	t.Helper()
	db, err := OpenDB(filepath.Join(t.TempDir(), "digests.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return NewRepository(db)
}

// Regression (A12): a newly created credential must be stored as a digest, and the
// presented credential must still authenticate.
func TestAPIKeyIsStoredAsDigestAndStillAuthenticates(t *testing.T) {
	repo := newDigestRepo(t)
	const credential = "sk-airoute-0123456789abcdef0123456789abcdef"

	rec := &APIKeyRecord{Key: credential, TenantID: "tenant", Status: "active"}
	if err := repo.CreateAPIKey(rec); err != nil {
		t.Fatalf("create key: %v", err)
	}

	// The row must not contain the credential.
	var stored string
	if err := repo.db.QueryRow(`SELECT key FROM api_keys WHERE id = ?`, rec.ID).Scan(&stored); err != nil {
		t.Fatalf("read stored key: %v", err)
	}
	if stored == credential {
		t.Fatal("api_keys.key still stores the cleartext credential")
	}
	if stored != HashAPIKey(credential) {
		t.Fatalf("api_keys.key = %q, want the SHA-256 digest", stored)
	}
	if !IsAPIKeyHash(stored) {
		t.Fatalf("stored value %q is not a digest", stored)
	}

	// The in-memory record reflects the row, so callers cannot mistake it for the
	// credential.
	if rec.Key != stored {
		t.Fatalf("record key %q does not match the stored digest %q", rec.Key, stored)
	}

	// Authentication path: hash the presented credential and resolve it.
	got, err := repo.GetAPIKeyBySecret(context.Background(), credential)
	if err != nil {
		t.Fatalf("credential no longer authenticates: %v", err)
	}
	if got.ID != rec.ID {
		t.Fatalf("resolved key id = %d, want %d", got.ID, rec.ID)
	}

	// A digest must never be usable as a bearer credential (it is not the secret).
	if _, err := repo.GetAPIKeyBySecret(context.Background(), stored); err == nil {
		t.Fatal("the stored digest was accepted as a credential")
	}
}

// The display helpers must never expose the credential.
func TestKeyDisplayHelpersNeverExposeCredential(t *testing.T) {
	const credential = "sk-airoute-ffffffffffffffffffffffffffffffff"
	digest := HashAPIKey(credential)

	masked := MaskStoredAPIKey(digest)
	if strings.Contains(masked, credential) || masked == digest {
		t.Fatalf("masked value %q leaks or equals the digest", masked)
	}
	if !strings.Contains(masked, "***") {
		t.Fatalf("masked value %q has no mask marker", masked)
	}

	// The fingerprint must be identical whether derived from the credential or the
	// digest, and must not contain the credential.
	if fromSecret, fromDigest := APIKeyFingerprint(credential), APIKeyFingerprint(digest); fromSecret != fromDigest {
		t.Fatalf("fingerprint differs by input form: %q vs %q", fromSecret, fromDigest)
	}
	if fp := APIKeyFingerprint(credential); strings.Contains(fp, credential) || len(fp) > 32 {
		t.Fatalf("fingerprint %q is not a bounded non-reversible identifier", fp)
	}

	// MaskForDisplay is what list endpoints call.
	rec := &APIKeyRecord{Key: digest}
	rec.MaskForDisplay()
	if rec.Key == digest || strings.Contains(rec.Key, credential) {
		t.Fatalf("display key %q exposes the stored digest", rec.Key)
	}
}

// Ingestion must hash any legacy cleartext reference so a pre-migration spill or
// Redis message cannot write a credential into the audit trail, while still
// attributing the charge to the migrated key.
func TestUsageLogIngestionNeverPersistsCleartextKey(t *testing.T) {
	repo := newDigestRepo(t)
	const credential = "sk-airoute-11111111111111111111111111111111"

	key := &APIKeyRecord{Key: credential, TenantID: "tenant", Status: "active"}
	if err := repo.CreateAPIKey(key); err != nil {
		t.Fatalf("create key: %v", err)
	}

	// Simulate a record produced by a pre-migration binary (cleartext api_key) that
	// is still in flight during the rollout.
	if err := repo.RecordUsageLog(&UsageLogRecord{
		APIKey:     credential,
		TenantID:   "tenant",
		Model:      "m",
		Cost:       1,
		StatusCode: 200,
	}); err != nil {
		t.Fatalf("record usage: %v", err)
	}

	var storedKey string
	if err := repo.db.QueryRow(`SELECT api_key FROM usage_logs ORDER BY id DESC LIMIT 1`).Scan(&storedKey); err != nil {
		t.Fatalf("read usage row: %v", err)
	}
	if storedKey == credential {
		t.Fatal("usage_logs.api_key persists the cleartext credential")
	}
	if storedKey != HashAPIKey(credential) {
		t.Fatalf("usage_logs.api_key = %q, want the key digest (attribution preserved)", storedKey)
	}
}

// Regression: the console filters logs by key id, which resolves to api_keys.key.
// After hashing, that filter must still match the stored usage rows.
func TestUsageLogFilterByKeyIDStillMatchesAfterHashing(t *testing.T) {
	repo := newDigestRepo(t)
	key := &APIKeyRecord{Key: "sk-airoute-22222222222222222222222222222222", TenantID: "t", Status: "active"}
	if err := repo.CreateAPIKey(key); err != nil {
		t.Fatalf("create key: %v", err)
	}
	for i := 0; i < 3; i++ {
		if err := repo.RecordUsageLog(&UsageLogRecord{APIKey: key.Key, TenantID: "t", Model: "m", StatusCode: 200}); err != nil {
			t.Fatalf("record usage: %v", err)
		}
	}

	// The admin handler resolves selected key ids to api_keys.key values.
	listed, err := repo.GetAPIKey(key.ID)
	if err != nil {
		t.Fatalf("get key: %v", err)
	}
	logs, total, err := repo.ListUsageLogsWithFilterAndCount(LogFilter{APIKeys: []string{listed.Key}, ScopeByAPIKeys: true, Limit: 10})
	if err != nil {
		t.Fatalf("list logs: %v", err)
	}
	if total != 3 || len(logs) != 3 {
		t.Fatalf("key-filtered log query returned total=%d rows=%d, want 3", total, len(logs))
	}

	// Scoped stats use the same identifiers.
	stats, err := repo.GetStatsOverviewForKeys([]string{listed.Key})
	if err != nil {
		t.Fatalf("scoped stats: %v", err)
	}
	if stats.TotalRequests != 3 {
		t.Fatalf("scoped stats requests = %d, want 3", stats.TotalRequests)
	}
}
