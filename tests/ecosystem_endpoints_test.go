package tests

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/ifnodoraemon/airoute/internal/api"
	"github.com/ifnodoraemon/airoute/internal/model"
	"github.com/ifnodoraemon/airoute/internal/router"
)

func TestModelDetailEndpoint(t *testing.T) {
	channels := []model.ChannelConfig{
		{
			Name:     "upstream-1",
			Type:     model.ProviderOpenAI,
			BaseURL:  "http://localhost:8080",
			Models:   []string{"gpt-4o", "text-embedding-3-small"},
			Priority: 1,
			Weight:   10,
		},
	}

	dispatcher := router.NewDispatcher(channels)
	handler := api.NewHandler(dispatcher)

	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.GET("/v1/models/:model", handler.HandleModelDetail)

	// 1. Success case: model exists
	reqValid := httptest.NewRequest(http.MethodGet, "/v1/models/gpt-4o", nil)
	wValid := httptest.NewRecorder()
	engine.ServeHTTP(wValid, reqValid)

	if wValid.Code != http.StatusOK {
		t.Fatalf("expected 200 for existing model, got %d: %s", wValid.Code, wValid.Body.String())
	}
	var modelItem model.ModelItem
	if err := json.Unmarshal(wValid.Body.Bytes(), &modelItem); err != nil {
		t.Fatalf("failed to parse model item: %v", err)
	}
	if modelItem.ID != "gpt-4o" {
		t.Errorf("expected model id 'gpt-4o', got %s", modelItem.ID)
	}
	if modelItem.OwnedBy != "airoute" {
		t.Errorf("expected owned_by 'airoute', got %s", modelItem.OwnedBy)
	}

	// 2. 404 case: non-existent model
	reqInvalid := httptest.NewRequest(http.MethodGet, "/v1/models/non-existent-model", nil)
	wInvalid := httptest.NewRecorder()
	engine.ServeHTTP(wInvalid, reqInvalid)

	if wInvalid.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for missing model, got %d: %s", wInvalid.Code, wInvalid.Body.String())
	}
}

func TestModerationsEndpoint(t *testing.T) {
	dispatcher := router.NewDispatcher(nil)
	handler := api.NewHandler(dispatcher)

	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.POST("/v1/moderations", handler.HandleModerations)

	payload := `{"input": "Hello world, this is a test prompt."}`
	req := httptest.NewRequest(http.MethodPost, "/v1/moderations", bytes.NewBufferString(payload))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for moderations, got %d: %s", w.Code, w.Body.String())
	}

	var resp api.ModerationResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to unmarshal moderation response: %v", err)
	}

	if len(resp.Results) == 0 {
		t.Fatalf("expected results array in moderation response")
	}
	if resp.Results[0].Flagged {
		t.Errorf("expected flagged to be false for safe test input")
	}
	if _, exists := resp.Results[0].Categories["sexual"]; !exists {
		t.Errorf("expected 'sexual' category in moderation results")
	}
}

func TestGeminiInboundProtocol(t *testing.T) {
	// Mock OpenAI upstream responding to the translated request
	openAIServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{
			"id": "chatcmpl-gemini-inbound-test",
			"object": "chat.completion",
			"created": 1700000000,
			"model": "gemini-1.5-flash",
			"choices": [
				{
					"index": 0,
					"message": {
						"role": "assistant",
						"content": "Hello from airoute via Gemini Inbound!"
					},
					"finish_reason": "stop"
				}
			],
			"usage": {
				"prompt_tokens": 12,
				"completion_tokens": 8,
				"total_tokens": 20
			}
		}`)
	}))
	defer openAIServer.Close()

	channels := []model.ChannelConfig{
		{
			Name:     "upstream-gemini-mock",
			Type:     model.ProviderOpenAI,
			BaseURL:  openAIServer.URL,
			Models:   []string{"gemini-1.5-flash"},
			Priority: 1,
			Weight:   10,
		},
	}

	dispatcher := router.NewDispatcher(channels)
	handler := api.NewHandler(dispatcher)

	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.GET("/v1beta/models", handler.HandleGeminiModels)
	engine.POST("/v1beta/models/*modelAction", handler.HandleGeminiAction)

	// 1. Test GET /v1beta/models
	getReq := httptest.NewRequest(http.MethodGet, "/v1beta/models", nil)
	getW := httptest.NewRecorder()
	engine.ServeHTTP(getW, getReq)

	if getW.Code != http.StatusOK {
		t.Fatalf("expected 200 for Gemini models list, got %d: %s", getW.Code, getW.Body.String())
	}

	// 2. Test POST /v1beta/models/gemini-1.5-flash:generateContent
	geminiReqBody := `{
		"contents": [
			{
				"role": "user",
				"parts": [{"text": "Hello!"}]
			}
		]
	}`

	postReq := httptest.NewRequest(http.MethodPost, "/v1beta/models/gemini-1.5-flash:generateContent", bytes.NewBufferString(geminiReqBody))
	postReq.Header.Set("Content-Type", "application/json")
	postW := httptest.NewRecorder()
	engine.ServeHTTP(postW, postReq)

	if postW.Code != http.StatusOK {
		t.Fatalf("expected 200 for Gemini generateContent, got %d: %s", postW.Code, postW.Body.String())
	}

	var geminiResp map[string]any
	if err := json.Unmarshal(postW.Body.Bytes(), &geminiResp); err != nil {
		t.Fatalf("failed to parse Gemini response: %v", err)
	}

	candidates, ok := geminiResp["candidates"].([]any)
	if !ok || len(candidates) == 0 {
		t.Fatalf("expected candidates in Gemini response: %+v", geminiResp)
	}

	cand := candidates[0].(map[string]any)
	content := cand["content"].(map[string]any)
	parts := content["parts"].([]any)
	if len(parts) == 0 {
		t.Fatalf("expected parts in candidate content")
	}
	part0 := parts[0].(map[string]any)
	text := part0["text"].(string)

	if text != "Hello from airoute via Gemini Inbound!" {
		t.Errorf("unexpected translated text: %s", text)
	}
}
