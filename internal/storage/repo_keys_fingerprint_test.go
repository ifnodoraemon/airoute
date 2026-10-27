package storage

import (
	"strings"
	"testing"
)

// TestAPIKeyFingerprintIsStableAndNonReversible pins the identifier used to refer
// to a credential without copying it (A12 groundwork).
func TestAPIKeyFingerprintIsStableAndNonReversible(t *testing.T) {
	key := "sk-airoute-0123456789abcdef0123456789abcdef"
	fp := APIKeyFingerprint(key)

	if fp != APIKeyFingerprint(key) {
		t.Fatal("fingerprint is not stable for the same key")
	}
	if !strings.HasPrefix(fp, "kfp_") {
		t.Fatalf("fingerprint = %q, want the kfp_ prefix", fp)
	}
	if len(fp) != len("kfp_")+16 {
		t.Fatalf("fingerprint = %q, want 8 bytes of hex", fp)
	}
	if strings.Contains(fp, key) || strings.Contains(fp, "0123456789abcdef") {
		t.Fatalf("fingerprint leaks key material: %q", fp)
	}
	if APIKeyFingerprint(key) == APIKeyFingerprint(key+"x") {
		t.Fatal("different keys must produce different fingerprints")
	}
	if APIKeyFingerprint("") != "" {
		t.Fatal("an empty key has no fingerprint")
	}
}

// TestMaskAPIKeyNeverShowsTheWholeCredential covers the shared masking helper.
func TestMaskAPIKeyNeverShowsTheWholeCredential(t *testing.T) {
	if got := MaskAPIKey(""); got != "" {
		t.Fatalf("empty key mask = %q", got)
	}
	short := "sk-short"
	if got := MaskAPIKey(short); got != "sk-***" {
		t.Fatalf("short key mask = %q", got)
	}
	key := "sk-airoute-0123456789abcdef0123456789abcdef"
	got := MaskAPIKey(key)
	if got == key || strings.Contains(got, "0123456789abcdef") {
		t.Fatalf("masked key still contains the secret: %q", got)
	}
	if !strings.HasPrefix(got, "sk-airou") || !strings.HasSuffix(got, "cdef") {
		t.Fatalf("mask = %q, want prefix+last4 shape", got)
	}
}

// TestRepositoryRowsCarryAFingerprint verifies the derived identifier is present
// on rows returned by every read path the console/audit uses, so a future ledger
// migration can join on it.
func TestRepositoryRowsCarryAFingerprint(t *testing.T) {
	db, err := OpenDB(":memory:")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	repository := NewRepository(db)
	rec := &APIKeyRecord{
		Key: "sk-airoute-fingerprint-0123456789", TenantID: "tenant", Status: "active", GroupName: "default",
	}
	if err := repository.CreateAPIKey(rec); err != nil {
		t.Fatalf("create: %v", err)
	}
	want := APIKeyFingerprint(rec.Key)
	if rec.Fingerprint != want {
		t.Fatalf("created record fingerprint = %q, want %q", rec.Fingerprint, want)
	}

	byID, err := repository.GetAPIKey(rec.ID)
	if err != nil {
		t.Fatalf("get by id: %v", err)
	}
	if byID.Fingerprint != want {
		t.Fatalf("GetAPIKey fingerprint = %q, want %q", byID.Fingerprint, want)
	}
	byKey, err := repository.GetAPIKeyByKey(rec.Key)
	if err != nil {
		t.Fatalf("get by key: %v", err)
	}
	if byKey.Fingerprint != want {
		t.Fatalf("GetAPIKeyByKey fingerprint = %q, want %q", byKey.Fingerprint, want)
	}

	listed, err := repository.ListAPIKeys()
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(listed) != 1 || listed[0].Fingerprint != want {
		t.Fatalf("ListAPIKeys fingerprint = %+v, want %q", listed, want)
	}

	historical, err := repository.ListHistoricalAPIKeysByUser(byKey.UserID)
	if err != nil {
		t.Fatalf("historical: %v", err)
	}
	if len(historical) != 1 || historical[0].Fingerprint != want {
		t.Fatalf("ListHistoricalAPIKeysByUser fingerprint = %+v, want %q", historical, want)
	}
}
