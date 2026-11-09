package middleware

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/ifnodoraemon/airoute/internal/config"
	"github.com/ifnodoraemon/airoute/internal/model"
	"github.com/ifnodoraemon/airoute/internal/storage"
)

// lookupSnapshotKey resolves a presented credential against the in-memory snapshot.
//
// The snapshot is keyed by the stored digest. A snapshot built before migration 29
// (or seeded directly by a test) still holds cleartext identifiers; in that case the
// raw value is tried as well, which is exactly the pre-migration behaviour.
//
// The raw fallback is deliberately limited to values that cannot themselves be a
// digest: otherwise the stored digest — which is not a credential — would become
// replayable as a bearer token.
func lookupSnapshotKey(cfg *config.Config, presented, presentedHash string) *model.APIKeyConfig {
	if cfg == nil {
		return nil
	}
	if matched := cfg.GetAPIKey(presentedHash); matched != nil {
		return matched
	}
	if presented == "" || storage.IsAPIKeyHash(presented) {
		return nil
	}
	legacy := cfg.GetAPIKey(presented)
	if legacy == nil {
		return nil
	}
	// Hand downstream code the digest rather than the legacy identifier: the
	// authoritative database check, audit rows and ledger joins all key off it.
	normalized := *legacy
	normalized.Key = presentedHash
	return &normalized
}

// resolveAuthKey returns a nil key only for the established anonymous dev mode.
func resolveAuthKey(c *gin.Context, cfg *config.Config, resolve func(*gin.Context) (*model.APIKeyConfig, *AccessDenial)) (*model.APIKeyConfig, bool, bool) {
	if resolve != nil {
		key, denial := resolve(c)
		if denial != nil {
			AbortWithDenial(c, denial)
			return nil, false, false
		}
		if key == nil || key.Key == "" {
			AbortWithDenial(c, &AccessDenial{StatusCode: http.StatusForbidden, Message: "Select an accessible API key.", Type: "permission_error", Code: "key_access_denied"})
			return nil, false, false
		}
		copy := *key
		return &copy, true, true
	}
	// Disabled/revoked keys still make this an authenticated gateway.
	if cfg.ActiveAPIKeyCount() == 0 && !cfg.HasRegisteredAPIKeys() {
		return nil, false, true
	}
	rawKey := requestAuthKey(c)
	if rawKey == "" {
		auditAuthFailure(c, "", http.StatusUnauthorized, "Missing authentication key")
		c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
			"error": gin.H{
				"message": "Missing authentication key. Please provide 'Authorization: Bearer <key>', 'x-api-key: <key>', 'x-goog-api-key: <key>', or query parameter '?key=<key>'",
				"type":    "invalid_request_error", "code": "missing_api_key",
			},
		})
		return nil, false, false
	}
	keyHash := storage.HashAPIKey(rawKey)
	if matched := lookupSnapshotKey(cfg, rawKey, keyHash); matched != nil {
		copy := *matched
		return &copy, false, true
	}
	// A snapshot miss can be a new key on another replica. Read durable state once.
	return resolveStoredAuthKey(c, rawKey, keyHash)
}

func requestAuthKey(c *gin.Context) string {
	var rawKey string
	if authHeader := c.GetHeader("Authorization"); authHeader != "" {
		parts := strings.SplitN(authHeader, " ", 2)
		if len(parts) == 2 && strings.EqualFold(parts[0], "Bearer") {
			rawKey = strings.TrimSpace(parts[1])
		}
	}
	if rawKey == "" {
		rawKey = strings.TrimSpace(c.GetHeader("x-api-key"))
	}
	if rawKey == "" {
		rawKey = strings.TrimSpace(c.Query("key"))
	}
	if rawKey == "" {
		rawKey = strings.TrimSpace(c.GetHeader("x-goog-api-key"))
	}
	return rawKey
}

func resolveStoredAuthKey(c *gin.Context, rawKey, keyHash string) (*model.APIKeyConfig, bool, bool) {
	keyRec := lookupStorageCredential(c, rawKey, keyHash)
	if keyRec == nil {
		auditAuthFailure(c, rawKey, http.StatusUnauthorized, "Incorrect or unauthorized API key provided")
		c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
			"error": gin.H{
				"message": "Incorrect or unauthorized API key provided.",
				"type":    "authentication_error", "code": "invalid_api_key",
			},
		})
		return nil, false, false
	}
	if denial := KeyStatusDenial(keyRec.Status); denial != nil {
		auditAuthFailure(c, rawKey, denial.StatusCode, strings.TrimSuffix(denial.Message, "."))
		AbortWithDenial(c, denial)
		return nil, false, false
	}
	key := keyRec.ToConfig()
	return &key, true, true
}

func abortResolvedKeyDenial(c *gin.Context, digest string, denial *AccessDenial) bool {
	if denial == nil {
		return true
	}
	auditResolvedKeyFailure(c, digest, denial.StatusCode, strings.TrimSuffix(denial.Message, "."))
	AbortWithDenial(c, denial)
	return false
}

func admitSnapshotKey(c *gin.Context, key *model.APIKeyConfig, freeRead bool) bool {
	if !abortResolvedKeyDenial(c, key.Key, KeyStatusDenial(key.Status)) {
		return false
	}
	if !freeRead {
		return abortResolvedKeyDenial(c, key.Key, KeyBudgetDenial(key.BudgetUnits, key.UsedCostUnits, key.Budget, key.UsedCost))
	}
	return true
}
