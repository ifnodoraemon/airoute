package tests

import (
	"bytes"
	"encoding/json"
	"github.com/gin-gonic/gin"
	"github.com/ifnodoraemon/airoute/internal/api"
	"github.com/ifnodoraemon/airoute/internal/router"
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestGeminiCountTokensEndpoint verifies POST /v1beta/models/:model:countTokens.
func TestGeminiCountTokensEndpoint(t *testing.T) {
	dispatcher := router.NewDispatcher(nil)
	handler := api.NewHandler(dispatcher)

	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.POST("/v1beta/models/*modelAction", handler.HandleGeminiAction)

	// Valid countTokens payload with system instruction and user contents
	payload := `{
		"contents": [
			{
				"role": "user",
				"parts": [
					{"text": "Hello world, what is the capital of France?"}
				]
			}
		],
		"systemInstruction": {
			"parts": [
				{"text": "You are a concise geography assistant."}
			]
		}
	}`

	req := httptest.NewRequest(http.MethodPost, "/v1beta/models/gemini-1.5-pro:countTokens", bytes.NewBufferString(payload))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for countTokens, got %d: %s", w.Code, w.Body.String())
	}

	var resp struct {
		TotalTokens int `json:"totalTokens"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to unmarshal countTokens response: %v", err)
	}

	if resp.TotalTokens <= 0 {
		t.Errorf("expected totalTokens > 0, got %d", resp.TotalTokens)
	}
}
