package middleware

import (
	"net"
	"net/http"
	"strings"
	"sync/atomic"

	"github.com/gin-gonic/gin"
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
	for name, value := range denial.Headers {
		c.Header(name, value)
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
//
// This cheap precheck uses settled usage. ReserveBilling performs the
// authoritative check under database locks, including other requests' holds.
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

// AccountLockedDenial reports a locked/deleted owner account. It is split out
// from AccountDenial so read-only endpoints that are exempt from the wallet
// balance gate (video task polling) can still refuse a locked/deleted account.
func AccountLockedDenial(userStatus string) *AccessDenial {
	if strings.EqualFold(userStatus, "locked") || strings.EqualFold(userStatus, "deleted") {
		return &AccessDenial{
			StatusCode: http.StatusForbidden,
			Message:    "User account is locked. Please contact administrator.",
			Type:       "account_locked",
			Code:       "account_locked",
		}
	}
	return nil
}

// AccountDenial reports a locked/deleted owner or an exhausted wallet. An empty
// user status means the memory snapshot was not populated; the middleware then
// falls back to storage, and open/dev-mode keys have no owner at all.
func AccountDenial(userStatus, userRole string, userBalance float64) *AccessDenial {
	if userStatus == "" {
		return nil
	}
	if denial := AccountLockedDenial(userStatus); denial != nil {
		return denial
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

// BalanceUnknownDenial is the fail-closed response served when the gateway knows
// a key belongs to a user but cannot establish that user's status or balance:
// the in-memory snapshot was built without user fields and the authoritative
// lookup timed out.
//
// Failing open here (the old behaviour) meant a database stall silently disabled
// the wallet gate *and* the pre-authorisation for those requests: exactly the
// burst the amount reservation exists to stop. A short 503 with Retry-After is
// the safe direction: the client retries when storage answers again.
func BalanceUnknownDenial() *AccessDenial {
	return &AccessDenial{
		StatusCode: http.StatusServiceUnavailable,
		Message:    "Account state is temporarily unavailable; the request was not admitted because the wallet balance could not be verified. Please retry shortly.",
		Type:       "service_unavailable",
		Code:       "account_state_unavailable",
		Headers:    map[string]string{"Retry-After": "2"},
		LogMessage: "account status/balance could not be resolved; failing closed",
	}
}

// IsFreeReadVideoTaskQuery reports whether the request is exactly a GET for a
// video task status (/v1/videos/tasks/:id, and the root-mounted
// /videos/tasks/:id). Those reads are never billed — the task was already
// charged when it was created — so an exhausted wallet, per-key budget or
// in-flight reservation must not block a customer from polling a job the
// provider is still running. Authentication, key status and locked/deleted
// account checks still apply.
func IsFreeReadVideoTaskQuery(c *gin.Context) bool {
	if c == nil || c.Request == nil || c.Request.Method != http.MethodGet {
		return false
	}
	var rest string
	for _, prefix := range []string{"/v1/videos/tasks/", "/videos/tasks/"} {
		if strings.HasPrefix(c.Request.URL.Path, prefix) {
			rest = strings.TrimPrefix(c.Request.URL.Path, prefix)
			break
		}
	}
	if rest == "" {
		return false
	}
	// Exactly one path segment: a trailing slash is tolerated, a nested path is not.
	rest = strings.TrimSuffix(rest, "/")
	return rest != "" && !strings.Contains(rest, "/")
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

// ---------------------------------------------------------------------------
// Client identity resolution
//
// Gin's default is to trust every peer as a reverse proxy, so c.ClientIP()
// returns the first X-Forwarded-For entry — a value any client can choose. That
// made the login lockout, the per-IP rate limiters and the access-log address
// attacker-controlled: rotating the header created a fresh bucket per attempt.
// The resolver below only believes forwarding headers that arrive from a peer
// configured in GATEWAY_TRUSTED_PROXIES, and walks X-Forwarded-For from the
// right so a client-supplied prefix cannot shadow the real address.

const trustedProxyEnv = "GATEWAY_TRUSTED_PROXIES"

// ClientIdentity is the resolved origin of a request.
type ClientIdentity struct {
	// IP is the best available client address (or the TCP peer when no proxy
	// hop is trusted).
	IP string
	// Forwarded reports whether IP came from a trusted proxy's forwarding
	// header. It is false when the address is merely the TCP peer, which is the
	// proxy itself in an unconfigured deployment; callers that apply a per-IP
	// penalty should only do so when this is true.
	Forwarded bool
}

type trustedProxyCacheEntry struct {
	spec string
	nets []*net.IPNet
	err  error
}

var trustedProxyCache atomic.Pointer[trustedProxyCacheEntry]
