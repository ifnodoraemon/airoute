package api

import (
	"crypto/subtle"
	"net"
	"net/http"
	"os"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/ifnodoraemon/airoute/internal/middleware"
)

const (
	metricsTokenEnv       = "GATEWAY_METRICS_TOKEN"
	metricsAllowPublicEnv = "GATEWAY_METRICS_ALLOW_PUBLIC"
)

// metricsDeniedStatus decides whether a /metrics scrape may proceed and returns
// the HTTP status to reject it with, or 0 when access is allowed.
//
// Precedence:
//  1. GATEWAY_METRICS_TOKEN, when set, is authoritative: every scrape must carry
//     it as "Authorization: Bearer <token>", including private clients.
//  2. GATEWAY_METRICS_ALLOW_PUBLIC=true explicitly re-opens the endpoint.
//  3. Otherwise the endpoint is limited to attributable loopback/private clients.
//
// Point 3 uses the same predicate as middleware.AdminAccessGuard, and for the same
// reason: "the TCP peer is a private address" does not mean "a private client sent
// this". Behind the shipped Compose topology the gateway only exposes port 8080 and
// nginx forwards everything, so the peer is always the proxy - and a bare
// private-peer check let any internet client read /metrics (billing drift, queue
// depth, channel failure counts) as soon as the endpoint was mounted on the edge
// proxy. /metrics therefore requires attribution: the address has to come from a
// configured trusted proxy's forwarding header, or be loopback (a sidecar scraper
// or an operator on the box). An un-attributable peer fails closed.
func metricsDeniedStatus(c *gin.Context) int {
	if expected := strings.TrimSpace(os.Getenv(metricsTokenEnv)); expected != "" {
		got := strings.TrimPrefix(c.GetHeader("Authorization"), "Bearer ")
		if subtle.ConstantTimeCompare([]byte(got), []byte(expected)) != 1 {
			return http.StatusUnauthorized
		}
		return 0
	}
	if envFlagEnabled(metricsAllowPublicEnv) {
		return 0
	}
	identity := middleware.ResolveClientIdentity(c)
	ip := net.ParseIP(identity.IP)
	if ip == nil {
		return http.StatusForbidden
	}
	if identity.Forwarded {
		// The address came from a trusted proxy: it is the real client, so the
		// private-range check means what it says.
		if ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() {
			return 0
		}
		return http.StatusForbidden
	}
	// No usable forwarding information: only loopback is attributable to this host.
	// A private non-loopback peer may be an unconfigured proxy, and trusting it is
	// what made the old check fail open.
	if ip.IsLoopback() {
		return 0
	}
	return http.StatusForbidden
}

// envFlagEnabled parses the same truthy spellings the rest of the gateway uses.
func envFlagEnabled(name string) bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv(name))) {
	case "1", "true", "yes", "on":
		return true
	}
	return false
}
