package middleware

import (
	"net"
	"net/http"
	"os"
	"strings"
	"sync/atomic"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/ifnodoraemon/airoute/internal/telemetry"
)

// Environment knobs for the control-plane access guard (D15).
//
// The gateway used to rely on nginx's api.* / console.* Host routing to keep
// /api/v1/admin and /app away from inference clients. That is routing, not a
// security boundary: the default catch-all vhost served both, so any client
// could reach the admin API by simply not sending the special Host header.
//
// The guard below is enforced inside the gateway (behind every proxy) and is
// fail-closed by default: only loopback and private-network clients reach the
// control plane. Public clients are rejected unless the operator explicitly
// configures an allowlist (or opts into public access, which logs a warning).
const (
	adminAllowedIPsEnv   = "GATEWAY_ADMIN_ALLOWED_IPS"
	adminAllowedHostsEnv = "GATEWAY_ADMIN_ALLOWED_HOSTS"
	adminAllowPublicEnv  = "GATEWAY_ADMIN_ALLOW_PUBLIC"
)

// adminGuardCache caches the parsed policy for the current environment values,
// like the trusted-proxy cache: the lists are static for the process lifetime
// and the guard runs on every console poll.
type adminGuardCache struct {
	spec   string
	policy adminAccessPolicy
}

var adminGuardCacheValue atomic.Pointer[adminGuardCache]

type adminAccessPolicy struct {
	allowedIPs   []*net.IPNet
	explicitIPs  bool
	allowedHosts []string
	allowPublic  bool
	err          error
}

// AdminAccessGuard restricts control-plane paths (/api/v1/admin and the embedded
// console under /app) to trusted clients.
//
// It must run after the trace/access-log middleware so denials are auditable, and
// before the static/console routes are registered.
func AdminAccessGuard() gin.HandlerFunc {
	return func(c *gin.Context) {
		if c == nil || c.Request == nil || !isControlPlanePath(c.Request.URL.Path) {
			c.Next()
			return
		}

		policy := loadAdminAccessPolicy()
		if reason, status := policy.decide(c); reason != "" {
			telemetry.WarnLogger(telemetry.ScopedLogger("auth"), "admin-access-denied", 30*time.Second,
				"control-plane access denied", "path", c.Request.URL.Path, "host", c.Request.Host,
				"reason", reason)
			if c.Writer != nil {
				c.Header("Cache-Control", "no-store")
			}
			c.AbortWithStatusJSON(status, gin.H{"code": status, "error": reason})
			return
		}
		c.Next()
	}
}

// isControlPlanePath reports whether path belongs to the management surface.
func isControlPlanePath(path string) bool {
	switch {
	case path == "/api/v1/admin", strings.HasPrefix(path, "/api/v1/admin/"):
		return true
	case path == "/app", strings.HasPrefix(path, "/app/"):
		return true
	}
	return false
}

func loadAdminAccessPolicy() adminAccessPolicy {
	ipsSpec := strings.TrimSpace(os.Getenv(adminAllowedIPsEnv))
	hostsSpec := strings.TrimSpace(os.Getenv(adminAllowedHostsEnv))
	publicSpec := strings.ToLower(strings.TrimSpace(os.Getenv(adminAllowPublicEnv)))
	spec := ipsSpec + "\x00" + hostsSpec + "\x00" + publicSpec

	if cached := adminGuardCacheValue.Load(); cached != nil && cached.spec == spec {
		return cached.policy
	}

	policy := parseAdminAccessPolicy(ipsSpec, hostsSpec, publicSpec)
	adminGuardCacheValue.Store(&adminGuardCache{spec: spec, policy: policy})
	return policy
}

func parseAdminAccessPolicy(ipsSpec, hostsSpec, publicSpec string) adminAccessPolicy {
	policy := adminAccessPolicy{
		allowPublic: publicSpec == "1" || publicSpec == "true" || publicSpec == "yes" || publicSpec == "on",
	}
	for _, field := range strings.FieldsFunc(hostsSpec, func(r rune) bool { return r == ',' || r == ' ' || r == '\t' }) {
		host := strings.ToLower(strings.TrimSpace(field))
		if host != "" {
			policy.allowedHosts = append(policy.allowedHosts, host)
		}
	}
	for _, field := range strings.FieldsFunc(ipsSpec, func(r rune) bool { return r == ',' || r == ' ' || r == '\t' }) {
		entry := strings.TrimSpace(field)
		if entry == "" {
			continue
		}
		policy.explicitIPs = true
		if _, network, err := net.ParseCIDR(entry); err == nil {
			policy.allowedIPs = append(policy.allowedIPs, network)
			continue
		}
		if ip := net.ParseIP(entry); ip != nil {
			bits := 128
			if v4 := ip.To4(); v4 != nil {
				ip = v4
				bits = 32
			}
			policy.allowedIPs = append(policy.allowedIPs, &net.IPNet{IP: ip, Mask: net.CIDRMask(bits, bits)})
			continue
		}
		policy.err = &adminPolicyError{entry: entry}
	}
	return policy
}

type adminPolicyError struct{ entry string }

func (e *adminPolicyError) Error() string {
	return "invalid entry in " + adminAllowedIPsEnv + ": " + e.entry
}

// decide returns a non-empty reason (and the HTTP status) when access is refused.
func (p adminAccessPolicy) decide(c *gin.Context) (string, int) {
	if p.err != nil {
		// A misconfigured allowlist must fail closed for the control plane.
		return "control-plane access policy is misconfigured", http.StatusServiceUnavailable
	}
	if len(p.allowedHosts) > 0 {
		if !hostMatches(c.Request.Host, p.allowedHosts) {
			return "control-plane access is restricted to configured hosts", http.StatusForbidden
		}
	}
	if p.allowPublic {
		return "", 0
	}

	raw := ResolveClientIP(c)
	ip := net.ParseIP(raw)
	if ip == nil {
		return "control-plane access could not determine the client address", http.StatusForbidden
	}
	if p.explicitIPs {
		if !ipInNetworks(ip, p.allowedIPs) {
			return "control-plane access is restricted to the configured allowlist", http.StatusForbidden
		}
		return "", 0
	}
	if ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() {
		return "", 0
	}
	return "control-plane access is restricted to loopback and private networks", http.StatusForbidden
}

// hostMatches compares the request host (port stripped, lower-cased) against an
// allowlist. A pattern beginning with "*." matches any sub-domain of that suffix.
func hostMatches(hostHeader string, patterns []string) bool {
	host := strings.ToLower(strings.TrimSpace(hostHeader))
	if h, _, err := net.SplitHostPort(host); err == nil {
		host = h
	}
	host = strings.Trim(host, "[]")
	if host == "" {
		return false
	}
	for _, pattern := range patterns {
		pattern = strings.ToLower(strings.TrimSpace(pattern))
		if pattern == "" {
			continue
		}
		if strings.HasPrefix(pattern, "*.") {
			suffix := pattern[1:] // ".example.com"
			if strings.HasSuffix(host, suffix) && len(host) > len(suffix) {
				return true
			}
			continue
		}
		if host == pattern {
			return true
		}
	}
	return false
}
