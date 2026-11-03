package adapter

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/ifnodoraemon/airoute/internal/model"
	"github.com/ifnodoraemon/airoute/internal/provider"
)

// F-07 regression (inbound): the Anthropic Messages API accepts a remote image as
// source.type="url". Dropping that block made the model answer a question about a
// picture it never received.
func TestAnthropicAdapterKeepsURLSourceImage(t *testing.T) {
	inbound := &model.AnthropicInboundRequest{
		Model:     "claude-3-5-sonnet",
		MaxTokens: 64,
		Messages: []model.AnthropicInboundMessage{{
			Role: "user",
			Content: []any{
				map[string]any{"type": "text", "text": "what is in this image?"},
				map[string]any{"type": "image", "source": map[string]any{
					"type": "url",
					"url":  "https://example.com/photo.png",
				}},
			},
		}},
	}

	canonical, err := NewAnthropicAdapter().ToCanonical(context.Background(), inbound)
	if err != nil {
		t.Fatalf("ToCanonical: %v", err)
	}
	if len(canonical.Messages) != 1 {
		t.Fatalf("expected one canonical message, got %d", len(canonical.Messages))
	}
	parts := model.ParseMessageContent(canonical.Messages[0].Content)
	var image *model.ContentPart
	for i := range parts {
		if parts[i].Type == model.ContentPartImageURL {
			image = &parts[i]
		}
	}
	if image == nil || image.ImageURL == nil {
		t.Fatalf("URL-source image was dropped: %s", mustJSON(canonical.Messages[0].Content))
	}
	if image.ImageURL.URL != "https://example.com/photo.png" {
		t.Fatalf("image URL = %q", image.ImageURL.URL)
	}

	// The base64 form still becomes an inline data URI.
	base64Inbound := &model.AnthropicInboundRequest{
		Model:     "claude-3-5-sonnet",
		MaxTokens: 64,
		Messages: []model.AnthropicInboundMessage{{
			Role: "user",
			Content: []any{
				map[string]any{"type": "image", "source": map[string]any{
					"type": "base64", "media_type": "image/png", "data": "iVBORw0KGgo=",
				}},
			},
		}},
	}
	base64Canonical, err := NewAnthropicAdapter().ToCanonical(context.Background(), base64Inbound)
	if err != nil {
		t.Fatalf("ToCanonical(base64): %v", err)
	}
	if !strings.Contains(mustJSON(base64Canonical.Messages[0].Content), "data:image/png;base64,iVBORw0KGgo=") {
		t.Fatalf("base64 image form changed: %s", mustJSON(base64Canonical.Messages[0].Content))
	}
}

// A Gemini fileData part is a remote URI; it must survive the inbound conversion
// as a URL so the outbound side can forward it as a URL (F-07).
func TestGeminiAdapterKeepsFileDataURI(t *testing.T) {
	req := &provider.GeminiRequest{
		Contents: []provider.GeminiContent{{
			Role: "user",
			Parts: []provider.GeminiPart{
				{Text: "describe"},
				{FileData: &provider.GeminiFileData{MimeType: "image/png", FileURI: "https://example.com/photo.png"}},
			},
		}},
	}
	canonical, err := NewGeminiAdapter().ToCanonical(context.Background(), req)
	if err != nil {
		t.Fatalf("ToCanonical: %v", err)
	}
	encoded := mustJSON(canonical.Messages)
	if !strings.Contains(encoded, "https://example.com/photo.png") {
		t.Fatalf("fileData URI was dropped: %s", encoded)
	}
}

func mustJSON(v any) string {
	encoded, err := json.Marshal(v)
	if err != nil {
		return "<marshal error>"
	}
	return string(encoded)
}
