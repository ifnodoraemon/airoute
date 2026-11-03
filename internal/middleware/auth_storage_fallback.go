package middleware

import (
	"context"
	"database/sql"
	"errors"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/ifnodoraemon/airoute/internal/storage"
)

// Authentication has two sources of truth. The in-memory snapshot is the fast
// path every request uses; durable storage is authoritative. They differ for a
// short window in two situations that operators hit in practice:
//
//   - Freshly created keys. A key minted on one replica is live there at once but
//     reaches its peers only when their Redis reload broadcast is processed (or,
//     with Redis unavailable, the next periodic reload — 10s by default). During
//     that window the peer answered "Incorrect or unauthorized API key" for a
//     credential the console had just handed out.
//   - Revoked/disabled keys. The snapshot only carries active keys, so a key
//     deleted since the last reload fell into the same generic 401 even though it
//     is a real credential whose real answer is "revoked or disabled".
//
// A snapshot miss therefore asks storage once, by digest (api_keys.key is unique,
// so this is an index probe), bounded by authStorageTimeout like the refresh the
// hot path already performs for every authenticated request. A hit also lets the
// middleware deny a revoked key with the accurate reason instead of the generic
// invalid-key message.

const (
	// Console-issued keys are 43 characters, and an administrator cannot create a
	// key shorter than 16. Anything outside this range cannot be a credential this
	// gateway issued, so it is rejected without a storage query — junk traffic does
	// not get to spend a query per request.
	minCredentialLength = 16
	maxCredentialLength = 256

	// A caller repeating the same unknown credential must not turn the auth path
	// into a database loader, so definitive misses are remembered briefly. The
	// cache is an optimisation only: eviction or expiry just costs one more probe.
	rejectedCredentialCapacity = 1024
	rejectedCredentialTTL      = 30 * time.Second
)

// lookupStorageCredential resolves a credential the runtime snapshot does not
// know. It returns nil when storage cannot confirm the credential, which the
// caller reports as an invalid API key.
//
// The lookup runs under authStorageTimeout; a stalled database must not pin the
// request goroutine, exactly like the authoritative refresh on the hot path.
func lookupStorageCredential(c *gin.Context, rawKey, presentedHash string) *storage.APIKeyRecord {
	repo := storage.GetGlobalRepository()
	if repo == nil || presentedHash == "" || !credentialCouldExist(rawKey) {
		return nil
	}
	if rejectedCredentials.recent(presentedHash) {
		return nil
	}

	lookupCtx, cancel := context.WithTimeout(c.Request.Context(), authStorageTimeout)
	defer cancel()
	rec, err := repo.GetAPIKeyByKeyContext(lookupCtx, presentedHash)
	if err == nil && rec != nil {
		return rec
	}

	// Only a definitive "no such row" is cached. A timeout or connection error is
	// transient: caching it would keep rejecting a credential storage was never
	// actually able to rule out.
	if err == nil || errors.Is(err, sql.ErrNoRows) {
		rejectedCredentials.remember(presentedHash)
	}
	return nil
}

// credentialCouldExist reports whether the presented value has the shape of a
// credential this gateway could have issued. Legacy keys seeded through the
// configuration file are already in the snapshot, so they never depend on this
// check being permissive.
func credentialCouldExist(raw string) bool {
	if len(raw) < minCredentialLength || len(raw) > maxCredentialLength {
		return false
	}
	for i := 0; i < len(raw); i++ {
		switch raw[i] {
		case ' ', '\t', '\r', '\n':
			return false
		}
	}
	return true
}

// rejectedCredentialCache holds digests that storage has definitively reported
// as unknown, so repeated attempts with the same credential cost no query.
type rejectedCredentialCache struct {
	mu      sync.Mutex
	entries map[string]time.Time
	// now is injectable so expiry is testable without sleeping.
	now func() time.Time
}

func newRejectedCredentialCache() *rejectedCredentialCache {
	return &rejectedCredentialCache{
		entries: make(map[string]time.Time),
		now:     time.Now,
	}
}

// rejectedCredentials is process-wide: authentication is a global concern, and a
// per-request cache would be useless.
var rejectedCredentials = newRejectedCredentialCache()

// recent reports whether digest was rejected within the TTL.
func (rc *rejectedCredentialCache) recent(digest string) bool {
	if rc == nil || digest == "" {
		return false
	}
	rc.mu.Lock()
	defer rc.mu.Unlock()
	expiresAt, ok := rc.entries[digest]
	if !ok {
		return false
	}
	if rc.now().After(expiresAt) {
		delete(rc.entries, digest)
		return false
	}
	return true
}

// remember records a definitive rejection for the TTL. The map is bounded: when
// it is full, expired entries are dropped and, failing that, the whole cache is
// reset. Forgetting a rejection is harmless (one extra probe), so simplicity
// beats a precise eviction policy here.
func (rc *rejectedCredentialCache) remember(digest string) {
	if rc == nil || digest == "" {
		return
	}
	now := rc.now()
	rc.mu.Lock()
	defer rc.mu.Unlock()
	if len(rc.entries) >= rejectedCredentialCapacity {
		for key, expiresAt := range rc.entries {
			if now.After(expiresAt) {
				delete(rc.entries, key)
			}
		}
		if len(rc.entries) >= rejectedCredentialCapacity {
			rc.entries = make(map[string]time.Time, rejectedCredentialCapacity)
		}
	}
	rc.entries[digest] = now.Add(rejectedCredentialTTL)
}
