package api

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/ifnodoraemon/airoute/internal/provider"
)

func newTestContext(method, target string) (*gin.Context, *httptest.ResponseRecorder) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(method, target, nil)
	return c, w
}

// TestLLMSessionCookieDoesNotCollideWithConsoleSession pins the cookie name
// split: the data plane used to read AND overwrite "airoute_session", which is
// the control-plane login cookie. That logged console users out on every
// inference call and leaked the admin JWT into usage_logs / Redis keys via the
// session-affinity path.
func TestLLMSessionCookieDoesNotCollideWithConsoleSession(t *testing.T) {
	if llmSessionCookie == "airoute_session" {
		t.Fatal("LLM affinity cookie must not reuse the console session cookie name")
	}

	c, _ := newTestContext(http.MethodPost, "/v1/chat/completions")
	// A console session cookie must never be picked up as an LLM session ID.
	c.Request.AddCookie(&http.Cookie{Name: "airoute_session", Value: "admin-jwt-token"})
	if got := extractSessionID(c, ""); got != "" {
		t.Fatalf("console session cookie leaked into LLM affinity: %q", got)
	}

	// The dedicated affinity cookie is honoured, and the value is bounded.
	c2, _ := newTestContext(http.MethodPost, "/v1/chat/completions")
	c2.Request.AddCookie(&http.Cookie{Name: llmSessionCookie, Value: "conv-123"})
	if got := extractSessionID(c2, ""); got != "conv-123" {
		t.Fatalf("expected affinity cookie to be used, got %q", got)
	}

	long := strings.Repeat("x", maxSessionIDLen*3)
	c3, _ := newTestContext(http.MethodPost, "/v1/chat/completions")
	c3.Request.Header.Set("X-Session-ID", long)
	if got := extractSessionID(c3, ""); len(got) > maxSessionIDLen {
		t.Fatalf("session ID not bounded: %d bytes", len(got))
	}
}

// TestSanitizeErrorMessageRedactsCredentials covers the leak that let upstream
// error bodies echo API keys back to the downstream caller.
func TestSanitizeErrorMessageRedactsCredentials(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string // must NOT contain
	}{
		{"openai style", `Incorrect API key provided: sk-proj-abcdef1234567890`, "sk-proj-abcdef1234567890"},
		{"bearer header", `Authorization: Bearer eyJhbGciOiJIUzI1NiJ9.payload.sig`, "eyJhbGciOiJIUzI1NiJ9.payload.sig"},
		{"json api_key", `{"error":{"api_key":"abcd1234efgh5678"}}`, "abcd1234efgh5678"},
		{"groq key", `invalid key gsk_ABCDEFGH12345678`, "gsk_ABCDEFGH12345678"},
		{"google key", `API key not valid: AIzaSyA1234567890abcdefghijklmnopqrstu`, "AIzaSyA1234567890abcdefghijklmnopqrstu"},
		{"key=value", `upstream said api_key=supersecretvalue`, "supersecretvalue"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := sanitizeErrorMessage(errors.New(tc.in))
			if strings.Contains(got, tc.want) {
				t.Fatalf("credential leaked: %q", got)
			}
			if !strings.Contains(got, "REDACTED") {
				t.Fatalf("expected a REDACTED marker, got %q", got)
			}
		})
	}

	// Ordinary diagnostics must stay readable.
	plain := "upstream openai returned status 400: model gpt-4o-mini does not exist"
	if got := sanitizeErrorMessage(errors.New(plain)); got != plain {
		t.Fatalf("normal message was mangled: %q", got)
	}
}

// TestEvalSimpleMathBoundsInput guards the remote stack-overflow DoS: deeply
// nested parentheses used to recurse once per character.
func TestEvalSimpleMathBoundsInput(t *testing.T) {
	deep := strings.Repeat("(", 5000) + "1" + strings.Repeat(")", 5000)
	if _, err := evalSimpleMath(deep); err == nil {
		t.Fatal("expected deeply nested expression to be rejected")
	}
	if _, err := evalSimpleMath(strings.Repeat("1+", 1000) + "1"); err == nil {
		t.Fatal("expected over-long expression to be rejected")
	}
	if got, err := evalSimpleMath("2*(3+4)^2"); err != nil || got != 98 {
		t.Fatalf("basic arithmetic regressed: got %v, err %v", got, err)
	}
}

// TestIsChannelFailureClassification pins which upstream outcomes may trip the
// circuit breaker. Treating every non-200 as a channel failure meant three
// malformed client requests could blackhole a perfectly healthy channel.
//
// The "client fault" half is split in two, because they call for different
// actions: IsFatalPayloadError (400/413/422) means the same request would fail on
// every channel, while IsModelNotFound (404) only means this node does not host
// the model, so dispatch keeps fanning out.
func TestIsChannelFailureClassification(t *testing.T) {
	cases := []struct {
		status        int
		channel       bool
		fatalPayload  bool
		modelNotFound bool
	}{
		{400, false, true, false},
		{404, false, false, true},
		{413, false, true, false},
		{422, false, true, false},
		{401, true, false, false},
		{403, true, false, false},
		{408, true, false, false},
		// 429 is backpressure from a healthy upstream, not an outage: tripping the
		// breaker on it replaces the actionable rate-limit response with gateway
		// 502s for the whole cooldown window.
		{429, false, false, false},
		{500, true, false, false},
		{502, true, false, false},
		{503, true, false, false},
	}
	for _, tc := range cases {
		err := provider.NewUpstreamError("ch", tc.status, []byte("boom"))
		if got := provider.IsChannelFailure(err); got != tc.channel {
			t.Errorf("status %d: IsChannelFailure=%v, want %v", tc.status, got, tc.channel)
		}
		if got := provider.IsFatalPayloadError(err); got != tc.fatalPayload {
			t.Errorf("status %d: IsFatalPayloadError=%v, want %v", tc.status, got, tc.fatalPayload)
		}
		if got := provider.IsModelNotFound(err); got != tc.modelNotFound {
			t.Errorf("status %d: IsModelNotFound=%v, want %v", tc.status, got, tc.modelNotFound)
		}
	}
}
