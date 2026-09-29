package middleware

import (
	"context"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/ifnodoraemon/airoute/internal/config"
	"github.com/ifnodoraemon/airoute/internal/distributed"
	"github.com/ifnodoraemon/airoute/internal/model"
	"github.com/ifnodoraemon/airoute/internal/storage"
)

const (
	ContextKeyTenant       = "tenant_id"
	ContextKeyAPIKey       = "api_key"
	ContextKeyAPIKeyConfig = "api_key_config"
	ContextKeyUserRecord   = "user_record"
)

// UserQuotaTracker manages in-flight request reserves to prevent concurrent overdraft attacks.
type UserQuotaTracker struct {
	mu             sync.Mutex
	inFlightUsers  map[int64]int
	minReserveCost float64
}

var globalQuotaTracker = &UserQuotaTracker{
	inFlightUsers:  make(map[int64]int),
	minReserveCost: 0.0005, // minimal reserved credit per in-flight request
}

func (t *UserQuotaTracker) TryAcquire(userID int64, balance float64) bool {
	if userID <= 0 {
		return true
	}

	// Cluster distributed check via Redis if active
	if rClient := distributed.GetClient(); rClient != nil && rClient.IsActive() {
		ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
		defer cancel()
		acquired, err := rClient.TryAcquireInFlightQuota(ctx, userID, balance, t.minReserveCost)
		if err == nil {
			return acquired
		}
		// On Redis failure, fall back to local memory below
	}

	t.mu.Lock()
	defer t.mu.Unlock()
	inFlight := t.inFlightUsers[userID]
	if balance-float64(inFlight+1)*t.minReserveCost < 0 {
		return false
	}
	t.inFlightUsers[userID] = inFlight + 1
	return true
}

func (t *UserQuotaTracker) Release(userID int64) {
	if userID <= 0 {
		return
	}

	// Cluster distributed release via Redis if active
	if rClient := distributed.GetClient(); rClient != nil && rClient.IsActive() {
		ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
		defer cancel()
		if err := rClient.ReleaseInFlightQuota(ctx, userID); err == nil {
			return
		}
	}

	t.mu.Lock()
	defer t.mu.Unlock()
	if t.inFlightUsers[userID] > 1 {
		t.inFlightUsers[userID]--
	} else {
		delete(t.inFlightUsers, userID)
	}
}

// AuthMiddleware authenticates incoming requests via Bearer API keys.
func AuthMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		cfg := config.GetGlobalConfig()

		// If no API keys configured in system and no keys ever created, allow all requests in open dev mode.
		// If keys have been registered in the gateway (even if currently disabled/revoked), strictly enforce auth!
		if len(cfg.APIKeys) == 0 && !cfg.HasConfiguredKeys {
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
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"error": gin.H{
					"message": "Missing authentication key. Please provide 'Authorization: Bearer <key>', 'x-api-key: <key>', 'x-goog-api-key: <key>', or query parameter '?key=<key>'",
					"type":    "invalid_request_error",
					"code":    "missing_api_key",
				},
			})
			return
		}
		matchedKey := cfg.GetAPIKey(rawKey)

		if matchedKey == nil {
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

		// Check associated user account status and wallet quota balance
		repo := storage.GetGlobalRepository()
		if repo != nil {
			var user *storage.UserRecord
			if effectiveKey.UserID > 0 {
				user, _ = repo.GetUserByID(effectiveKey.UserID)
			}
			if user == nil && effectiveKey.TenantID != "" {
				user, _ = repo.GetUserByUsername(effectiveKey.TenantID)
			}

			// Key budget and status check
			keyRec, _ := repo.GetAPIKeyByKey(effectiveKey.Key)
			if keyRec != nil {
				if keyRec.Status != "" && keyRec.Status != "active" {
					c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
						"error": gin.H{
							"message": "The API key has been revoked or disabled.",
							"type":    "invalid_request_error",
							"code":    "api_key_revoked",
						},
					})
					return
				}
				effectiveBudget := keyRec.Budget
				if effectiveBudget <= 0 {
					effectiveBudget = effectiveKey.Budget
				}
				if effectiveBudget > 0 && keyRec.UsedCost >= effectiveBudget {
					c.AbortWithStatusJSON(http.StatusPaymentRequired, gin.H{
						"error": gin.H{
							"message": "This API key has exceeded its assigned quota budget limit.",
							"type":    "key_budget_exceeded",
							"code":    "key_budget_exceeded",
						},
					})
					return
				}
				if keyRec.GroupName != "" && (effectiveKey.GroupName == "" || effectiveKey.GroupName == "default") {
					effectiveKey.GroupName = keyRec.GroupName
				}
			}

			if user != nil {
				// 1. Account lock check
				if strings.EqualFold(user.Status, "locked") {
					c.AbortWithStatusJSON(http.StatusForbidden, gin.H{
						"error": gin.H{
							"message": "User account is locked. Please contact administrator.",
							"type":    "account_locked",
							"code":    "account_locked",
						},
					})
					return
				}

				// 2. Wallet balance check (admin accounts have unlimited quota)
				if !strings.EqualFold(user.Role, "admin") {
					if user.Balance <= 0 {
						c.AbortWithStatusJSON(http.StatusPaymentRequired, gin.H{
							"error": gin.H{
								"message": "You have exceeded your current balance/quota (insufficient_quota). Please recharge your wallet at the console.",
								"type":    "insufficient_quota",
								"code":    "insufficient_quota",
							},
						})
						return
					}
					// Pre-flight in-flight reservation to prevent concurrent overdraft storm
					if !globalQuotaTracker.TryAcquire(user.ID, user.Balance) {
						c.AbortWithStatusJSON(http.StatusPaymentRequired, gin.H{
							"error": gin.H{
								"message": "Concurrent requests exceed available wallet balance reserve. Please wait for pending requests to finish or recharge.",
								"type":    "insufficient_quota",
								"code":    "insufficient_quota",
							},
						})
						return
					}
					defer globalQuotaTracker.Release(user.ID)
				}

				// Inherit user's group if key does not have an explicit group
				if (effectiveKey.GroupName == "" || effectiveKey.GroupName == "default") && user.GroupName != "" {
					effectiveKey.GroupName = user.GroupName
				}
				c.Set(ContextKeyUserRecord, user)
			}
		}

		// Save context info
		c.Set(ContextKeyTenant, effectiveKey.TenantID)
		c.Set(ContextKeyAPIKey, effectiveKey.Key)
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
	if !ok || len(keyCfg.AllowedModels) == 0 {
		return true // all models allowed
	}

	for _, m := range keyCfg.AllowedModels {
		if m == "*" || m == requestedModel {
			return true
		}
		if strings.HasSuffix(m, "/*") {
			prefix := strings.TrimSuffix(m, "/*") + "/"
			if strings.HasPrefix(requestedModel, prefix) {
				return true
			}
		}
	}
	return false
}
