package provider

import (
	"encoding/json"
	"testing"

	"github.com/ifnodoraemon/airoute/internal/model"
)

func TestNativeAnthropicRequestPreservesSignedBlocksAndOptions(t *testing.T) {
	topK := 10
	original := &model.AnthropicInboundRequest{
		Model: "public-model", MaxTokens: 2048, TopK: &topK,
		System:     []any{map[string]any{"type": "text", "text": "system", "cache_control": map[string]any{"type": "ephemeral"}}},
		Messages:   []model.AnthropicInboundMessage{{Role: "assistant", Content: []any{map[string]any{"type": "thinking", "thinking": "reason", "signature": "signed-value"}, map[string]any{"type": "text", "text": "answer"}}}},
		Thinking:   map[string]any{"type": "enabled", "budget_tokens": 1024},
		ToolChoice: map[string]any{"type": "auto"},
		Metadata:   map[string]any{"user_id": "user-1"},
	}
	req := &model.ChatCompletionRequest{Model: original.Model, AnthropicOriginal: original}
	channel := &model.ChannelConfig{ModelMapping: map[string]string{"public-model": "upstream-model"}}
	payload, err := json.Marshal(convertOpenAIToAnthropic(req, channel))
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	if err := json.Unmarshal(payload, &got); err != nil {
		t.Fatal(err)
	}
	if got["model"] != "upstream-model" || got["top_k"] != float64(10) || got["metadata"].(map[string]any)["user_id"] != "user-1" {
		t.Fatalf("options lost: %s", payload)
	}
	blocks := got["messages"].([]any)[0].(map[string]any)["content"].([]any)
	if blocks[0].(map[string]any)["signature"] != "signed-value" {
		t.Fatalf("thinking signature lost: %s", payload)
	}
	if got["system"].([]any)[0].(map[string]any)["cache_control"] == nil {
		t.Fatalf("cache control lost: %s", payload)
	}
}
