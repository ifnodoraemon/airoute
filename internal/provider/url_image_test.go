package provider

import (
	"encoding/json"
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

func TestGeminiConvertForwardsRemoteImageAsFileData(t *testing.T) {
	encoded, err := json.Marshal(convertOpenAIToGemini(urlImageChatRequest()))
	if err != nil {
		t.Fatalf("marshal gemini: %v", err)
	}
	if !strings.Contains(string(encoded), `"fileData":{"mimeType":"image/jpeg","fileUri":"https://example.com/photo.png"}`) {
		t.Fatalf("remote image not forwarded as Gemini fileData: %s", string(encoded))
	}
	if strings.Contains(string(encoded), `"inlineData":{"mimeType":"image/jpeg","data":"https://`) {
		t.Fatalf("remote URL was written into inlineData.data again: %s", string(encoded))
	}

	dataURIReq := &model.ChatCompletionRequest{
		Model: "gemini-2.5-flash",
		Messages: []model.ChatMessage{{Role: "user", Content: []any{
			map[string]any{"type": "image_url", "image_url": map[string]any{"url": "data:image/png;base64,iVBORw0KGgo="}},
		}}},
	}
	dataEncoded, err := json.Marshal(convertOpenAIToGemini(dataURIReq))
	if err != nil {
		t.Fatalf("marshal data URI: %v", err)
	}
	if !strings.Contains(string(dataEncoded), `"inlineData":{"mimeType":"image/png","data":"iVBORw0KGgo="}`) {
		t.Fatalf("data URI image is no longer inlined: %s", string(dataEncoded))
	}
}
