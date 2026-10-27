package middleware

import (
	"context"
	"net/http"
	"strings"
	"sync/atomic"
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
	ContextKeyUserRecord   = "user_record"
	// ContextKeyAPIKeyFingerprint carries the non-reversible display identifier
	// for the authenticated credential (never the credential or its digest).
	ContextKeyAPIKeyFingerprint = "api_key_fingerprint"
)

// auditAuthFailure logs client-side authentication rejection into the audit table so 401s/402s are visible to operators.
func auditAuthFailure(c *gin.Context, rawKey string, statusCode int, errMsg string) {
	if storage.GlobalAsyncLogger == nil {
		return
	}
	// Store the non-reversible fingerprint, never a fragment of the credential:
	// api_keys.key / usage_logs.api_key hold digests now, and a 6+2 character
	// prefix/suffix would be the last cleartext remnant in the audit trail.
	maskedKey := ""
	if rawKey != "" {
		maskedKey = storage.APIKeyFingerprint(rawKey)
	}
	traceID := c.GetString("trace_id")
	if traceID == "" {
		traceID = c.GetHeader("x-request-id")
	}
	storage.GlobalAsyncLogger.Record(&storage.UsageLogRecord{
		TraceID:      traceID,
		APIKey:       maskedKey,
		StatusCode:   statusCode,
		ErrorMessage: errMsg,
		UsageSource:  "gateway_auth",
	})
}

// dataPlaneReady distinguishes "the gateway genuinely has no API keys" from
// "the runtime configuration has not been loaded yet".
//
// It defaults to true so embedders and tests that never wire the startup path
// keep working; cmd/airoute sets it to false before the first snapshot load and
// flips it back once storage answered. Without that distinction a failed first
// snapshot read leaves the in-memory key set empty, and the open-dev-mode
// branch below would admit every caller anonymously.
var dataPlaneReady atomic.Bool

func init() { dataPlaneReady.Store(true) }

// SetDataPlaneReady marks whether the runtime configuration has been loaded
// from durable storage at least once. It is called by the server bootstrap.
func SetDataPlaneReady(ready bool) { dataPlaneReady.Store(ready) }

// DataPlaneReady reports whether the data plane has a loaded configuration.
func DataPlaneReady() bool { return dataPlaneReady.Load() }

// DataPlaneUnreadyDenial is the fail-closed response served while the gateway
// has not loaded its configuration yet.
func DataPlaneUnreadyDenial() *AccessDenial {
	return &AccessDenial{
		StatusCode: http.StatusServiceUnavailable,
		Message:    "Gateway data plane is not ready: configuration has not been loaded from storage yet. Please retry shortly.",
		Type:       "service_unavailable",
		Code:       "data_plane_unready",
		Headers:    map[string]string{"Retry-After": "5"},
		LogMessage: "data plane configuration not loaded yet",
	}
}

// AuthMiddleware authenticates incoming requests via Bearer API keys.
func AuthMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		cfg := config.GetGlobalConfig()

		// Fail closed until the first successful configuration load. The open-mode
		// check below must never be reached with a snapshot that was never read.
		if !DataPlaneReady() {
			denial := DataPlaneUnreadyDenial()
			auditAuthFailure(c, "", denial.StatusCode, denial.LogMessage)
			for header, value := range denial.Headers {
				c.Header(header, value)
			}
			AbortWithDenial(c, denial)
			return
		}

		// If no API keys configured in system and no keys ever created, allow all requests in open dev mode.
		// If keys have been registered in the gateway (even if currently disabled/revoked), strictly enforce auth!
		// Both fields are rewritten by the periodic hot reload, so they must be
		// read through the locked accessors (see config.ActiveAPIKeyCount).
		if cfg.ActiveAPIKeyCount() == 0 && !cfg.HasRegisteredAPIKeys() {
			c.Next()
			return
		}

		var rawKey string

		authHeader := c.GetHeader("Authorization")
		if authHeader != "" {
			parts := strings.SplitN(authHeader, " ", 2)
			if len(parts) == 2 && strings.EqualFold(parts[0], "Bearer") {
				rawKey = strings.TrimSpace(parts[1])
			}
		}

		// Also support Anthropic SDK's x-api-key header
		if rawKey == "" {
			rawKey = strings.TrimSpace(c.GetHeader("x-api-key"))
		}

		// Also support Google Gemini SDK's query param 'key' and header 'x-goog-api-key'
		if rawKey == "" {
			rawKey = strings.TrimSpace(c.Query("key"))
		}
		if rawKey == "" {
			rawKey = strings.TrimSpace(c.GetHeader("x-goog-api-key"))
		}

		if rawKey == "" {
			auditAuthFailure(c, "", http.StatusUnauthorized, "Missing authentication key")
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"error": gin.H{
					"message": "Missing authentication key. Please provide 'Authorization: Bearer <key>', 'x-api-key: <key>', 'x-goog-api-key: <key>', or query parameter '?key=<key>'",
					"type":    "invalid_request_error",
					"code":    "missing_api_key",
				},
			})
			return
		}
		// The snapshot is keyed by the stored digest (api_keys.key holds
		// HashAPIKey(credential)), so the presented credential is hashed before
		// the lookup. Hashing is deterministic, which keeps every existing
		// credential authenticating after the migration.
		keyHash := storage.HashAPIKey(rawKey)
		matchedKey := lookupSnapshotKey(cfg, rawKey, keyHash)

		if matchedKey == nil {
			auditAuthFailure(c, rawKey, http.StatusUnauthorized, "Incorrect or unauthorized API key provided")
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"error": gin.H{
					"message": "Incorrect or unauthorized API key provided.",
					"type":    "authentication_error",
					"code":    "invalid_api_key",
				},
			})
			return
		}

		effectiveKey := *matchedKey

		// 1. Key status check (pure in-memory)
		if denial := KeyStatusDenial(effectiveKey.Status); denial != nil {
			auditAuthFailure(c, rawKey, denial.StatusCode, strings.TrimSuffix(denial.Message, "."))
			AbortWithDenial(c, denial)
			return
		}

		// 2. Key budget check (fast in-memory check first). Use the integer
		// 1e-9-unit mirror when available so repeated float accumulation cannot
		// drift across the budget boundary.
		if denial := KeyBudgetDenial(effectiveKey.BudgetUnits, effectiveKey.UsedCostUnits, effectiveKey.Budget, effectiveKey.UsedCost); denial != nil {
			auditAuthFailure(c, rawKey, denial.StatusCode, strings.TrimSuffix(denial.Message, "."))
			AbortWithDenial(c, denial)
			return
		}

		// 3. Authoritative storage key check if database repository is available.
		// Bounded by a short deadline: this runs on every data-plane request, so a
		// stalled database must fail the request rather than pin the goroutine
		// (and, on SQLite's single connection, the whole gateway).
		repo := storage.GetGlobalRepository()
		if repo != nil {
			lookupCtx, cancel := context.WithTimeout(c.Request.Context(), authStorageTimeout)
			keyRec, _ := repo.GetAPIKeyByKeyContext(lookupCtx, effectiveKey.Key)
			cancel()
			if keyRec != nil {
				if denial := KeyStatusDenial(keyRec.Status); denial != nil {
					auditAuthFailure(c, rawKey, denial.StatusCode, strings.TrimSuffix(denial.Message, "."))
					AbortWithDenial(c, denial)
					return
				}
				effectiveBudget := keyRec.Budget
				if effectiveBudget <= 0 {
					effectiveBudget = effectiveKey.Budget
				}
				budgetUnits := keyRec.BudgetUnits
				if budgetUnits <= 0 {
					budgetUnits = effectiveKey.BudgetUnits
				}
				if denial := KeyBudgetDenial(budgetUnits, keyRec.UsedCostUnits, effectiveBudget, keyRec.UsedCost); denial != nil {
					AbortWithDenial(c, denial)
					return
				}
				if keyRec.GroupName != "" {
					effectiveKey.GroupName = keyRec.GroupName
				}
				if keyRec.UserID > 0 {
					effectiveKey.UserID = keyRec.UserID
				}
				if keyRec.UserStatus != "" {
					effectiveKey.UserStatus = keyRec.UserStatus
					effectiveKey.UserRole = keyRec.UserRole
					effectiveKey.UserBalance = keyRec.UserBalance
				}
			}
		}

		// 4. User account status & wallet balance check
		userStatus := effectiveKey.UserStatus
		userRole := effectiveKey.UserRole
		userBalance := effectiveKey.UserBalance
		userID := effectiveKey.UserID

		// Fallback to storage only if memory snapshot was not populated
		if userStatus == "" && effectiveKey.UserID > 0 && repo != nil {
			lookupCtx, cancel := context.WithTimeout(c.Request.Context(), authStorageTimeout)
			u, _ := repo.GetUserByIDContext(lookupCtx, effectiveKey.UserID)
			cancel()
			if u != nil {
				userStatus = u.Status
				userRole = u.Role
				userBalance = u.Balance
			}
		}

		if denial := AccountDenial(userStatus, userRole, userBalance); denial != nil {
			AbortWithDenial(c, denial)
			return
		}

		// Pre-flight in-flight reservation to prevent concurrent overdraft storm
		if userStatus != "" && !strings.EqualFold(userRole, "admin") && userID > 0 {
			acquired, backend := ReserveUserQuota(userID, userBalance)
			if !acquired {
				AbortWithDenial(c, &AccessDenial{
					StatusCode: http.StatusPaymentRequired,
					Message:    "Concurrent requests exceed available wallet balance reserve. Please wait for pending requests to finish or recharge.",
					Type:       "insufficient_quota",
					Code:       "insufficient_quota",
				})
				return
			}
			defer ReleaseUserQuota(userID, backend)
		}

		// Save context info
		c.Set(ContextKeyTenant, effectiveKey.TenantID)
		// The digest is what audit rows and ledger joins use; the fingerprint is
		// what logs and the console display.
		c.Set(ContextKeyAPIKey, effectiveKey.Key)
		c.Set(ContextKeyAPIKeyFingerprint, storage.APIKeyFingerprint(rawKey))
		c.Set(ContextKeyAPIKeyConfig, &effectiveKey)
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
