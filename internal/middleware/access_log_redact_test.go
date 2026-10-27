package middleware

import (
	"strings"
	"testing"
)

// The MCP SSE transport carries its session capability as a query parameter, so
// an unredacted access-log line hands over the ability to impersonate that
// session (A08). Every credential-shaped query parameter must be redacted while
// the rest of the query string stays readable for debugging.
func TestSanitizeURLQueryRedactsMCPCredentials(t *testing.T) {
	for _, param := range []string{"sessionId", "session_id", "apiKey", "api_key", "key", "token"} {
		got := sanitizeURLQuery(param + "=super-secret-value&model=gpt-4o")
		if strings.Contains(got, "super-secret-value") {
			t.Errorf("%s query parameter was logged verbatim: %s", param, got)
		}
		if !strings.Contains(got, param+"=[REDACTED]") {
			t.Errorf("%s was not redacted in place: %s", param, got)
		}
	}

	// Non-credential parameters stay untouched: the log is still useful.
	got := sanitizeURLQuery("model=gpt-4o&page=2")
	if got != "model=gpt-4o&page=2" {
		t.Fatalf("non-sensitive query parameters were rewritten: %s", got)
	}
}
