package storage

import (
	"encoding/json"
	"strings"
	"testing"
)

// TestNewAPIKeyPublicIDIsIndependentOfTheCredential pins what the console relies
// on: a recognisable fixed shape, uniqueness per key, and no relationship to the
// secret or its digest.
func TestNewAPIKeyPublicIDIsIndependentOfTheCredential(t *testing.T) {
	first, err := NewAPIKeyPublicID()
	if err != nil {
		t.Fatalf("generate public id: %v", err)
	}
	if !strings.HasPrefix(first, apiKeyPublicIDPrefix) {
		t.Fatalf("public id = %q, want the %q prefix", first, apiKeyPublicIDPrefix)
	}
	if len(first) != len(apiKeyPublicIDPrefix)+apiKeyPublicIDBytes*2 {
		t.Fatalf("public id = %q, want %d hex characters", first, apiKeyPublicIDBytes*2)
	}
	for _, c := range strings.TrimPrefix(first, apiKeyPublicIDPrefix) {
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') {
			t.Fatalf("public id = %q, want lowercase hex after the prefix", first)
		}
	}
	if IsAPIKeyHash(first) {
		t.Fatalf("public id %q is digest-shaped and could be confused with a stored key", first)
	}

	second, err := NewAPIKeyPublicID()
	if err != nil {
		t.Fatalf("generate second public id: %v", err)
	}
	if first == second {
		t.Fatal("two keys received the same public id")
	}
}

// TestMaskAPIKeyNeverShowsTheWholeCredential covers the audit masking helper.
func TestMaskAPIKeyNeverShowsTheWholeCredential(t *testing.T) {
	if got := MaskAPIKey(""); got != "" {
		t.Fatalf("empty key mask = %q", got)
	}
	if got := MaskAPIKey("sk-short"); got != "sk-***" {
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

// TestRepositoryRowsCarryThePublicID verifies the identifier is present on every
// read path the console and audit use. It is minted at creation, so no caller has
// to derive it.
func TestRepositoryRowsCarryThePublicID(t *testing.T) {
	db, err := OpenDB(":memory:")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	repository := NewRepository(db)
	rec := &APIKeyRecord{
		Key: "sk-airoute-publicid-0123456789", TenantID: "tenant", Status: "active", GroupName: "default",
	}
	if err := repository.CreateAPIKey(rec); err != nil {
		t.Fatalf("create: %v", err)
	}
	if rec.PublicID == "" || !strings.HasPrefix(rec.PublicID, apiKeyPublicIDPrefix) {
		t.Fatalf("created record public id = %q, want an %q identifier", rec.PublicID, apiKeyPublicIDPrefix)
	}
	want := rec.PublicID

	byID, err := repository.GetAPIKey(rec.ID)
	if err != nil {
		t.Fatalf("get by id: %v", err)
	}
	if byID.PublicID != want {
		t.Fatalf("GetAPIKey public id = %q, want %q", byID.PublicID, want)
	}
	byKey, err := repository.GetAPIKeyByKey(rec.Key)
	if err != nil {
		t.Fatalf("get by key: %v", err)
	}
	if byKey.PublicID != want {
		t.Fatalf("GetAPIKeyByKey public id = %q, want %q", byKey.PublicID, want)
	}

	listed, err := repository.ListAPIKeys()
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(listed) != 1 || listed[0].PublicID != want {
		t.Fatalf("ListAPIKeys public id = %+v, want %q", listed, want)
	}
	listedByUser, err := repository.ListAPIKeysByUser(byKey.UserID)
	if err != nil {
		t.Fatalf("list by user: %v", err)
	}
	if len(listedByUser) != 1 || listedByUser[0].PublicID != want {
		t.Fatalf("ListAPIKeysByUser public id = %+v, want %q", listedByUser, want)
	}
	if got := byID.ToConfig().PublicID; got != want {
		t.Fatalf("ToConfig public id = %q, want %q", got, want)
	}
}

// TestPublicIDCannotAuthenticate proves the identifier is display-only: the auth
// lookup hashes what is presented, so a public id never matches a stored digest.
func TestPublicIDCannotAuthenticate(t *testing.T) {
	db, err := OpenDB(":memory:")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	repository := NewRepository(db)
	rec := &APIKeyRecord{Key: "sk-airoute-cannot-auth-0123456789", TenantID: "t", Status: "active"}
	if err := repository.CreateAPIKey(rec); err != nil {
		t.Fatalf("create: %v", err)
	}
	if _, err := repository.GetAPIKeyBySecret(t.Context(), rec.PublicID); err == nil {
		t.Fatalf("the public id %q was accepted as a credential", rec.PublicID)
	}
}

// TestAPIKeyRecordJSONNeverCarriesTheDigest pins the serialization contract: the
// stored digest has no JSON representation at all, and the public id does.
func TestAPIKeyRecordJSONNeverCarriesTheDigest(t *testing.T) {
	const credential = "sk-airoute-ffffffffffffffffffffffffffffffff"
	digest := HashAPIKey(credential)
	payload, err := json.Marshal(&APIKeyRecord{ID: 7, PublicID: "ak_0123456789abcdef0123", Key: digest, TenantID: "t"})
	if err != nil {
		t.Fatalf("marshal record: %v", err)
	}
	if strings.Contains(string(payload), digest) || strings.Contains(string(payload), `"key":`) {
		t.Fatalf("serialized record leaks the key digest: %s", payload)
	}
	if !strings.Contains(string(payload), `"key_id":"ak_0123456789abcdef0123"`) {
		t.Fatalf("serialized record does not expose key_id: %s", payload)
	}
}
