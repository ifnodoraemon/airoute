package tests

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ifnodoraemon/airoute/internal/api"
	"github.com/ifnodoraemon/airoute/internal/config"
	"github.com/ifnodoraemon/airoute/internal/model"
	"github.com/ifnodoraemon/airoute/internal/router"
)

func TestOpenAIResponses_NonStreaming(t *testing.T) {
	// Mock upstream OpenAI chat server
	upstreamServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			http.NotFound(w, r)
			return
		}
		var chatReq model.ChatCompletionRequest
		_ = json.NewDecoder(r.Body).Decode(&chatReq)

		stop := "stop"
		resp := model.ChatCompletionResponse{
			ID:      "chatcmpl-test-12345",
			Object:  "chat.completion",
			Created: 1727330000,
			Model:   chatReq.Model,
			Choices: []model.ChatCompletionChoice{
				{
					Index: 0,
					Message: model.ChatMessage{
						Role:    "assistant",
						Content: "Hello from OpenAI Responses adapter!",
					},
					FinishReason: &stop,
				},
			},
			Usage: &model.Usage{
				PromptTokens:     15,
				CompletionTokens: 10,
				TotalTokens:      25,
			},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer upstreamServer.Close()

	oldCfg := config.GetGlobalConfig()
	defer config.SetGlobalConfig(oldCfg)
	config.SetGlobalConfig(&config.Config{
		VirtualKeys: []model.VirtualKeyConfig{
			{
				Key:      "sk-test-client-key",
				TenantID: "default-tenant",
			},
		},
	})

	dispatcher := router.NewDispatcher(nil)

	// Create channel supporting openai_response
	ch := &model.ChannelConfig{
		Name:      "test-openai-provider",
		Type:      model.ProviderOpenAI,
		BaseURL:   upstreamServer.URL + "/v1",
		APIKey:    "sk-mock-key",
		Models:    []string{"gpt-4o"},
		Protocols: []string{"openai_response", "openai_chat"},
		Priority:  1,
		Weight:    1,
	}
	dispatcher.UpdateChannels([]model.ChannelConfig{*ch})

	engine := api.SetupRouter(dispatcher, nil)

	// Test 1: Simple text input
	reqBody := `{"model": "gpt-4o", "input": "Hello", "instructions": "You are a friendly AI."}`
	req := httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewBufferString(reqBody))
	req.Header.Set("Authorization", "Bearer sk-test-client-key")
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	engine.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", w.Code, w.Body.String())
	}

	var res model.ResponseResult
	if err := json.Unmarshal(w.Body.Bytes(), &res); err != nil {
		t.Fatalf("failed to parse response json: %v", err)
	}

	if res.Object != "response" {
		t.Fatalf("expected object 'response', got '%s'", res.Object)
	}
	if len(res.Output) == 0 {
		t.Fatalf("expected non-empty output array")
	}
	if res.Output[0].Content[0].Text != "Hello from OpenAI Responses adapter!" {
		t.Fatalf("unexpected output text: %s", res.Output[0].Content[0].Text)
	}
	if res.Usage.TotalTokens != 25 {
		t.Fatalf("expected total_tokens 25, got %d", res.Usage.TotalTokens)
	}
}

func TestOpenAIResponses_Streaming(t *testing.T) {
	// Mock upstream OpenAI streaming chat server
	upstreamServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		flusher, _ := w.(http.Flusher)

		chunk1 := `data: {"id":"chatcmpl-stream-1","choices":[{"index":0,"delta":{"role":"assistant","content":"Stream "}}]}` + "\n\n"
		_, _ = w.Write([]byte(chunk1))
		flusher.Flush()

		chunk2 := `data: {"id":"chatcmpl-stream-1","choices":[{"index":0,"delta":{"content":"Response!"}}]}` + "\n\n"
		_, _ = w.Write([]byte(chunk2))
		flusher.Flush()

		_, _ = w.Write([]byte("data: [DONE]\n\n"))
		flusher.Flush()
	}))
	defer upstreamServer.Close()

	dispatcher := router.NewDispatcher(nil)
	ch := &model.ChannelConfig{
		Name:      "test-openai-provider",
		Type:      model.ProviderOpenAI,
		BaseURL:   upstreamServer.URL + "/v1",
		APIKey:    "sk-mock-key",
		Models:    []string{"gpt-4o"},
		Protocols: []string{"openai_response"},
		Priority:  1,
		Weight:    1,
	}
	dispatcher.UpdateChannels([]model.ChannelConfig{*ch})

	oldCfg := config.GetGlobalConfig()
	defer config.SetGlobalConfig(oldCfg)
	config.SetGlobalConfig(&config.Config{
		VirtualKeys: []model.VirtualKeyConfig{
			{
				Key:      "sk-test-client-key",
				TenantID: "default-tenant",
			},
		},
	})

	engine := api.SetupRouter(dispatcher, nil)

	reqBody := `{"model": "gpt-4o", "input": "Tell me something", "stream": true}`
	req := httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewBufferString(reqBody))
	req.Header.Set("Authorization", "Bearer sk-test-client-key")
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	engine.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", w.Code, w.Body.String())
	}

	bodyStr := w.Body.String()
	if !strings.Contains(bodyStr, "event: response.created") {
		t.Fatalf("expected response.created event in stream, got:\n%s", bodyStr)
	}
	if !strings.Contains(bodyStr, "event: response.output_text.delta") {
		t.Fatalf("expected response.output_text.delta event in stream, got:\n%s", bodyStr)
	}
	if !strings.Contains(bodyStr, "event: response.completed") {
		t.Fatalf("expected response.completed event in stream, got:\n%s", bodyStr)
	}
	if !strings.Contains(bodyStr, "data: [DONE]") {
		t.Fatalf("expected data: [DONE] at end of stream, got:\n%s", bodyStr)
	}
}
