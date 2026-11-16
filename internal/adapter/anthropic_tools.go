package adapter

import (
	"encoding/json"
	"strings"

	"github.com/ifnodoraemon/airoute/internal/model"
)

func requiresNativeAnthropic(req *model.AnthropicInboundRequest) bool {
	if req.Thinking != nil || req.TopK != nil || req.Metadata != nil {
		return true
	}
	// Cache-control annotations and signed reasoning cannot survive conversion
	// through a different provider's request schema.
	for _, item := range append(append([]any{req.System}, req.Tools...), messagesAsAny(req.Messages)...) {
		encoded, err := json.Marshal(item)
		if err == nil && (strings.Contains(string(encoded), `"cache_control"`) || strings.Contains(string(encoded), `"signature"`) || strings.Contains(string(encoded), `"redacted_thinking"`)) {
			return true
		}
	}
	// Server-side tools and opaque content blocks have no faithful OpenAI/Gemini
	// representation; route them to a native Anthropic channel instead of
	// silently dropping them.
	for _, t := range req.Tools {
		if tm, ok := t.(map[string]any); ok && isAnthropicServerTool(tm) {
			return true
		}
	}
	for _, msg := range req.Messages {
		if messageRequiresNativeAnthropic(msg) {
			return true
		}
	}
	return false
}

func messagesAsAny(messages []model.AnthropicInboundMessage) []any {
	items := make([]any, 0, len(messages))
	for _, message := range messages {
		items = append(items, message.Content)
	}
	return items
}

func convertAnthropicToolChoice(tc any) any {
	if tc == nil {
		return nil
	}
	if s, ok := tc.(string); ok {
		switch s {
		case "any":
			return "required"
		default:
			return s
		}
	}
	if m, ok := tc.(map[string]any); ok {
		tcType, _ := m["type"].(string)
		switch tcType {
		case "auto":
			return "auto"
		case "any":
			return "required"
		case "none":
			return "none"
		case "tool":
			if name, ok := m["name"].(string); ok {
				return map[string]any{
					"type": "function",
					"function": map[string]any{
						"name": name,
					},
				}
			}
		}
	}
	return tc
}

func anthropicCanonicalTools(req *model.AnthropicInboundRequest) []model.Tool {
	var canonicalTools []model.Tool
	for _, t := range req.Tools {
		tm, ok := t.(map[string]any)
		if !ok || isAnthropicServerTool(tm) {
			continue
		}
		name, _ := tm["name"].(string)
		desc, _ := tm["description"].(string)
		schema, _ := tm["input_schema"].(map[string]any)
		canonicalTools = append(canonicalTools, model.Tool{
			Type: "function",
			Function: map[string]any{
				"name":        name,
				"description": desc,
				"parameters":  schema,
			},
		})
	}
	return canonicalTools
}

// isAnthropicServerTool reports a built-in server tool (web_search, code
// execution, MCP toolsets, ...): it carries a non-custom type and no client
// input_schema, so it cannot become an OpenAI function declaration.
func isAnthropicServerTool(tm map[string]any) bool {
	typ, _ := tm["type"].(string)
	if typ == "" || typ == "custom" {
		return false
	}
	_, hasSchema := tm["input_schema"]
	return !hasSchema
}

// anthropicNativeOnlyBlocks are content blocks with no faithful representation
// in another provider's schema; a request carrying them must stay native.
var anthropicNativeOnlyBlocks = map[string]bool{
	"document": true, "search_result": true, "container_upload": true,
	"server_tool_use": true, "web_search_tool_result": true,
	"mcp_tool_use": true, "mcp_tool_result": true,
}

func messageRequiresNativeAnthropic(msg model.AnthropicInboundMessage) bool {
	blocks, ok := msg.Content.([]any)
	if !ok {
		return false
	}
	for _, b := range blocks {
		bm, ok := b.(map[string]any)
		if !ok {
			continue
		}
		kind, _ := bm["type"].(string)
		if anthropicNativeOnlyBlocks[kind] {
			return true
		}
		if kind == "tool_result" && toolResultRequiresNative(bm) {
			return true
		}
	}
	return false
}

// toolResultRequiresNative flags tool results carrying an error state or images,
// which an OpenAI tool message cannot represent without losing information.
func toolResultRequiresNative(bm map[string]any) bool {
	if isErr, _ := bm["is_error"].(bool); isErr {
		return true
	}
	if arr, ok := bm["content"].([]any); ok {
		for _, item := range arr {
			if im, ok := item.(map[string]any); ok {
				if kind, _ := im["type"].(string); kind == "image" {
					return true
				}
			}
		}
	}
	return false
}

// anthropicParallelToolCalls maps tool_choice.disable_parallel_tool_use onto the
// standard parallel_tool_calls field so the constraint is not silently dropped.
func anthropicParallelToolCalls(tc any) *bool {
	if m, ok := tc.(map[string]any); ok {
		if disable, ok := m["disable_parallel_tool_use"].(bool); ok && disable {
			f := false
			return &f
		}
	}
	return nil
}
