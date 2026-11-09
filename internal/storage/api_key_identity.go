package storage

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"strings"
)

// HashAPIKey returns the value stored for a bearer credential.
//
// Credentials are never persisted in cleartext: api_keys.key holds this digest,
// and the ledger columns that reference a key (usage_logs.api_key,
// billing_settlements.api_key, request_traces.api_key) hold the same digest. That
// keeps every existing join intact - settlement still runs
// "UPDATE api_keys ... WHERE key = ?", reconciliation still groups by
// usage_logs.api_key, and the log console still filters by
// "api_key IN (SELECT key FROM api_keys WHERE id = ?)" - while no readable
// credential exists at rest.
//
// The credential has 128 bits of system entropy, so a plain SHA-256 digest cannot
// be brute-forced and needs no salt (a per-row salt would make the deterministic
// ledger join impossible). Authentication hashes the presented credential and
// compares digests.
func HashAPIKey(secret string) string {
	if secret == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(secret))
	return hex.EncodeToString(sum[:])
}

// IsAPIKeyHash reports whether value has the shape of a stored key digest (64
// lowercase hex characters). The migration uses it to stay idempotent, and the
// ingestion path uses it to refuse legacy cleartext credentials.
func IsAPIKeyHash(value string) bool {
	if len(value) != 64 {
		return false
	}
	for i := 0; i < len(value); i++ {
		c := value[i]
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') {
			return false
		}
	}
	return true
}

const (
	// apiKeyPublicIDPrefix marks a key's public identifier so a value copied out
	// of the console cannot be mistaken for a bearer credential.
	apiKeyPublicIDPrefix = "ak_"
	// apiKeyPublicIDBytes is 80 bits of entropy. Collisions stay negligible at
	// any realistic key count; the partial unique index from migration 39 is the
	// backstop, not the primary defence.
	apiKeyPublicIDBytes = 10
)

// NewAPIKeyPublicID mints the public identifier shown for a key in the console,
// logs, exports and support tickets.
//
// It is deliberately independent of the credential: it is not derived from the
// secret or from its digest, so it reveals nothing about what authenticates a
// call, does not change when the stored digest is replaced, and cannot be
// mistaken for a credential (it is not a digest and never matches a stored key).
func NewAPIKeyPublicID() (string, error) {
	b := make([]byte, apiKeyPublicIDBytes)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return apiKeyPublicIDPrefix + hex.EncodeToString(b), nil
}

// MaskAPIKey renders a credential as a short, non-reusable prefix so audit rows
// can tell unrecognised submissions apart without storing the value.
//
// Console key listings use NewAPIKeyPublicID instead: they never show a fragment
// of a credential, because the fragment is what operators copy by mistake.
func MaskAPIKey(key string) string {
	if key == "" {
		return ""
	}
	if len(key) <= 12 {
		return "sk-***"
	}
	return key[:8] + "***" + key[len(key)-4:]
}

// IsAPIKeyPublicID reports whether value is a key's public identifier (ak_…)
// rather than the digest the audit columns persist.
func IsAPIKeyPublicID(value string) bool {
	return strings.HasPrefix(value, apiKeyPublicIDPrefix)
}
