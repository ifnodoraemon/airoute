package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

func init() {
	gin.SetMode(gin.TestMode)
}

func TestRespondOpenAIError(t *testing.T) {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	RespondOpenAIError(c, http.StatusBadRequest, "Invalid parameter", "invalid_request_error", "bad_param")

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d", w.Code)
	}

	var res map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &res); err != nil {
		t.Fatalf("failed to parse json response: %v", err)
	}
	errObj, ok := res["error"].(map[string]any)
	if !ok {
		t.Fatalf("expected 'error' object in response: %v", res)
	}
	if errObj["message"] != "Invalid parameter" || errObj["code"] != "bad_param" {
		t.Errorf("unexpected error payload: %+v", errObj)
	}
}

func TestRespondAnthropicError(t *testing.T) {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	RespondAnthropicError(c, http.StatusForbidden, "Forbidden access", "permission_error")

	if w.Code != http.StatusForbidden {
		t.Fatalf("expected status 403, got %d", w.Code)
	}

	var res map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &res); err != nil {
		t.Fatalf("failed to parse json response: %v", err)
	}
	if res["type"] != "error" {
		t.Errorf("expected type 'error', got %v", res["type"])
	}
	errObj, ok := res["error"].(map[string]any)
	if !ok || errObj["message"] != "Forbidden access" {
		t.Errorf("unexpected error payload: %+v", errObj)
	}
}

func TestRespondGeminiError(t *testing.T) {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	RespondGeminiError(c, http.StatusBadGateway, "Upstream down", "UNAVAILABLE")

	if w.Code != http.StatusBadGateway {
		t.Fatalf("expected status 502, got %d", w.Code)
	}

	var res map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &res); err != nil {
		t.Fatalf("failed to parse json response: %v", err)
	}
	errObj, ok := res["error"].(map[string]any)
	if !ok || errObj["status"] != "UNAVAILABLE" {
		t.Errorf("unexpected error payload: %+v", errObj)
	}
}

func TestResolveGenericSessionID(t *testing.T) {
	// Case 1: header provided
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("GET", "/", nil)
	c.Request.Header.Set("X-Session-ID", "custom_session_123")

	s1 := resolveGenericSessionID(c, "test")
	if s1 != "custom_session_123" {
		t.Errorf("expected 'custom_session_123', got '%s'", s1)
	}
	if w.Header().Get("X-Airoute-Session-ID") != "custom_session_123" {
		t.Errorf("expected header set, got '%s'", w.Header().Get("X-Airoute-Session-ID"))
	}

	// Case 2: generated with prefix
	w2 := httptest.NewRecorder()
	c2, _ := gin.CreateTestContext(w2)
	c2.Request = httptest.NewRequest("GET", "/", nil)

	s2 := resolveGenericSessionID(c2, "img")
	if len(s2) == 0 || s2[:9] != "sess_img_" {
		t.Errorf("expected prefix sess_img_, got '%s'", s2)
	}
}

func TestInitSSEStream(t *testing.T) {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	_, ok := InitSSEStream(c)
	if !ok {
		t.Errorf("expected flusher to be ok with httptest.ResponseRecorder")
	}

	if w.Header().Get("Content-Type") != "text/event-stream" {
		t.Errorf("expected Content-Type text/event-stream, got %s", w.Header().Get("Content-Type"))
	}
	if w.Header().Get("Cache-Control") != "no-cache" {
		t.Errorf("expected Cache-Control no-cache, got %s", w.Header().Get("Cache-Control"))
	}
}

func TestRecordUsage_SafeNilEngine(t *testing.T) {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("POST", "/v1/chat/completions", nil)

	cost := RecordUsage(c, AuditRecordParams{
		SessionID:        "sess_test",
		Model:            "gpt-4o",
		PromptTokens:     10,
		CompletionTokens: 20,
		Duration:         50 * time.Millisecond,
		StatusCode:       http.StatusOK,
	})

	if cost < 0 {
		t.Errorf("unexpected negative cost: %f", cost)
	}
}
