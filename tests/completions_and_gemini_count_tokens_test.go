package tests

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/ifnodoraemon/airoute/internal/api"
	"github.com/ifnodoraemon/airoute/internal/model"
	"github.com/ifnodoraemon/airoute/internal/router"
)

// TestV1Completions verifies legacy OpenAI POST /v1/completions non-stream and stream modes.
func TestV1Completions(t *testing.T) {
	// Mock upstream OpenAI server returning chat completions
	mockUpstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/chat/completions" {
			var chatReq model.ChatCompletionRequest
			if err := json.NewDecoder(r.Body).Decode(&chatReq); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}

			if chatReq.Stream {
				w.Header().Set("Content-Type", "text/event-stream")
				w.WriteHeader(http.StatusOK)
				flusher := w.(http.Flusher)

				chunk1 := `{"id":"cmpl-1","choices":[{"index":0,"delta":{"content":"Hello "},"finish_reason":null}]}`
				fmt.Fprintf(w, "data: %s\n\n", chunk1)
				flusher.Flush()

				stopReason := "stop"
				chunk2 := fmt.Sprintf(`{"id":"cmpl-1","choices":[{"index":0,"delta":{"content":"world!"},"finish_reason":"%s"}]}`, stopReason)
				fmt.Fprintf(w, "data: %s\n\n", chunk2)
				flusher.Flush()

				fmt.Fprintf(w, "data: [DONE]\n\n")
				flusher.Flush()
				return
			}

			stopReason := "stop"
			resp := model.ChatCompletionResponse{
				ID:      "chatcmpl-mock",
				Object:  "chat.completion",
				Created: 1234567890,
				Model:   chatReq.Model,
				Choices: []model.ChatCompletionChoice{
					{
						Index: 0,
						Message: model.ChatMessage{
							Role:    "assistant",
							Content: "This is a completed text response.",
						},
						FinishReason: &stopReason,
					},
				},
				Usage: &model.Usage{
					PromptTokens:     10,
					CompletionTokens: 6,
					TotalTokens:      16,
				},
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(resp)
			return
		}
		http.NotFound(w, r)
	}))
	defer mockUpstream.Close()

	channels := []model.ChannelConfig{
		{
			Name:     "upstream-openai",
			Type:     model.ProviderOpenAI,
			BaseURL:  mockUpstream.URL,
			Models:   []string{"text-davinci-003", "gpt-3.5-turbo-instruct"},
			Priority: 1,
			Weight:   10,
		},
	}

	dispatcher := router.NewDispatcher(channels)
	handler := api.NewHandler(dispatcher)

	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.POST("/v1/completions", handler.HandleCompletions)

	// 1. Non-streaming text completion with string prompt
	t.Run("Non-streaming string prompt", func(t *testing.T) {
		reqBody := `{"model":"text-davinci-003","prompt":"Say hello","max_tokens":50,"temperature":0.7}`
		req := httptest.NewRequest(http.MethodPost, "/v1/completions", bytes.NewBufferString(reqBody))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		engine.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
		}

		var textResp api.TextCompletionResponse
		if err := json.Unmarshal(w.Body.Bytes(), &textResp); err != nil {
			t.Fatalf("failed to unmarshal text response: %v", err)
		}

		if textResp.Object != "text_completion" {
			t.Errorf("expected object 'text_completion', got '%s'", textResp.Object)
		}
		if len(textResp.Choices) == 0 {
			t.Fatalf("expected choices, got empty")
		}
		if textResp.Choices[0].Text != "This is a completed text response." {
			t.Errorf("unexpected choice text: %s", textResp.Choices[0].Text)
		}
		if textResp.Usage == nil || textResp.Usage.TotalTokens != 16 {
			t.Errorf("expected 16 total tokens, got %+v", textResp.Usage)
		}
	})

	// 2. Non-streaming text completion with array prompt
	t.Run("Non-streaming array prompt", func(t *testing.T) {
		reqBody := `{"model":"gpt-3.5-turbo-instruct","prompt":["Line 1","Line 2"],"max_tokens":30}`
		req := httptest.NewRequest(http.MethodPost, "/v1/completions", bytes.NewBufferString(reqBody))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		engine.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
		}

		var textResp api.TextCompletionResponse
		if err := json.Unmarshal(w.Body.Bytes(), &textResp); err != nil {
			t.Fatalf("failed to unmarshal response: %v", err)
		}
		if len(textResp.Choices) == 0 {
			t.Fatalf("expected at least 1 choice")
		}
	})

	// 3. Streaming text completion
	t.Run("Streaming SSE text completion", func(t *testing.T) {
		reqBody := `{"model":"gpt-3.5-turbo-instruct","prompt":"Count to 2","stream":true}`
		req := httptest.NewRequest(http.MethodPost, "/v1/completions", bytes.NewBufferString(reqBody))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		engine.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
		}

		bodyStr := w.Body.String()
		if !strings.Contains(bodyStr, "data: [DONE]") {
			t.Errorf("expected SSE stream to contain [DONE], got:\n%s", bodyStr)
		}
		if !strings.Contains(bodyStr, "Hello ") || !strings.Contains(bodyStr, "world!") {
			t.Errorf("expected SSE chunks with 'Hello ' and 'world!', got:\n%s", bodyStr)
		}
	})
}

// TestUpstreamPureTextCompletionAdapter verifies that an upstream channel supporting only
// "openai_text" (i.e. /v1/completions) properly receives adapted prompt requests and responses.
func TestUpstreamPureTextCompletionAdapter(t *testing.T) {
	var receivedPrompt string
	var wasStream bool

	mockTextUpstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/completions" {
			http.NotFound(w, r)
			return
		}

		var payload struct {
			Model  string `json:"model"`
			Prompt string `json:"prompt"`
			Stream bool   `json:"stream"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		receivedPrompt = payload.Prompt
		wasStream = payload.Stream

		if payload.Stream {
			w.Header().Set("Content-Type", "text/event-stream")
			w.WriteHeader(http.StatusOK)
			flusher := w.(http.Flusher)

			fmt.Fprintf(w, "data: {\"id\":\"cmpl-stream\",\"created\":1700000000,\"choices\":[{\"text\":\"Adapted \",\"index\":0,\"finish_reason\":null}]}\n\n")
			flusher.Flush()
			fmt.Fprintf(w, "data: {\"id\":\"cmpl-stream\",\"created\":1700000000,\"choices\":[{\"text\":\"stream!\",\"index\":0,\"finish_reason\":\"stop\"}]}\n\n")
			flusher.Flush()
			fmt.Fprintf(w, "data: [DONE]\n\n")
			flusher.Flush()
			return
		}

		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"id":"cmpl-text-1","created":1700000000,"choices":[{"text":"Response from pure text upstream","index":0,"finish_reason":"stop"}],"usage":{"prompt_tokens":5,"completion_tokens":6,"total_tokens":11}}`)
	}))
	defer mockTextUpstream.Close()

	channels := []model.ChannelConfig{
		{
			Name:      "legacy-text-node",
			Type:      model.ProviderOpenAI,
			BaseURL:   mockTextUpstream.URL,
			Models:    []string{"text-davinci-003"},
			Priority:  1,
			Weight:    10,
			Protocols: []string{"openai_text"}, // explicitly only supports openai_text
		},
	}

	dispatcher := router.NewDispatcher(channels)
	handler := api.NewHandler(dispatcher)

	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.POST("/v1/chat/completions", handler.HandleChatCompletions)

	// Call /v1/chat/completions -> Airoute adapts it to /completions upstream
	t.Run("Chat to Text Upstream Adaptation Non-stream", func(t *testing.T) {
		reqBody := `{"model":"text-davinci-003","messages":[{"role":"system","content":"You are a helpful assistant."},{"role":"user","content":"Hello world!"}],"stream":false}`
		req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewBufferString(reqBody))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		engine.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
		}

		if !strings.Contains(receivedPrompt, "Hello world!") {
			t.Errorf("expected receivedPrompt to contain 'Hello world!', got: %s", receivedPrompt)
		}
		if wasStream {
			t.Errorf("expected wasStream to be false")
		}

		var chatResp model.ChatCompletionResponse
		if err := json.Unmarshal(w.Body.Bytes(), &chatResp); err != nil {
			t.Fatalf("failed to decode chat response: %v", err)
		}
		if len(chatResp.Choices) == 0 || chatResp.Choices[0].Message.Content != "Response from pure text upstream" {
			t.Errorf("unexpected choice message: %+v", chatResp.Choices)
		}
	})

	t.Run("Chat to Text Upstream Adaptation Streaming", func(t *testing.T) {
		reqBody := `{"model":"text-davinci-003","messages":[{"role":"user","content":"Stream this prompt"}],"stream":true}`
		req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewBufferString(reqBody))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		engine.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
		}

		scanner := bufio.NewScanner(w.Body)
		var collectedText string
		hasDone := false

		for scanner.Scan() {
			line := scanner.Text()
			if strings.HasPrefix(line, "data: [DONE]") {
				hasDone = true
				continue
			}
			if strings.HasPrefix(line, "data: ") {
				var chunk model.ChatCompletionChunk
				dataStr := strings.TrimPrefix(line, "data: ")
				if err := json.Unmarshal([]byte(dataStr), &chunk); err == nil && len(chunk.Choices) > 0 {
					collectedText += chunk.Choices[0].Delta.Content
				}
			}
		}

		if !hasDone {
			t.Errorf("expected stream to finish with [DONE]")
		}
		if collectedText != "Adapted stream!" {
			t.Errorf("expected 'Adapted stream!', got '%s'", collectedText)
		}
	})
}

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

// TestCompletionsProtocolRoutingPreference verifies that completions route to text-enabled channels
// and chat completions route to chat-enabled channels when both exist.
func TestCompletionsProtocolRoutingPreference(t *testing.T) {
	var invokedServer string

	chatUpstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		invokedServer = "chat-server"
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"id":"chat-1","choices":[{"message":{"role":"assistant","content":"chat response"}}]}`)
	}))
	defer chatUpstream.Close()

	textUpstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		invokedServer = "text-server"
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"id":"text-1","choices":[{"text":"text response"}]}`)
	}))
	defer textUpstream.Close()

	channels := []model.ChannelConfig{
		{
			Name:      "chat-only-channel",
			Type:      model.ProviderOpenAI,
			BaseURL:   chatUpstream.URL,
			Models:    []string{"shared-model"},
			Priority:  1,
			Protocols: []string{"openai_chat"},
		},
		{
			Name:      "text-only-channel",
			Type:      model.ProviderOpenAI,
			BaseURL:   textUpstream.URL,
			Models:    []string{"shared-model"},
			Priority:  1,
			Protocols: []string{"openai_text"},
		},
	}

	dispatcher := router.NewDispatcher(channels)
	handler := api.NewHandler(dispatcher)

	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.POST("/v1/chat/completions", handler.HandleChatCompletions)
	engine.POST("/v1/completions", handler.HandleCompletions)

	// 1. POST /v1/completions should route to text-server
	invokedServer = ""
	req1 := httptest.NewRequest(http.MethodPost, "/v1/completions", bytes.NewBufferString(`{"model":"shared-model","prompt":"hi"}`))
	req1.Header.Set("Content-Type", "application/json")
	w1 := httptest.NewRecorder()
	engine.ServeHTTP(w1, req1)
	if w1.Code != http.StatusOK {
		t.Fatalf("completions failed: %d, %s", w1.Code, w1.Body.String())
	}
	if invokedServer != "text-server" {
		t.Errorf("expected completions to route to text-server, got: %s", invokedServer)
	}

	// 2. POST /v1/chat/completions should route to chat-server
	invokedServer = ""
	req2 := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewBufferString(`{"model":"shared-model","messages":[{"role":"user","content":"hi"}]}`))
	req2.Header.Set("Content-Type", "application/json")
	w2 := httptest.NewRecorder()
	engine.ServeHTTP(w2, req2)
	if w2.Code != http.StatusOK {
		t.Fatalf("chat completions failed: %d, %s", w2.Code, w2.Body.String())
	}
	if invokedServer != "chat-server" {
		t.Errorf("expected chat completions to route to chat-server, got: %s", invokedServer)
	}
}
