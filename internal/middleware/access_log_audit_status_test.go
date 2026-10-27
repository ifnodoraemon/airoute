package middleware

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/ifnodoraemon/airoute/internal/telemetry"
)

const auditLogReason = "client closed request: 499 is synthesized by the gateway for a downstream disconnect"

// captureAccessLog runs one request through AccessLogMiddleware with the given
// wire status and optional audit status / error message, and returns the parsed
// http_access line. A dedicated logger keeps the assertion on our line instead of
// on stray output from other tests.
func captureAccessLog(t *testing.T, wireStatus, auditStatus int, message string) map[string]any {
	t.Helper()
	gin.SetMode(gin.TestMode)

	var buf bytes.Buffer
	previous := telemetry.Logger
	telemetry.Logger = slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelInfo}))
	t.Cleanup(func() { telemetry.Logger = previous })

	r := gin.New()
	r.Use(AccessLogMiddleware())
	r.POST("/v1/chat/completions", func(c *gin.Context) {
		if auditStatus != 0 {
			c.Set(ContextKeyAuditStatus, auditStatus)
		}
		if message != "" {
			c.Set(ContextKeyErrorMessage, message)
		}
		c.Status(wireStatus)
	})

	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	req.RemoteAddr = "203.0.113.7:4567"
	r.ServeHTTP(httptest.NewRecorder(), req)

	for _, line := range strings.Split(strings.TrimSpace(buf.String()), "\n") {
		if line == "" {
			continue
		}
		var entry map[string]any
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			t.Fatalf("access log line is not JSON: %q (%v)", line, err)
		}
		if entry["msg"] == "http_access" {
			return entry
		}
	}
	t.Fatalf("no http_access line was emitted; log was %q", buf.String())
	return nil
}

// The incident's stdout line said status=200 with no error, while the audit row
// said 499: the operator could not see the abort at all. When the audit status
// differs from the wire status, both must be on the access line.
func TestAccessLogShowsAuditStatusForWire200(t *testing.T) {
	entry := captureAccessLog(t, http.StatusOK, 499, auditLogReason)

	if got := entry["status"]; got != float64(http.StatusOK) {
		t.Fatalf("status = %v, want 200 (the wire status must stay unchanged)", got)
	}
	if got := entry["audit_status"]; got != float64(499) {
		t.Fatalf("audit_status = %v, want 499", got)
	}
	if got, _ := entry["error"].(string); !strings.Contains(got, "client closed request") {
		t.Fatalf("error = %q, want the synthesized 499 reason", got)
	}
}

// Requests whose audit status matches the wire status keep the pre-existing
// behaviour: a clean 200 line carries neither audit_status nor error.
func TestAccessLogLeavesCleanSuccessUnchanged(t *testing.T) {
	entry := captureAccessLog(t, http.StatusOK, 0, "")

	if got := entry["status"]; got != float64(http.StatusOK) {
		t.Fatalf("status = %v, want 200", got)
	}
	for _, key := range []string{"audit_status", "error"} {
		if _, exists := entry[key]; exists {
			t.Errorf("clean 200 line unexpectedly carries %s=%v", key, entry[key])
		}
	}
}

// The established failure path is untouched: a wire status >= 400 still prints
// the error message and gains no audit_status field.
func TestAccessLogFailurePathUnchanged(t *testing.T) {
	entry := captureAccessLog(t, http.StatusBadGateway, 0, "upstream unavailable")

	if got := entry["status"]; got != float64(http.StatusBadGateway) {
		t.Fatalf("status = %v, want 502", got)
	}
	if got, _ := entry["error"].(string); got != "upstream unavailable" {
		t.Fatalf("error = %q, want the recorded message", got)
	}
	if _, exists := entry["audit_status"]; exists {
		t.Errorf("502 line unexpectedly carries audit_status=%v", entry["audit_status"])
	}
}
