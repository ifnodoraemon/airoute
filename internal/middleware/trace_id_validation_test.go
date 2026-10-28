package middleware

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

// B-03 regression suite: a client-supplied trace id is adopted only when it is a
// well-formed id, because the value is echoed in response headers, written to every
// access-log line and persisted as usage_logs.trace_id.

func runTraceMiddleware(t *testing.T, headers map[string]string) (string, http.Header) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	req := httptest.NewRequest(http.MethodGet, "/v1/chat/completions", nil)
	for name, value := range headers {
		req.Header.Set(name, value)
	}
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = req
	TraceMiddleware()(c)
	return c.GetString(ContextKeyTraceID), w.Header()
}

func TestTraceIDAcceptsWellFormedValues(t *testing.T) {
	cases := map[string]string{
		HeaderRequestID:      "req-1234.ab_cd",
		HeaderAirouteTraceID: "tr-987654321-0123456789abcdef01234567",
		HeaderTraceID:        strings.Repeat("a", maxTraceIDLen),
	}

	for header, value := range cases {
		got, respHeaders := runTraceMiddleware(t, map[string]string{header: value})
		if got != value {
			t.Fatalf("%s=%q was not preserved: trace id = %q", header, value, got)
		}
		if respHeaders.Get(HeaderAirouteTraceID) != value {
			t.Fatalf("%s=%q not echoed in the response header: %q", header, value, respHeaders.Get(HeaderAirouteTraceID))
		}
	}

	// A proper W3C traceparent keeps only its trace-id, prefixed like the rest of
	// the gateway's ids.
	const traceparent = "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01"
	got, _ := runTraceMiddleware(t, map[string]string{HeaderTraceParent: traceparent})
	if want := "tr-4bf92f3577b34da6a3ce929d0e0e4736"; got != want {
		t.Fatalf("traceparent trace id = %q, want %q", got, want)
	}
}

func TestTraceIDRejectsHostileValues(t *testing.T) {
	long := strings.Repeat("A", 4096)
	cases := []struct {
		name    string
		headers map[string]string
	}{
		{"4KB header", map[string]string{HeaderRequestID: long}},
		{"CRLF injection", map[string]string{HeaderRequestID: "abc\r\nX-Injected: 1"}},
		{"space", map[string]string{HeaderRequestID: "abc def"}},
		{"over the length cap", map[string]string{HeaderRequestID: strings.Repeat("a", maxTraceIDLen+1)}},
		{"non-hex traceparent", map[string]string{HeaderTraceParent: "00-" + strings.Repeat("z", 32) + "-00f067aa0ba902b7-01"}},
		{"short traceparent fields", map[string]string{HeaderTraceParent: "00-abc-def-01"}},
		{"all-zero traceparent ids", map[string]string{HeaderTraceParent: "00-" + strings.Repeat("0", 32) + "-" + strings.Repeat("0", 16) + "-01"}},
		{"uppercase hex traceparent", map[string]string{HeaderTraceParent: "00-4BF92F3577B34DA6A3CE929D0E0E4736-00F067AA0BA902B7-01"}},
		{"too many traceparent fields", map[string]string{HeaderTraceParent: "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01-extra"}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, respHeaders := runTraceMiddleware(t, tc.headers)
			if got == "" {
				t.Fatal("no trace id was generated for a rejected header")
			}
			if !validCustomTraceID(got) {
				t.Fatalf("generated trace id %q is itself not a valid id", got)
			}
			for name, value := range tc.headers {
				if got == value || strings.Contains(got, "X-Injected") {
					t.Fatalf("hostile %s value leaked into the trace id: %q", name, got)
				}
			}
			if echoed := respHeaders.Get(HeaderAirouteTraceID); echoed != got {
				t.Fatalf("response header %q does not match the generated trace id %q", echoed, got)
			}
		})
	}
}

// TestTraceIDFallsThroughToTheNextValidHeader pins the header precedence: an
// invalid X-Request-ID must not stop the gateway from honouring a valid X-Trace-ID.
func TestTraceIDFallsThroughToTheNextValidHeader(t *testing.T) {
	got, _ := runTraceMiddleware(t, map[string]string{
		HeaderRequestID: "bad value with spaces",
		HeaderTraceID:   "good-trace-id",
	})
	if got != "good-trace-id" {
		t.Fatalf("trace id = %q, want the valid X-Trace-ID", got)
	}
}

// TestGeneratedTraceIDIsValid keeps the generator and the validator in sync: an id
// the gateway mints itself must pass the validator it applies to incoming values.
func TestGeneratedTraceIDIsValid(t *testing.T) {
	for i := 0; i < 100; i++ {
		if id := GenerateTraceID(); !validCustomTraceID(id) {
			t.Fatalf("generated trace id %q does not satisfy validCustomTraceID", id)
		}
	}
}
