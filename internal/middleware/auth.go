package middleware

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/ifnodoraemon/nano-gateway/internal/config"
	"github.com/ifnodoraemon/nano-gateway/internal/model"
	"github.com/ifnodoraemon/nano-gateway/internal/storage"
)

const (
	ContextKeyTenant           = "tenant_id"
	ContextKeyVirtualKey       = "virtual_key"
	ContextKeyVirtualKeyConfig = "virtual_key_config"
	ContextKeyUserRecord       = "user_record"
)

// AuthMiddleware authenticates incoming requests via Bearer API keys.
func AuthMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		cfg := config.GetGlobalConfig()

		// If no virtual keys configured in system and no keys ever created, allow all requests in open dev mode.
		// If keys have been registered in the gateway (even if currently disabled/revoked), strictly enforce auth!
		if len(cfg.VirtualKeys) == 0 && !cfg.HasConfiguredKeys {
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

		if rawKey == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"error": gin.H{
					"message": "Missing authentication key. Please provide 'Authorization: Bearer <key>' or 'x-api-key: <key>'",
					"type":    "invalid_request_error",
					"code":    "missing_api_key",
				},
			})
			return
		}
		matchedKey := cfg.GetVirtualKey(rawKey)

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

		// Check associated user account status and wallet quota balance
		repo := storage.GetGlobalRepository()
		if repo != nil {
			var user *storage.UserRecord
			if matchedKey.UserID > 0 {
				user, _ = repo.GetUserByID(matchedKey.UserID)
			}
			if user == nil && matchedKey.TenantID != "" {
				user, _ = repo.GetUserByUsername(matchedKey.TenantID)
			}

			// Key budget and status check
			vkRec, _ := repo.GetVirtualKeyByKey(matchedKey.Key)
			if vkRec != nil {
				if vkRec.Status != "" && vkRec.Status != "active" {
					c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
						"error": gin.H{
							"message": "The API key has been revoked or disabled.",
							"type":    "invalid_request_error",
							"code":    "api_key_revoked",
						},
					})
					return
				}
				if matchedKey.Budget > 0 && vkRec.UsedCost >= matchedKey.Budget {
					c.AbortWithStatusJSON(http.StatusPaymentRequired, gin.H{
						"error": gin.H{
							"message": "This API key has exceeded its assigned quota budget limit.",
							"type":    "key_budget_exceeded",
							"code":    "key_budget_exceeded",
						},
					})
					return
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
				if !strings.EqualFold(user.Role, "admin") && user.Balance <= 0 {
					c.AbortWithStatusJSON(http.StatusPaymentRequired, gin.H{
						"error": gin.H{
							"message": "You have exceeded your current balance/quota (insufficient_quota). Please recharge your wallet at the console.",
							"type":    "insufficient_quota",
							"code":    "insufficient_quota",
						},
					})
					return
				}

				// Inherit user's group if key does not have an explicit group
				if (matchedKey.GroupName == "" || matchedKey.GroupName == "default") && user.GroupName != "" {
					matchedKey.GroupName = user.GroupName
				}
				c.Set(ContextKeyUserRecord, user)
			}
		}

		// Save context info
		c.Set(ContextKeyTenant, matchedKey.TenantID)
		c.Set(ContextKeyVirtualKey, matchedKey.Key)
		c.Set(ContextKeyVirtualKeyConfig, matchedKey)
		c.Next()
	}
}

// ValidateModelAllowed checks if the requested model is permitted for this virtual key.
func ValidateModelAllowed(c *gin.Context, requestedModel string) bool {
	vkAny, exists := c.Get(ContextKeyVirtualKeyConfig)
	if !exists {
		return true // open mode
	}

	vk, ok := vkAny.(*model.VirtualKeyConfig)
	if !ok || len(vk.AllowedModels) == 0 {
		return true // all models allowed
	}

	for _, m := range vk.AllowedModels {
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
