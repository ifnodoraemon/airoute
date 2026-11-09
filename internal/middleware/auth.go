package middleware

import (
	"time"

	"github.com/gin-gonic/gin"
	"github.com/ifnodoraemon/airoute/internal/config"
	"github.com/ifnodoraemon/airoute/internal/model"
	"github.com/ifnodoraemon/airoute/internal/storage"
)

// authStorageTimeout bounds the per-request authoritative key/user lookups.
// The in-memory snapshot already carries the quota data; the database read is a
// consistency check, not something worth blocking a request (or the shared
// SQLite connection) for.
//
// Deliberately uncached: budget/quota changes written outside this process must
// be observed by the very next request, which a TTL cache would mask.
const authStorageTimeout = 2 * time.Second

const (
	ContextKeyTenant       = "tenant_id"
	ContextKeyAPIKey       = "api_key"
	ContextKeyAPIKeyConfig = "api_key_config"
	// ContextKeyAPIKeyID carries the key's public identifier (ak_…). Access logs
	// and audit lines name a key with it; the digest is never displayed.
	ContextKeyAPIKeyID = "api_key_id"
)

// AuthMiddleware authenticates incoming requests via Bearer API keys.
func AuthMiddleware() gin.HandlerFunc {
	return authMiddleware(nil)
}

// SessionKeyAuthMiddleware shares the data-plane admission/billing context with
// bearer clients. The resolver must authenticate the console session and verify
// access to the selected key; no client-supplied identifier is a credential.
// An absent selection always fails closed, even when the gateway is in open mode.
func SessionKeyAuthMiddleware(resolve func(*gin.Context) (*model.APIKeyConfig, *AccessDenial)) gin.HandlerFunc {
	if resolve == nil {
		resolve = func(*gin.Context) (*model.APIKeyConfig, *AccessDenial) { return nil, nil }
	}
	return authMiddleware(resolve)
}

func authMiddleware(resolve func(*gin.Context) (*model.APIKeyConfig, *AccessDenial)) gin.HandlerFunc {
	return func(c *gin.Context) {
		cfg := config.GetGlobalConfig()
		if !admitDataPlaneReady(c) {
			return
		}
		effectiveKey, resolvedFromStorage, ok := resolveAuthKey(c, cfg, resolve)
		if !ok {
			return
		}
		if effectiveKey == nil {
			c.Next()
			return
		}
		// Job-status reads skip spending gates, but still enforce key/account status.
		freeRead := isUnbilledResourceRequest(c)
		if !admitSnapshotKey(c, effectiveKey, freeRead) {
			return
		}
		repo := storage.GetGlobalRepository()
		if !refreshAuthKey(c, effectiveKey, repo, resolvedFromStorage, freeRead) {
			return
		}
		account := resolveAuthAccount(c, effectiveKey, repo)
		if !admitAuthAccount(c, effectiveKey.Key, account, freeRead) {
			return
		}
		release, ok := reserveAuthQuota(c, effectiveKey.Key, account, freeRead)
		if !ok {
			return
		}
		if release != nil {
			defer release()
		}
		c.Set(ContextKeyTenant, effectiveKey.TenantID)
		// Audit/ledger joins use the digest; logs and the console use the public id.
		c.Set(ContextKeyAPIKey, effectiveKey.Key)
		c.Set(ContextKeyAPIKeyID, effectiveKey.PublicID)
		c.Set(ContextKeyAPIKeyConfig, effectiveKey)
		c.Next()
	}
}

// ValidateModelAllowed checks if the requested model is permitted for this API key.
func ValidateModelAllowed(c *gin.Context, requestedModel string) bool {
	keyAny, exists := c.Get(ContextKeyAPIKeyConfig)
	if !exists {
		return true // open mode
	}

	keyCfg, ok := keyAny.(*model.APIKeyConfig)
	if !ok {
		return true // all models allowed
	}
	return ModelAllowed(keyCfg.AllowedModels, requestedModel)
}

// budgetExceeded compares spend against a budget using integer nano-units when
// the ledger mirror is populated, falling back to the float columns for legacy
// rows.
//
// The previous condition required BOTH sides to be positive, so a key configured
// with only budget_nano (float budget = 0) or a key that had not spent anything
// yet bypassed the integer check entirely and could be unbounded. The integer
// mirror is now authoritative whenever a budget is set in it.
func budgetExceeded(budgetUnits, usedUnits int64, budget, used float64) bool {
	if budgetUnits > 0 {
		return usedUnits >= budgetUnits
	}
	return budget > 0 && used >= budget
}
