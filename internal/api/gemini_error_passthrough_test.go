package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/ifnodoraemon/airoute/internal/provider"
)

func TestSanitizeGeminiErrorBodyPassesThroughEnvelope(t *testing.T) {
	raw := `{"error":{"code":400,"message":"Invalid JSON payload received.","status":"INVALID_ARGUMENT"}}`
	body, ok := sanitizeGeminiErrorBody(raw)
	if !ok {
		t.Fatalf("expected a Gemini envelope to pass through")
	}
	var got struct {
		Error struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
			Status  string `json:"status"`
		} `json:"error"`
	}
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("scrubbed body is not valid JSON: %v", err)
	}
	if got.Error.Status != "INVALID_ARGUMENT" || got.Error.Code != 400 || got.Error.Message != "Invalid JSON payload received." {
		t.Fatalf("upstream envelope was altered: %s", body)
	}
}

func TestSanitizeGeminiErrorBodyScrubsCredentials(t *testing.T) {
	raw := `{"error":{"code":401,"message":"Incorrect API key provided: sk-abcdef1234567890.","status":"UNAUTHENTICATED"}}`
	body, ok := sanitizeGeminiErrorBody(raw)
	if !ok {
		t.Fatalf("expected envelope to pass through")
	}
	if strings.Contains(string(body), "sk-abcdef1234567890") {
		t.Fatalf("credential leaked through passthrough: %s", body)
	}
	if !json.Valid(body) {
		t.Fatalf("scrubbing broke the JSON: %s", body)
	}
}

func TestSanitizeGeminiErrorBodyRejectsNonGemini(t *testing.T) {
	for _, raw := range []string{
		``,
		`not json`,
		`{"error":{"message":"openai shaped","type":"invalid_request_error","code":null}}`,
		`{"candidates":[]}`,
	} {
		if _, ok := sanitizeGeminiErrorBody(raw); ok {
			t.Fatalf("non-Gemini body accepted: %q", raw)
		}
	}
}

func TestRespondGeminiUpstreamErrorPassthrough(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1beta/models/gemini:generateContent", nil)

	upstreamBody := `{"error":{"code":429,"message":"Resource exhausted.","status":"RESOURCE_EXHAUSTED"}}`
	err := provider.NewUpstreamError("gemini-chan", http.StatusTooManyRequests, []byte(upstreamBody))
	respondGeminiUpstreamError(c, http.StatusTooManyRequests, err)

	if w.Code != http.StatusTooManyRequests {
		t.Fatalf("status = %d, want 429", w.Code)
	}
	var got struct {
		Error struct {
			Status string `json:"status"`
		} `json:"error"`
	}
	if e := json.Unmarshal(w.Body.Bytes(), &got); e != nil {
		t.Fatalf("response is not the upstream envelope: %v (%s)", e, w.Body.String())
	}
	if got.Error.Status != "RESOURCE_EXHAUSTED" {
		t.Fatalf("upstream status lost: %s", w.Body.String())
	}
	if strings.Contains(w.Body.String(), "returned status") {
		t.Fatalf("body was re-wrapped in gateway prose: %s", w.Body.String())
	}
}

func TestRespondGeminiUpstreamErrorFallsBackForNonGemini(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1beta/models/gemini:generateContent", nil)

	// An OpenAI channel serving a converted Gemini request returns an OpenAI-shaped
	// error; it must be re-enveloped as Gemini, not passed through verbatim.
	err := provider.NewUpstreamError("openai-chan", http.StatusBadRequest, []byte(`{"error":{"message":"bad","type":"invalid_request_error"}}`))
	respondGeminiUpstreamError(c, http.StatusBadRequest, err)

	var got struct {
		Error struct {
			Status string `json:"status"`
			Type   string `json:"type"`
		} `json:"error"`
	}
	if e := json.Unmarshal(w.Body.Bytes(), &got); e != nil {
		t.Fatalf("response not valid JSON: %v", e)
	}
	if got.Error.Status != "INVALID_ARGUMENT" {
		t.Fatalf("expected standardized Gemini envelope, got: %s", w.Body.String())
	}
}