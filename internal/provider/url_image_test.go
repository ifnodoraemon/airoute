package provider

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/ifnodoraemon/airoute/internal/model"
)

// F-07 regression (outbound): a remote image URL is a URL on every supported
// wire format. It must never be written into a base64-only field, where the
// upstream rejects it.
func urlImageChatRequest() *model.ChatCompletionRequest {
	return &model.ChatCompletionRequest{
		Model: "claude-3-5-sonnet",
		Messages: []model.ChatMessage{{Role: "user", Content: []any{
			map[string]any{"type": "text", "text": "describe this"},
			map[string]any{"type": "image_url", "image_url": map[string]any{"url": "https://example.com/photo.png"}},
		}}},
	}
}

func TestAnthropicConvertForwardsRemoteImageAsURLSource(t *testing.T) {
	encoded, err := json.Marshal(convertOpenAIToAnthropic(urlImageChatRequest(), &model.ChannelConfig{Name: "claude"}))
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if !strings.Contains(string(encoded), `"source":{"type":"url","url":"https://example.com/photo.png"}`) {
		t.Fatalf("remote image not forwarded as an Anthropic url source: %s", string(encoded))
	}
	// The base64 form of the same message stays base64 (regression guard for the
	// branch that already worked).
	dataURIReq := &model.ChatCompletionRequest{
		Model: "claude-3-5-sonnet",
		Messages: []model.ChatMessage{{Role: "user", Content: []any{
			map[string]any{"type": "image_url", "image_url": map[string]any{"url": "data:image/png;base64,iVBORw0KGgo="}},
		}}},
	}
	dataEncoded, err := json.Marshal(convertOpenAIToAnthropic(dataURIReq, &model.ChannelConfig{Name: "claude"}))
	if err != nil {
		t.Fatalf("marshal data URI: %v", err)
	}
	if !strings.Contains(string(dataEncoded), `"type":"base64"`) || !strings.Contains(string(dataEncoded), `"data":"iVBORw0KGgo="`) {
		t.Fatalf("data URI image is no longer inlined as base64: %s", string(dataEncoded))
	}
}

func TestGeminiConvertRejectsRemoteImageWithoutMIME(t *testing.T) {
	_, err := convertOpenAIToGemini(urlImageChatRequest())
	var unsupported *UnsupportedTranslationError
	if !errors.As(err, &unsupported) {
		t.Fatalf("remote image without a trustworthy MIME type must reject translation: %v", err)
	}

	dataURIReq := &model.ChatCompletionRequest{
		Model: "gemini-2.5-flash",
		Messages: []model.ChatMessage{{Role: "user", Content: []any{
			map[string]any{"type": "image_url", "image_url": map[string]any{"url": "data:image/png;base64,iVBORw0KGgo="}},
		}}},
	}
	converted, err := convertOpenAIToGemini(dataURIReq)
	if err != nil {
		t.Fatal(err)
	}
	dataEncoded, err := json.Marshal(converted)
	if err != nil {
		t.Fatalf("marshal data URI: %v", err)
	}
	if !strings.Contains(string(dataEncoded), `"inlineData":{"mimeType":"image/png","data":"iVBORw0KGgo="}`) {
		t.Fatalf("data URI image is no longer inlined: %s", string(dataEncoded))
	}
}
