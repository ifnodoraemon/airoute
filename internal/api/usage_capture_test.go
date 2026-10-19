package api

import (
	"encoding/json"
	"testing"

	"github.com/ifnodoraemon/airoute/internal/model"
)

// A usage-only SSE frame (empty choices array) is what OpenAI-compatible
// upstreams send last when stream_options.include_usage is set. It must be
// parsed into the canonical Usage so prompt/cached tokens are recorded.
func TestUsageOnlyChunkParses(t *testing.T) {
	raw := `{"id":"x","object":"chat.completion.chunk","created":1,"model":"m","choices":[],` +
		`"usage":{"prompt_tokens":32,"completion_tokens":28,"total_tokens":60,` +
		`"prompt_tokens_details":{"cached_tokens":16}}}`

	var chunk model.ChatCompletionChunk
	if err := json.Unmarshal([]byte(raw), &chunk); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(chunk.Choices) != 0 {
		t.Fatalf("expected empty choices, got %d", len(chunk.Choices))
	}
	if chunk.Usage == nil {
		t.Fatal("expected usage to be populated on an empty-choices frame")
	}
	if chunk.Usage.PromptTokens != 32 || chunk.Usage.CompletionTokens != 28 {
		t.Fatalf("unexpected usage: %+v", chunk.Usage)
	}
	if got := chunk.Usage.GetCachedTokens(); got != 16 {
		t.Fatalf("cached tokens = %d, want 16", got)
	}
}

// The canonical request built from an Anthropic stream must opt into usage
// reporting, otherwise OpenAI-compatible upstreams never send the final frame.
func TestAnthropicStreamRequestsUsage(t *testing.T) {
	req := &model.AnthropicInboundRequest{
		Model:     "m",
		Stream:    true,
		MaxTokens: 16,
		Messages:  []model.AnthropicInboundMessage{{Role: "user", Content: "hi"}},
	}
	canonical := ConvertAnthropicToCanonical(req)
	if canonical.StreamOptions == nil || !canonical.StreamOptions.IncludeUsage {
		t.Fatalf("expected include_usage to be requested, got %+v", canonical.StreamOptions)
	}

	// Non-streaming requests do not need stream_options.
	req.Stream = false
	canonical = ConvertAnthropicToCanonical(req)
	if canonical.StreamOptions != nil {
		t.Fatalf("expected no stream_options for unary requests, got %+v", canonical.StreamOptions)
	}
}
