package tests

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/ifnodoraemon/airoute/internal/api"
	"github.com/ifnodoraemon/airoute/internal/config"
	"github.com/ifnodoraemon/airoute/internal/model"
	"github.com/ifnodoraemon/airoute/internal/router"
)

// TestInboundAnthropic_ToOpenAIUpstream verifies that a Claude SDK client calling /v1/messages
// can be routed seamlessly to an upstream OpenAI/DeepSeek provider, and receive standard Anthropic format back!
func TestInboundAnthropic_ToOpenAIUpstream(t *testing.T) {
	// Mock upstream OpenAI-compatible server (e.g. DeepSeek / vLLM)
	upstreamServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Upstream receives canonical OpenAI request
		var req model.ChatCompletionRequest
		json.NewDecoder(r.Body).Decode(&req)

		// Verify system prompt was converted into a system message
		if len(req.Messages) < 2 || req.Messages[0].Role != "system" || req.Messages[0].GetContentString() != "You are a helpful coding assistant." {
			http.Error(w, "invalid messages", http.StatusBadRequest)
			return
		}

		resp := model.ChatCompletionResponse{
			ID:      "chatcmpl-from-deepseek-123",
			Object:  "chat.completion",
			Created: time.Now().Unix(),
			Model:   "deepseek-chat",
			Choices: []model.ChatCompletionChoice{
				{
					Index: 0,
					Message: model.ChatMessage{
						Role:    "assistant",
						Content: "Hello Claude user, this is answered by DeepSeek!",
					},
				},
			},
			Usage: &model.Usage{
				PromptTokens:     15,
				CompletionTokens: 10,
				TotalTokens:      25,
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer upstreamServer.Close()

	// Configure channel
	channels := []model.ChannelConfig{
		{
			Name:     "deepseek-upstream",
			Type:     model.ProviderOpenAI,
			BaseURL:  upstreamServer.URL,
			APIKey:   "sk-test-key",
			Models:   []string{"claude-3-5-sonnet"}, // client calls claude-3-5-sonnet, routed to this channel
			Priority: 1,
		},
	}

	testCfg := &config.Config{
		VirtualKeys: []model.VirtualKeyConfig{
			{
				Key:      "sk-gw-anthropic-client",
				TenantID: "claude-client-app",
			},
		},
		Channels: channels,
	}
	config.SetGlobalConfig(testCfg)

	dispatcher := router.NewDispatcher(channels)
	engine := api.SetupRouter(dispatcher, nil)

	// Inbound payload using Anthropic Messages format
	anthropicReqBody := `{
		"model": "claude-3-5-sonnet",
		"system": "You are a helpful coding assistant.",
		"messages": [
			{"role": "user", "content": "How are you?"}
		],
		"max_tokens": 1024,
		"stream": false
	}`

	req := httptest.NewRequest(http.MethodPost, "/v1/messages", bytes.NewBufferString(anthropicReqBody))
	// Test x-api-key header used by Anthropic Python/Node SDKs
	req.Header.Set("x-api-key", "sk-gw-anthropic-client")
	req.Header.Set("anthropic-version", "2023-06-01")
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 OK from /v1/messages, got %d: %s", w.Code, w.Body.String())
	}

	var anthropicResp map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &anthropicResp); err != nil {
		t.Fatalf("failed to parse JSON response: %v", err)
	}

	if anthropicResp["type"] != "message" {
		t.Errorf("expected type 'message', got %v", anthropicResp["type"])
	}

	contentList, ok := anthropicResp["content"].([]any)
	if !ok || len(contentList) == 0 {
		t.Fatalf("expected non-empty content blocks in response")
	}

	block := contentList[0].(map[string]any)
	text := block["text"].(string)
	if !strings.Contains(text, "Hello Claude user, this is answered by DeepSeek") {
		t.Errorf("unexpected content in Anthropic response: %s", text)
	}

	usage := anthropicResp["usage"].(map[string]any)
	if usage["input_tokens"] != float64(15) || usage["output_tokens"] != float64(10) {
		t.Errorf("unexpected usage: %+v", usage)
	}
}

// TestInboundAnthropic_Streaming verifies that Anthropic streaming calls emit Anthropic SSE events.
func TestInboundAnthropic_Streaming(t *testing.T) {
	// Mock upstream OpenAI-compatible streaming
	upstreamServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		flusher, ok := w.(http.Flusher)
		if !ok {
			return
		}

		chunk := model.ChatCompletionChunk{
			ID:      "chatcmpl-stream-1",
			Object:  "chat.completion.chunk",
			Created: time.Now().Unix(),
			Model:   "deepseek-chat",
			Choices: []model.ChunkChoice{
				{
					Index: 0,
					Delta: model.ChunkDelta{
						Content: "Hello from stream chunk!",
					},
				},
			},
		}
		b, _ := json.Marshal(chunk)
		fmt.Fprintf(w, "data: %s\n\n", b)
		flusher.Flush()

		fmt.Fprintf(w, "data: [DONE]\n\n")
		flusher.Flush()
	}))
	defer upstreamServer.Close()

	channels := []model.ChannelConfig{
		{
			Name:     "deepseek-upstream",
			Type:     model.ProviderOpenAI,
			BaseURL:  upstreamServer.URL,
			APIKey:   "sk-test-key",
			Models:   []string{"claude-3-5-sonnet"},
			Priority: 1,
		},
	}

	testCfg := &config.Config{
		VirtualKeys: []model.VirtualKeyConfig{
			{
				Key:      "sk-gw-stream-key",
				TenantID: "stream-tenant",
			},
		},
		Channels: channels,
	}
	config.SetGlobalConfig(testCfg)

	dispatcher := router.NewDispatcher(channels)
	engine := api.SetupRouter(dispatcher, nil)

	anthropicReqBody := `{
		"model": "claude-3-5-sonnet",
		"messages": [{"role": "user", "content": "Stream test"}],
		"stream": true
	}`

	req := httptest.NewRequest(http.MethodPost, "/v1/messages", bytes.NewBufferString(anthropicReqBody))
	req.Header.Set("x-api-key", "sk-gw-stream-key")
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for /v1/messages stream, got %d", w.Code)
	}

	bodyStr := w.Body.String()
	// Must contain Anthropic standard SSE events: message_start, content_block_delta, message_stop
	if !strings.Contains(bodyStr, "event: message_start") {
		t.Errorf("missing event: message_start in Anthropic stream: %s", bodyStr)
	}
	if !strings.Contains(bodyStr, "event: content_block_delta") {
		t.Errorf("missing event: content_block_delta in Anthropic stream: %s", bodyStr)
	}
	if !strings.Contains(bodyStr, "Hello from stream chunk!") {
		t.Errorf("missing delta text content in Anthropic stream: %s", bodyStr)
	}
	if !strings.Contains(bodyStr, "event: message_stop") {
		t.Errorf("missing event: message_stop in Anthropic stream: %s", bodyStr)
	}
}
