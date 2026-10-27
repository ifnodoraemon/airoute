package api

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestMetricsAccessGuard(t *testing.T) {
	cases := []struct {
		name          string
		token         string
		allowPublic   string
		authorization string
		remoteAddr    string
		wantStatus    int
	}{
		{"loopback allowed without token", "", "", "", "127.0.0.1:5000", http.StatusOK},
		{"private allowed without token", "", "", "", "10.1.2.3:5000", http.StatusOK},
		{"public denied without token", "", "", "", "203.0.113.9:5000", http.StatusForbidden},
		{"public allowed with explicit opt-in", "", "true", "", "203.0.113.9:5000", http.StatusOK},
		{"token required even on loopback", "s3cret", "", "", "127.0.0.1:5000", http.StatusUnauthorized},
		{"token accepted", "s3cret", "", "Bearer s3cret", "127.0.0.1:5000", http.StatusOK},
		{"wrong token rejected", "s3cret", "", "Bearer nope", "127.0.0.1:5000", http.StatusUnauthorized},
		{"token wins over public opt-in", "s3cret", "true", "", "203.0.113.9:5000", http.StatusUnauthorized},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv(metricsTokenEnv, tc.token)
			t.Setenv(metricsAllowPublicEnv, tc.allowPublic)
			t.Setenv("GATEWAY_TRUSTED_PROXIES", "")

			engine := gin.New()
			engine.GET("/metrics", metricsAuthGuard(), func(c *gin.Context) {
				c.String(http.StatusOK, "airoute_test_metric 1\n")
			})

			req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
			req.RemoteAddr = tc.remoteAddr
			if tc.authorization != "" {
				req.Header.Set("Authorization", tc.authorization)
			}
			rec := httptest.NewRecorder()
			engine.ServeHTTP(rec, req)

			if rec.Code != tc.wantStatus {
				t.Fatalf("status = %d, want %d (body: %s)", rec.Code, tc.wantStatus, rec.Body.String())
			}
		})
	}
}
