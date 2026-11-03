package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/ifnodoraemon/airoute/internal/middleware"
	"github.com/ifnodoraemon/airoute/internal/model"
	"github.com/ifnodoraemon/airoute/internal/validator"
)

// C-07: a protocol-validation rejection must leave a reason behind. It is what
// the access log and the audit recorder publish, and what an operator reads when
// a client reports a 400 the gateway refuses to explain.
func TestRejectInvalidRequestPublishesReason(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)

	rejectInvalidRequest(c, &validator.ValidationError{
		Protocol:   validator.ProtocolOpenAI,
		Field:      "messages",
		Message:    "Missing required parameter 'messages' or 'messages' array is empty",
		Code:       "invalid_messages",
		StatusCode: http.StatusBadRequest,
	})

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", w.Code)
	}
	reason := c.GetString(middleware.ContextKeyErrorMessage)
	if reason == "" {
		t.Fatal("validation rejection left no error message on the context")
	}
	for _, want := range []string{validator.ProtocolOpenAI, "messages", "array is empty"} {
		if !strings.Contains(reason, want) {
			t.Errorf("reason %q does not mention %q", reason, want)
		}
	}
	if body := w.Body.String(); !strings.Contains(body, "messages") {
		t.Errorf("client body %q does not carry the protocol error", body)
	}
}

// A nil error must not write a response: callers guard on valRes.Error, and a
// defensive no-op keeps this helper safe to call unconditionally.
func TestRejectInvalidRequestIgnoresNil(t *testing.T) {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	rejectInvalidRequest(c, nil)
	if w.Body.Len() != 0 {
		t.Fatalf("expected no response body, got %q", w.Body.String())
	}
}

// End-to-end through the handler: an invalid OpenAI payload used to produce a
// correct 400 with no audit reason at all. The validation level is pinned per key
// (FormatValidation overrides the global policy) so the case does not depend on
// the environment's default level.
func TestChatValidationRejectionIsAuditable(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	body := `{"model":"gpt-4o","messages":[]}`
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set(middleware.ContextKeyAPIKeyConfig, &model.APIKeyConfig{FormatValidation: validator.LevelStrict})

	// No dispatcher: validation must reject before any routing happens.
	h := NewHandler(nil)
	h.HandleChatCompletions(c)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 (body %q)", w.Code, w.Body.String())
	}
	if got := c.GetString(middleware.ContextKeyErrorMessage); got == "" {
		t.Fatal("rejected chat request produced no audit reason")
	}
}
