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
//  3. Otherwise the endpoint is limited to loopback/private clients. /metrics
//     exposes internal counters (billing drift, queue depth, channel failure
//     counts); leaving it world-readable made that topology reconnaissance for
//     free. Private clients keep the common Prometheus sidecar/Compose setups
//     working without configuration.
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
	ip := net.ParseIP(middleware.ResolveClientIP(c))
	if ip == nil {
		return http.StatusForbidden
	}
	if ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() {
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
