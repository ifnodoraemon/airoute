package adapter

import (
	"testing"

	"github.com/ifnodoraemon/airoute/internal/model"
)

func TestConvertAnthropicToolChoiceNone(t *testing.T) {
	if got := convertAnthropicToolChoice(map[string]any{"type": "none"}); got != "none" {
		t.Fatalf("none object = %#v, want \"none\"", got)
	}
	if got := convertAnthropicToolChoice("none"); got != "none" {
		t.Fatalf("none string = %#v, want \"none\"", got)
	}
}

func TestAnthropicDisableParallelToolUse(t *testing.T) {
	p := anthropicParallelToolCalls(map[string]any{"type": "auto", "disable_parallel_tool_use": true})
	if p == nil || *p != false {
		t.Fatalf("parallel_tool_calls = %#v, want false", p)
	}
	if anthropicParallelToolCalls(map[string]any{"type": "auto"}) != nil {
		t.Fatal("parallel_tool_calls should be nil without the constraint")
	}
}

func TestRequiresNativeAnthropicServerToolsAndBlocks(t *testing.T) {
	msg := func(blocks ...any) []model.AnthropicInboundMessage {
		return []model.AnthropicInboundMessage{{Role: "user", Content: blocks}}
	}
	cases := []struct {
		name string
		req  *model.AnthropicInboundRequest
		want bool
	}{
		{"plain text", &model.AnthropicInboundRequest{Messages: []model.AnthropicInboundMessage{{Role: "user", Content: "hi"}}}, false},
		{"server tool", &model.AnthropicInboundRequest{Tools: []any{map[string]any{"type": "web_search_20250305", "name": "web_search", "max_uses": 5}}}, true},
		{"client tool", &model.AnthropicInboundRequest{Tools: []any{map[string]any{"name": "f", "input_schema": map[string]any{"type": "object"}}}}, false},
		{"document block", &model.AnthropicInboundRequest{Messages: msg(map[string]any{"type": "document", "source": map[string]any{"type": "base64", "media_type": "application/pdf", "data": "x"}})}, true},
		{"tool_result error", &model.AnthropicInboundRequest{Messages: msg(map[string]any{"type": "tool_result", "tool_use_id": "t", "is_error": true, "content": "boom"})}, true},
		{"tool_result image", &model.AnthropicInboundRequest{Messages: msg(map[string]any{"type": "tool_result", "tool_use_id": "t", "content": []any{map[string]any{"type": "image", "source": map[string]any{}}}})}, true},
		{"tool_result text", &model.AnthropicInboundRequest{Messages: msg(map[string]any{"type": "tool_result", "tool_use_id": "t", "content": "ok"})}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := requiresNativeAnthropic(tc.req); got != tc.want {
				t.Fatalf("requiresNativeAnthropic = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestAnthropicCanonicalToolsSkipsServerTools(t *testing.T) {
	req := &model.AnthropicInboundRequest{Tools: []any{
		map[string]any{"type": "web_search_20250305", "name": "web_search"},
		map[string]any{"name": "get_weather", "description": "d", "input_schema": map[string]any{"type": "object"}},
	}}
	tools := anthropicCanonicalTools(req)
	if len(tools) != 1 {
		t.Fatalf("canonical tools = %d, want 1 (server tool skipped)", len(tools))
	}
	if name, _ := tools[0].Function["name"].(string); name != "get_weather" {
		t.Fatalf("kept tool = %v, want get_weather", tools[0].Function)
	}
	if tools[0].Function["parameters"] == nil {
		t.Fatal("client tool parameters lost")
	}
}

func TestAnthropicToCanonicalCarriesParallelFlag(t *testing.T) {
	req := &model.AnthropicInboundRequest{
		Model: "m", MaxTokens: 16,
		ToolChoice: map[string]any{"type": "auto", "disable_parallel_tool_use": true},
		Messages:   []model.AnthropicInboundMessage{{Role: "user", Content: "hi"}},
	}
	c, err := AnthropicToCanonical(req)
	if err != nil {
		t.Fatal(err)
	}
	if c.ParallelToolCalls == nil || *c.ParallelToolCalls != false {
		t.Fatalf("ParallelToolCalls = %#v, want false", c.ParallelToolCalls)
	}
}