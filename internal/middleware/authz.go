package middleware

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync/atomic"

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
//
// The check runs before dispatch and compares the usage settled so far, so it is
// a soft ceiling rather than a reservation: concurrent requests that all pass
// this check can push the key past its budget, bounded by the number of in-flight
// requests times the per-request cost. Concurrency is resolved at settlement
// (atomic increments), which keeps used_cost_units exactly equal to the sum of
// the recorded usage rows — see the rate-limiting section of
// docs/CONFIGURATION.md.
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

	// 2. Fallback to the local in-memory sliding window. It enforces the same 60s
	// semantics as the Redis path above, so a Redis outage does not loosen the
	// limit (see rpmWindowSpan).
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

// ParseTrustedProxies parses a comma/space separated list of CIDRs or bare IP
// addresses. An empty specification means "trust no proxy".
func ParseTrustedProxies(spec string) ([]*net.IPNet, error) {
	spec = strings.TrimSpace(spec)
	if spec == "" {
		return nil, nil
	}
	fields := strings.FieldsFunc(spec, func(r rune) bool {
		return r == ',' || r == ' ' || r == '\t' || r == '\n' || r == '\r'
	})
	nets := make([]*net.IPNet, 0, len(fields))
	for _, field := range fields {
		field = strings.TrimSpace(field)
		if field == "" {
			continue
		}
		if _, network, err := net.ParseCIDR(field); err == nil {
			nets = append(nets, network)
			continue
		}
		ip := net.ParseIP(field)
		if ip == nil {
			return nil, fmt.Errorf("invalid trusted proxy address %q (want an IP or CIDR)", field)
		}
		if v4 := ip.To4(); v4 != nil {
			nets = append(nets, &net.IPNet{IP: v4, Mask: net.CIDRMask(32, 32)})
		} else {
			nets = append(nets, &net.IPNet{IP: ip, Mask: net.CIDRMask(128, 128)})
		}
	}
	return nets, nil
}

// TrustedProxiesFromEnv returns the configured trusted proxy networks, caching
// the parse per environment value so it can run on every request.
func TrustedProxiesFromEnv() ([]*net.IPNet, error) {
	spec := strings.TrimSpace(os.Getenv(trustedProxyEnv))
	if cached := trustedProxyCache.Load(); cached != nil && cached.spec == spec {
		return cached.nets, cached.err
	}
	nets, err := ParseTrustedProxies(spec)
	trustedProxyCache.Store(&trustedProxyCacheEntry{spec: spec, nets: nets, err: err})
	return nets, err
}

// TrustedProxyConfigured reports whether any reverse proxy is trusted. Without
// one, forwarding headers are ignored (there is no way to tell a real client
// address from a forged one).
func TrustedProxyConfigured() bool {
	nets, err := TrustedProxiesFromEnv()
	return err == nil && len(nets) > 0
}

// ConfigureTrustedProxies applies GATEWAY_TRUSTED_PROXIES to the Gin engine so
// every c.ClientIP() caller sees the same, unspoofable address as
// ResolveClientIdentity. An invalid list is an error rather than a silent
// fallback, because "trust everything" is exactly the failure being fixed.
func ConfigureTrustedProxies(engine *gin.Engine) error {
	if engine == nil {
		return nil
	}
	spec := strings.TrimSpace(os.Getenv(trustedProxyEnv))
	nets, err := ParseTrustedProxies(spec)
	if err != nil {
		return err
	}
	if len(nets) == 0 {
		// Setting an empty list (nil) makes Gin ignore forwarding headers and
		// report the TCP peer.
		return engine.SetTrustedProxies(nil)
	}
	proxies := make([]string, 0, len(nets))
	for _, network := range nets {
		proxies = append(proxies, network.String())
	}
	return engine.SetTrustedProxies(proxies)
}

// ResolveClientIdentity resolves the request origin, honouring forwarding
// headers only when the direct peer is a configured trusted proxy and the
// forwarded address itself is not another trusted proxy.
func ResolveClientIdentity(c *gin.Context) ClientIdentity {
	if c == nil || c.Request == nil {
		return ClientIdentity{}
	}
	peer := peerIP(c.Request.RemoteAddr)
	if peer == "" {
		return ClientIdentity{}
	}
	trusted, err := TrustedProxiesFromEnv()
	peerIPValue := net.ParseIP(peer)
	if err != nil || !ipInNetworks(peerIPValue, trusted) {
		// Untrusted (or unconfigured) peer: any forwarding header it sends is
		// self-asserted, so the peer is the only address we can attribute.
		return ClientIdentity{IP: peer}
	}

	// Walk X-Forwarded-For from the right: the last trusted proxy appends the
	// last untrusted hop, which is the closest thing to the real client.
	if forwarded := c.Request.Header.Values("X-Forwarded-For"); len(forwarded) > 0 {
		hops := strings.Split(strings.Join(forwarded, ","), ",")
		for i := len(hops) - 1; i >= 0; i-- {
			ip := net.ParseIP(strings.TrimSpace(hops[i]))
			if ip == nil {
				break
			}
			if !ipInNetworks(ip, trusted) {
				return ClientIdentity{IP: ip.String(), Forwarded: true}
			}
		}
		// Every hop was itself a trusted proxy; no client attribution exists.
		return ClientIdentity{IP: peer}
	}

	// X-Real-IP is the second header the shipped nginx sets.
	if realIP := net.ParseIP(strings.TrimSpace(c.GetHeader("X-Real-IP"))); realIP != nil && !ipInNetworks(realIP, trusted) {
		return ClientIdentity{IP: realIP.String(), Forwarded: true}
	}
	return ClientIdentity{IP: peer}
}

// ResolveClientIP is ResolveClientIdentity(...).IP.
func ResolveClientIP(c *gin.Context) string {
	return ResolveClientIdentity(c).IP
}

func peerIP(remoteAddr string) string {
	remoteAddr = strings.TrimSpace(remoteAddr)
	if remoteAddr == "" {
		return ""
	}
	if host, _, err := net.SplitHostPort(remoteAddr); err == nil {
		if ip := net.ParseIP(host); ip != nil {
			return ip.String()
		}
		return host
	}
	if ip := net.ParseIP(remoteAddr); ip != nil {
		return ip.String()
	}
	return remoteAddr
}

func ipInNetworks(ip net.IP, networks []*net.IPNet) bool {
	if ip == nil {
		return false
	}
	for _, network := range networks {
		if network != nil && network.Contains(ip) {
			return true
		}
	}
	return false
}
