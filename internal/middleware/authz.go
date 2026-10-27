package middleware

import (
	"context"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/ifnodoraemon/airoute/internal/distributed"
	"github.com/ifnodoraemon/airoute/internal/model"
)

// AccessDenial describes a data-plane admission failure.
//
// The same rules now gate HTTP /v1/* requests (AuthMiddleware / RateLimitMiddleware)
// and the MCP JSON-RPC transport, which used to authenticate the caller and then
// skip every one of them. Keeping the decision in these helpers is what stops
// the two entrypoints from drifting back into subtly different policies.
type AccessDenial struct {
	StatusCode int
	Message    string
	Type       string
	Code       string
	// Headers are optional response headers (X-RateLimit-*, Retry-After) that
	// belong with the rejection.
	Headers map[string]string
	// LogMessage is the operator-facing access-log reason. It is deliberately
	// shorter than Message (see RateLimitMiddleware).
	LogMessage string
}

// AbortWithDenial writes the denial as the gateway's standard OpenAI-shaped
// error body and aborts the request.
func AbortWithDenial(c *gin.Context, denial *AccessDenial) {
	if denial == nil {
		return
	}
	c.AbortWithStatusJSON(denial.StatusCode, gin.H{
		"error": gin.H{
			"message": denial.Message,
			"type":    denial.Type,
			"code":    denial.Code,
		},
	})
}

// KeyStatusDenial reports an API key that is not active. An empty status means
// "unknown", which legacy rows use; only an explicit non-active status is a
// rejection (matching the historical middleware behaviour).
func KeyStatusDenial(status string) *AccessDenial {
	if status == "" || status == "active" {
		return nil
	}
	return &AccessDenial{
		StatusCode: http.StatusUnauthorized,
		Message:    "The API key has been revoked or disabled.",
		Type:       "invalid_request_error",
		Code:       "api_key_revoked",
	}
}

// KeyBudgetDenial reports a key whose assigned quota budget is exhausted.
func KeyBudgetDenial(budgetUnits, usedUnits int64, budget, used float64) *AccessDenial {
	if !budgetExceeded(budgetUnits, usedUnits, budget, used) {
		return nil
	}
	return &AccessDenial{
		StatusCode: http.StatusPaymentRequired,
		Message:    "This API key has exceeded its assigned quota budget limit.",
		Type:       "key_budget_exceeded",
		Code:       "key_budget_exceeded",
	}
}

// AccountDenial reports a locked/deleted owner or an exhausted wallet. An empty
// user status means the memory snapshot was not populated; the middleware then
// falls back to storage, and open/dev-mode keys have no owner at all.
func AccountDenial(userStatus, userRole string, userBalance float64) *AccessDenial {
	if userStatus == "" {
		return nil
	}
	if strings.EqualFold(userStatus, "locked") || strings.EqualFold(userStatus, "deleted") {
		return &AccessDenial{
			StatusCode: http.StatusForbidden,
			Message:    "User account is locked. Please contact administrator.",
			Type:       "account_locked",
			Code:       "account_locked",
		}
	}
	// Admin accounts have unlimited quota.
	if strings.EqualFold(userRole, "admin") {
		return nil
	}
	if userBalance <= 0 {
		return &AccessDenial{
			StatusCode: http.StatusPaymentRequired,
			Message:    "You have exceeded your current balance/quota (insufficient_quota). Please recharge your wallet at the console.",
			Type:       "insufficient_quota",
			Code:       "insufficient_quota",
		}
	}
	return nil
}

// AuthorizeKey applies the key/account admission rules shared by every
// data-plane entrypoint. Callers that hold an authoritative storage record use
// this directly; the HTTP middleware applies the same helpers between its
// in-memory fast path and its storage overlay so its responses stay identical.
func AuthorizeKey(key *model.APIKeyConfig) *AccessDenial {
	if key == nil {
		return nil
	}
	if denial := KeyStatusDenial(key.Status); denial != nil {
		return denial
	}
	if denial := KeyBudgetDenial(key.BudgetUnits, key.UsedCostUnits, key.Budget, key.UsedCost); denial != nil {
		return denial
	}
	return AccountDenial(key.UserStatus, key.UserRole, key.UserBalance)
}

// ModelAllowed reports whether an API key's AllowedModels permits the requested
// model. An empty allowlist means "all models"; "*" and "prefix/*" wildcards are
// honoured exactly as the HTTP data plane always has.
func ModelAllowed(allowedModels []string, requestedModel string) bool {
	if len(allowedModels) == 0 {
		return true
	}
	for _, m := range allowedModels {
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

// KeyRateLimitOutcome is the result of the per-key TPM/RPM gate.
type KeyRateLimitOutcome struct {
	// Denial is nil when the request may proceed.
	Denial *AccessDenial
	// RemainingTPM is -1 when the key has no TPM limit configured.
	RemainingTPM int
}

// CheckKeyRateLimits applies the per-key TPM and RPM limits. HTTP requests reach
// it through RateLimitMiddleware; the MCP transport calls it directly, because
// /mcp/* was never mounted behind the rate-limit middleware.
func CheckKeyRateLimits(ctx context.Context, key *model.APIKeyConfig) KeyRateLimitOutcome {
	outcome := KeyRateLimitOutcome{RemainingTPM: -1}
	if key == nil {
		return outcome
	}

	// Token-per-minute gate: reject before any upstream call when the key has
	// already consumed its minute budget. RPM limiting below stays independent.
	if key.TPM > 0 {
		if !GlobalTokenQuota.Allow(key.Key, key.TPM) {
			outcome.Denial = &AccessDenial{
				StatusCode: http.StatusTooManyRequests,
				Message:    "Token rate limit exceeded (TPM limit reached). Please slow down requests.",
				Type:       "rate_limit_error",
				Code:       "token_rate_limit_exceeded",
				Headers: map[string]string{
					"X-RateLimit-Limit-Tokens": strconv.Itoa(key.TPM),
					"Retry-After":              "60",
				},
				LogMessage: "token rate limit exceeded (TPM)",
			}
			return outcome
		}
		outcome.RemainingTPM = GlobalTokenQuota.Remaining(key.Key, key.TPM)
	}

	if key.RPM <= 0 {
		return outcome
	}

	// 1. Enterprise distributed Redis cluster rate limit
	client := distributed.GetClient()
	if client != nil && client.IsActive() {
		rpmCtx, cancel := context.WithTimeout(ctx, redisRateLimitTimeout)
		allowed, err := client.AllowRPM(rpmCtx, key.Key, key.RPM)
		cancel()
		if err == nil {
			if !allowed {
				outcome.Denial = &AccessDenial{
					StatusCode: http.StatusTooManyRequests,
					Message:    "Cluster rate limit exceeded (RPM limit reached). Please slow down requests.",
					Type:       "rate_limit_error",
					Code:       "rate_limit_exceeded",
					Headers: map[string]string{
						"X-RateLimit-Limit-Requests": strconv.Itoa(key.RPM),
						"Retry-After":                "1",
					},
					LogMessage: "cluster rate limit exceeded (RPM)",
				}
			}
			return outcome
		}
		// On Redis error/timeout, gracefully fall through to the local bucket.
	}

	// 2. Fallback to the local in-memory token bucket
	bucket := GlobalRateLimiter.GetBucket(key.Key, key.RPM)
	if !bucket.Allow() {
		outcome.Denial = &AccessDenial{
			StatusCode: http.StatusTooManyRequests,
			Message:    "Rate limit exceeded (RPM limit reached). Please slow down requests.",
			Type:       "rate_limit_error",
			Code:       "rate_limit_exceeded",
			Headers: map[string]string{
				"X-RateLimit-Limit-Requests": strconv.Itoa(key.RPM),
				"Retry-After":                "1",
			},
			LogMessage: "local rate limit exceeded (RPM)",
		}
	}
	return outcome
}

// ReserveUserQuota reserves one in-flight request slot for a non-admin user.
func ReserveUserQuota(userID int64, balance float64) (bool, QuotaBackend) {
	return globalQuotaTracker.TryAcquire(userID, balance)
}

// ReleaseUserQuota frees a reservation acquired with ReserveUserQuota.
func ReleaseUserQuota(userID int64, backend QuotaBackend) {
	globalQuotaTracker.Release(userID, backend)
}
