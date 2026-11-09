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
		if tm, ok := t.(map[string]any); ok {
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
	}

	return canonicalTools
}
