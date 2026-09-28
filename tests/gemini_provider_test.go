package tests

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ifnodoraemon/airoute/internal/model"
	"github.com/ifnodoraemon/airoute/internal/provider"
)

// TestGeminiProvider_NonStreaming tests Google Gemini specialized adapter.
func TestGeminiProvider_NonStreaming(t *testing.T) {
	mockGeminiServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// 1. Verify query key or header
		if r.URL.Query().Get("key") != "test-gemini-key" && r.Header.Get("x-goog-api-key") != "test-gemini-key" {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}

		// 2. Decode Gemini payload
		var geminiReq provider.GeminiRequest
		if err := json.NewDecoder(r.Body).Decode(&geminiReq); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		// Verify system instruction was captured
		if geminiReq.SystemInstruction == nil || len(geminiReq.SystemInstruction.Parts) == 0 || geminiReq.SystemInstruction.Parts[0].Text != "You are Gemini." {
			http.Error(w, "missing system instruction", http.StatusBadRequest)
			return
		}

		// Verify safety settings are present
		if len(geminiReq.SafetySettings) == 0 {
			http.Error(w, "missing safety settings", http.StatusBadRequest)
			return
		}

		// 3. Return Gemini format response
		respJSON := `{
			"candidates": [
				{
					"content": {
						"parts": [
							{"text": "Hello! I am Google Gemini translated by airoute."}
						],
						"role": "model"
					},
					"finishReason": "STOP",
					"index": 0
				}
			],
			"usageMetadata": {
				"promptTokenCount": 20,
				"candidatesTokenCount": 15,
				"totalTokenCount": 35
			}
		}`

		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(respJSON))
	}))
	defer mockGeminiServer.Close()

	chCfg := &model.ChannelConfig{
		Name:     "gemini-test-ch",
		Type:     model.ProviderGemini,
		BaseURL:  mockGeminiServer.URL,
		APIKey:   "test-gemini-key",
		Models:   []string{"gemini-1.5-pro"},
		Priority: 1,
	}

	p := provider.NewGeminiProvider(nil)

	req := &model.ChatCompletionRequest{
		Model: "gemini-1.5-pro",
		Messages: []model.ChatMessage{
			{Role: "system", Content: "You are Gemini."},
			{Role: "user", Content: "Hello Gemini!"},
		},
	}

	resp, err := p.ChatComplete(context.Background(), req, chCfg)
	if err != nil {
		t.Fatalf("failed to call Gemini provider: %v", err)
	}

	if len(resp.Choices) == 0 {
		t.Fatalf("expected choices from Gemini")
	}

	content := resp.Choices[0].Message.GetContentString()
	if !strings.Contains(content, "Google Gemini translated by airoute") {
		t.Errorf("unexpected content: %s", content)
	}

	if resp.Usage == nil || resp.Usage.TotalTokens != 35 {
		t.Errorf("expected 35 total tokens, got %+v", resp.Usage)
	}
}

// TestGeminiProvider_Streaming tests Google Gemini streamGenerateContent SSE translation.
func TestGeminiProvider_Streaming(t *testing.T) {
	mockGeminiServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		flusher, ok := w.(http.Flusher)
		if !ok {
			return
		}

		chunk1 := `{"candidates":[{"content":{"parts":[{"text":"Gemini streaming token 1"}]}}]}`
		fmt.Fprintf(w, "data: %s\n\n", chunk1)
		flusher.Flush()

		chunk2 := `{"candidates":[{"content":{"parts":[{"text":" token 2"}]},"finishReason":"STOP"}],"usageMetadata":{"promptTokenCount":10,"candidatesTokenCount":5,"totalTokenCount":15}}`
		fmt.Fprintf(w, "data: %s\n\n", chunk2)
		flusher.Flush()
	}))
	defer mockGeminiServer.Close()

	chCfg := &model.ChannelConfig{
		Name:     "gemini-stream-ch",
		Type:     model.ProviderGemini,
		BaseURL:  mockGeminiServer.URL,
		APIKey:   "test-gemini-key",
		Models:   []string{"gemini-1.5-flash"},
		Priority: 1,
	}

	p := provider.NewGeminiProvider(nil)

	req := &model.ChatCompletionRequest{
		Model: "gemini-1.5-flash",
		Messages: []model.ChatMessage{
			{Role: "user", Content: "Stream test"},
		},
		Stream: true,
	}

	streamChan, err := p.ChatCompleteStream(context.Background(), req, chCfg)
	if err != nil {
		t.Fatalf("failed to call Gemini stream: %v", err)
	}

	var accumulated strings.Builder
	for event := range streamChan {
		if event.Err != nil {
			t.Fatalf("stream event error: %v", event.Err)
		}
		if event.Chunk != nil && len(event.Chunk.Choices) > 0 {
			accumulated.WriteString(event.Chunk.Choices[0].Delta.Content)
		}
	}

	fullText := accumulated.String()
	if !strings.Contains(fullText, "Gemini streaming token 1 token 2") {
		t.Errorf("unexpected accumulated stream text: %s", fullText)
	}
}

// TestMultimodal_Parsing verifies parsing of complex multimodal OpenAI content arrays.
func TestMultimodal_Parsing(t *testing.T) {
	rawContent := []any{
		map[string]any{
			"type": "text",
			"text": "What is in this image?",
		},
		map[string]any{
			"type": "image_url",
			"image_url": map[string]any{
				"url": "data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mNk+M9QDwADhgGAWjR9awAAAABJRU5ErkJggg==",
			},
		},
		map[string]any{
			"type": "input_audio",
			"input_audio": map[string]any{
				"data":   "UklGRigAAABXQVZFZm10IBIAAAABAAEARKwAAIhYAQACABAAAABkYXRhAgAAAAEA",
				"format": "wav",
			},
		},
	}

	parts := model.ParseMessageContent(rawContent)
	if len(parts) != 3 {
		t.Fatalf("expected 3 parts, got %d", len(parts))
	}

	if parts[0].Type != model.ContentPartText || parts[0].Text != "What is in this image?" {
		t.Errorf("part 0 mismatch: %+v", parts[0])
	}
	if parts[1].Type != model.ContentPartImageURL || parts[1].ImageURL == nil {
		t.Errorf("part 1 mismatch: %+v", parts[1])
	}
	mime, b64 := model.ParseDataURI(parts[1].ImageURL.URL)
	if mime != "image/png" || !strings.HasPrefix(b64, "iVBORw") {
		t.Errorf("data URI parse mismatch: mime=%s, b64=%s", mime, b64)
	}

	if parts[2].Type != model.ContentPartInputAudio || parts[2].InputAudio == nil || parts[2].InputAudio.Format != "wav" {
		t.Errorf("part 2 mismatch: %+v", parts[2])
	}
}
