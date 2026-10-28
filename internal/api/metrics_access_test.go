package api

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

// TestMetricsAccessGuard pins the /metrics admission rules.
//
// C-01: the endpoint is limited to *attributable* loopback/private clients, the
// same predicate middleware.AdminAccessGuard uses. "The TCP peer is private" is not
// enough: behind the shipped Compose topology every request arrives from the nginx
// container, so a bare private-peer check let any internet client scrape the
// gateway's internal counters as soon as /metrics was mounted on the edge proxy.
func TestMetricsAccessGuard(t *testing.T) {
	cases := []struct {
		name           string
		token          string
		allowPublic    string
		authorization  string
		trustedProxies string
		remoteAddr     string
		xff            string
		wantStatus     int
	}{
		{"loopback allowed without token", "", "", "", "", "127.0.0.1:5000", "", http.StatusOK},
		{"private peer is not attributable without a trusted proxy", "", "", "", "", "10.1.2.3:5000", "", http.StatusForbidden},
		{"public denied without token", "", "", "", "", "203.0.113.9:5000", "", http.StatusForbidden},
		{"public allowed with explicit opt-in", "", "true", "", "", "203.0.113.9:5000", "", http.StatusOK},
		{"token required even on loopback", "s3cret", "", "", "", "127.0.0.1:5000", "", http.StatusUnauthorized},
		{"token accepted", "s3cret", "", "Bearer s3cret", "", "127.0.0.1:5000", "", http.StatusOK},
		{"wrong token rejected", "s3cret", "", "Bearer nope", "", "127.0.0.1:5000", "", http.StatusUnauthorized},
		{"token wins over public opt-in", "s3cret", "true", "", "", "203.0.113.9:5000", "", http.StatusUnauthorized},
		{
			name:           "trusted proxy forwarding a private client is allowed",
			trustedProxies: "10.0.0.0/8",
			remoteAddr:     "10.0.0.5:40000",
			xff:            "172.16.0.9",
			wantStatus:     http.StatusOK,
		},
		{
			name:           "trusted proxy forwarding a public client is denied",
			trustedProxies: "10.0.0.0/8",
			remoteAddr:     "10.0.0.5:40000",
			xff:            "203.0.113.9",
			wantStatus:     http.StatusForbidden,
		},
		{
			name:           "trusted proxy without a forwarding header is un-attributable",
			trustedProxies: "10.0.0.0/8",
			remoteAddr:     "10.0.0.5:40000",
			wantStatus:     http.StatusForbidden,
		},
		{
			name:           "peer outside the trusted list cannot claim a private client",
			trustedProxies: "10.0.0.0/8",
			remoteAddr:     "198.51.100.9:40000",
			xff:            "172.16.0.9",
			wantStatus:     http.StatusForbidden,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv(metricsTokenEnv, tc.token)
			t.Setenv(metricsAllowPublicEnv, tc.allowPublic)
			t.Setenv("GATEWAY_TRUSTED_PROXIES", tc.trustedProxies)

			engine := gin.New()
			engine.GET("/metrics", metricsAuthGuard(), func(c *gin.Context) {
				c.String(http.StatusOK, "airoute_test_metric 1\n")
			})

			req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
			req.RemoteAddr = tc.remoteAddr
			if tc.authorization != "" {
				req.Header.Set("Authorization", tc.authorization)
			}
			if tc.xff != "" {
				req.Header.Set("X-Forwarded-For", tc.xff)
			}
			rec := httptest.NewRecorder()
			engine.ServeHTTP(rec, req)

			if rec.Code != tc.wantStatus {
				t.Fatalf("status = %d, want %d (body: %s)", rec.Code, tc.wantStatus, rec.Body.String())
			}
		})
	}
}
