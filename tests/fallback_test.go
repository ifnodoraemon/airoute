package tests

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/ifnodoraemon/airoute/internal/config"
	"github.com/ifnodoraemon/airoute/internal/model"
	"github.com/ifnodoraemon/airoute/internal/router"
)

// TestSafeFallbackWindow verifies that if the primary channel returns HTTP 500,
// the dispatcher automatically falls back to the secondary backup channel seamlessly!
func TestSafeFallbackWindow_NonStreaming(t *testing.T) {
	// Mock Primary Channel: always returns 500 Internal Server Error
	primaryHits := 0
	primaryServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		primaryHits++
		http.Error(w, `{"error":"Upstream GPU server out of memory"}`, http.StatusInternalServerError)
	}))
	defer primaryServer.Close()

	// Mock Backup Channel: returns successful 200 OK
	backupHits := 0
	backupServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		backupHits++
		resp := model.ChatCompletionResponse{
			ID:      "chatcmpl-backup-123",
			Object:  "chat.completion",
			Created: time.Now().Unix(),
			Model:   "gpt-4o",
			Choices: []model.ChatCompletionChoice{
				{
					Index: 0,
					Message: model.ChatMessage{
						Role:    "assistant",
						Content: "Hello from backup channel!",
					},
				},
			},
			Usage: &model.Usage{
				PromptTokens:     10,
				CompletionTokens: 5,
				TotalTokens:      15,
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer backupServer.Close()

	// Configure channels: Primary (Priority 1), Backup (Priority 2)
	channels := []model.ChannelConfig{
		{
			Name:     "primary-faulty",
			Type:     model.ProviderOpenAI,
			BaseURL:  primaryServer.URL,
			APIKey:   "none",
			Models:   []string{"gpt-4o"},
			Priority: 1,
		},
		{
			Name:     "secondary-backup",
			Type:     model.ProviderOpenAI,
			BaseURL:  backupServer.URL,
			APIKey:   "none",
			Models:   []string{"gpt-4o"},
			Priority: 2,
		},
	}

	dispatcher := router.NewDispatcher(channels)

	req := &model.ChatCompletionRequest{
		Model: "gpt-4o",
		Messages: []model.ChatMessage{
			{Role: "user", Content: "Hello world"},
		},
		Stream: false,
	}

	resp, err := dispatcher.Dispatch(context.Background(), req)
	if err != nil {
		t.Fatalf("expected fallback to succeed, got error: %v", err)
	}

	if primaryHits != 1 {
		t.Errorf("expected primary to be attempted 1 time, got %d", primaryHits)
	}
	if backupHits != 1 {
		t.Errorf("expected backup to be hit 1 time, got %d", backupHits)
	}
	if resp.ID != "chatcmpl-backup-123" {
		t.Errorf("expected backup response ID, got %s", resp.ID)
	}
	content := resp.Choices[0].Message.GetContentString()
	if !strings.Contains(content, "backup channel") {
		t.Errorf("expected content from backup channel, got %s", content)
	}
}

// TestSafeFallbackWindow_Streaming verifies fallback during stream setup.
func TestSafeFallbackWindow_Streaming(t *testing.T) {
	// Mock Primary: returns 429 Too Many Requests
	primaryServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"error":"Rate limit exceeded"}`, http.StatusTooManyRequests)
	}))
	defer primaryServer.Close()

	// Mock Backup: streams SSE
	backupServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		flusher, ok := w.(http.Flusher)
		if !ok {
			return
		}

		chunk := model.ChatCompletionChunk{
			ID:      "chatcmpl-stream-backup",
			Object:  "chat.completion.chunk",
			Created: time.Now().Unix(),
			Model:   "gpt-4o",
			Choices: []model.ChunkChoice{
				{
					Index: 0,
					Delta: model.ChunkDelta{
						Content: "Stream token from backup",
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
	defer backupServer.Close()

	channels := []model.ChannelConfig{
		{
			Name:     "primary-rate-limited",
			Type:     model.ProviderOpenAI,
			BaseURL:  primaryServer.URL,
			APIKey:   "none",
			Models:   []string{"gpt-4o"},
			Priority: 1,
		},
		{
			Name:     "backup-stream",
			Type:     model.ProviderOpenAI,
			BaseURL:  backupServer.URL,
			APIKey:   "none",
			Models:   []string{"gpt-4o"},
			Priority: 2,
		},
	}

	dispatcher := router.NewDispatcher(channels)

	req := &model.ChatCompletionRequest{
		Model: "gpt-4o",
		Messages: []model.ChatMessage{
			{Role: "user", Content: "Stream test"},
		},
		Stream: true,
	}

	streamChan, err := dispatcher.DispatchStream(context.Background(), req)
	if err != nil {
		t.Fatalf("expected stream fallback to succeed, got error: %v", err)
	}

	var receivedChunks []*model.ChatCompletionChunk
	for event := range streamChan {
		if event.Err != nil {
			t.Fatalf("unexpected stream error: %v", event.Err)
		}
		if event.Chunk != nil {
			receivedChunks = append(receivedChunks, event.Chunk)
		}
	}

	if len(receivedChunks) == 0 {
		t.Fatalf("expected at least 1 chunk from backup, got 0")
	}

	if receivedChunks[0].Choices[0].Delta.Content != "Stream token from backup" {
		t.Errorf("unexpected content: %s", receivedChunks[0].Choices[0].Delta.Content)
	}
}

// TestAnthropicProviderAdapter verifies that Anthropic responses are normalized to OpenAI format.
func TestAnthropicProviderAdapter(t *testing.T) {
	mockAnthropicServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify Anthropic headers
		if r.Header.Get("x-api-key") != "anthropic-secret-key" {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		if r.Header.Get("anthropic-version") != "2023-06-01" {
			http.Error(w, "missing version", http.StatusBadRequest)
			return
		}

		// Return Anthropic format response
		anthropicResponse := map[string]any{
			"id":   "msg_anthropic_test_999",
			"type": "message",
			"role": "assistant",
			"content": []map[string]any{
				{
					"type": "text",
					"text": "Hello, I am Claude converted by airoute!",
				},
			},
			"model":       "claude-3-5-sonnet-20241022",
			"stop_reason": "end_turn",
			"usage": map[string]any{
				"input_tokens":  25,
				"output_tokens": 12,
			},
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(anthropicResponse)
	}))
	defer mockAnthropicServer.Close()

	channels := []model.ChannelConfig{
		{
			Name:     "claude-channel",
			Type:     model.ProviderAnthropic,
			BaseURL:  mockAnthropicServer.URL,
			APIKey:   "anthropic-secret-key",
			Models:   []string{"claude-3-5-sonnet"},
			Priority: 1,
		},
	}

	dispatcher := router.NewDispatcher(channels)

	req := &model.ChatCompletionRequest{
		Model: "claude-3-5-sonnet",
		Messages: []model.ChatMessage{
			{Role: "system", Content: "You are a helpful assistant."},
			{Role: "user", Content: "Hi Claude!"},
		},
		Stream: false,
	}

	resp, err := dispatcher.Dispatch(context.Background(), req)
	if err != nil {
		t.Fatalf("failed to call Anthropic provider: %v", err)
	}

	if resp.ID != "msg_anthropic_test_999" {
		t.Errorf("expected ID msg_anthropic_test_999, got %s", resp.ID)
	}
	content := resp.Choices[0].Message.GetContentString()
	if !strings.Contains(content, "I am Claude converted by airoute") {
		t.Errorf("expected translated content, got %s", content)
	}
	if resp.Usage == nil || resp.Usage.TotalTokens != 37 {
		t.Errorf("expected total tokens 37, got %+v", resp.Usage)
	}
}

// Ensure unused import warnings are cleared
var _ = bytes.NewReader
var _ = config.DefaultConfig
