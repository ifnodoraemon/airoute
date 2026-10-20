package model

import (
	"encoding/json"
	"testing"
)

func TestExtractAnthropicContentString(t *testing.T) {
	// 1. Plain string
	if s := ExtractAnthropicContentString("Hello world"); s != "Hello world" {
		t.Fatalf("expected 'Hello world', got %q", s)
	}

	// 2. Nil
	if s := ExtractAnthropicContentString(nil); s != "" {
		t.Fatalf("expected empty string, got %q", s)
	}

	// 3. Array of content blocks (with cache_control)
	blocks := []any{
		map[string]any{
			"type":          "text",
			"text":          "Instruction block 1",
			"cache_control": map[string]any{"type": "ephemeral"},
		},
		map[string]any{
			"type": "text",
			"text": "Instruction block 2",
		},
	}
	expected := "Instruction block 1\n\nInstruction block 2"
	if s := ExtractAnthropicContentString(blocks); s != expected {
		t.Fatalf("expected %q, got %q", expected, s)
	}

	// 4. Array of strings
	strArr := []string{"Line 1", "Line 2"}
	if s := ExtractAnthropicContentString(strArr); s != "Line 1\n\nLine 2" {
		t.Fatalf("expected 'Line 1\\n\\nLine 2', got %q", s)
	}

	// 5. Array of any containing strings
	anyStrArr := []any{"Part A", "Part B"}
	if s := ExtractAnthropicContentString(anyStrArr); s != "Part A\n\nPart B" {
		t.Fatalf("expected 'Part A\\n\\nPart B', got %q", s)
	}

	// 6. JSON unmarshaled request with array system
	rawJSON := `{
		"model": "claude-3-7-sonnet",
		"system": [
			{"type": "text", "text": "Deepseek system prompt", "cache_control": {"type": "ephemeral"}}
		],
		"messages": [{"role": "user", "content": "Hi"}],
		"max_tokens": 100
	}`
	var req AnthropicInboundRequest
	if err := json.Unmarshal([]byte(rawJSON), &req); err != nil {
		t.Fatalf("failed to unmarshal JSON into AnthropicInboundRequest: %v", err)
	}
	if req.GetSystemPrompt() != "Deepseek system prompt" {
		t.Fatalf("expected 'Deepseek system prompt', got %q", req.GetSystemPrompt())
	}
}
