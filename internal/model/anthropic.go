package model

import (
	"encoding/json"
	"strings"
)

// AnthropicInboundMessage represents an inbound message from Claude SDK.
type AnthropicInboundMessage struct {
	Role    string `json:"role"`
	Content any    `json:"content"` // can be string or content blocks
}

// AnthropicInboundRequest represents the incoming payload from Anthropic SDK.
// Matches Anthropic Messages API specification including multi-part system prompts,
// stop sequences, thinking budget, and tool choices.
type AnthropicInboundRequest struct {
	Model         string                    `json:"model"`
	Messages      []AnthropicInboundMessage `json:"messages"`
	System        any                       `json:"system,omitempty"` // can be string, []any (content blocks), or []string
	MaxTokens     int                       `json:"max_tokens"`
	Temperature   *float64                  `json:"temperature,omitempty"`
	TopP          *float64                  `json:"top_p,omitempty"`
	TopK          *int                      `json:"top_k,omitempty"`
	Stream        bool                      `json:"stream,omitempty"`
	Tools         []any                     `json:"tools,omitempty"`
	ToolChoice    any                       `json:"tool_choice,omitempty"`
	StopSequences []string                  `json:"stop_sequences,omitempty"`
	Thinking      any                       `json:"thinking,omitempty"` // e.g. {"type": "enabled", "budget_tokens": 1024}
	Metadata      any                       `json:"metadata,omitempty"`
}

// ExtractAnthropicContentString extracts a flat string from a string, []any content blocks, or content block maps.
func ExtractAnthropicContentString(content any) string {
	if content == nil {
		return ""
	}
	switch v := content.(type) {
	case string:
		return v
	case []any:
		var parts []string
		for _, item := range v {
			if s, ok := item.(string); ok {
				if trimmed := strings.TrimSpace(s); trimmed != "" {
					parts = append(parts, trimmed)
				}
				continue
			}
			if m, ok := item.(map[string]any); ok {
				// Anthropic content blocks: {"type": "text", "text": "...", "cache_control": ...}
				if t, ok := m["text"].(string); ok && strings.TrimSpace(t) != "" {
					parts = append(parts, t)
				}
			}
		}
		return strings.Join(parts, "\n\n")
	case []string:
		var parts []string
		for _, s := range v {
			if trimmed := strings.TrimSpace(s); trimmed != "" {
				parts = append(parts, trimmed)
			}
		}
		return strings.Join(parts, "\n\n")
	case map[string]any:
		if t, ok := v["text"].(string); ok {
			return t
		}
		b, _ := json.Marshal(v)
		return string(b)
	default:
		return ""
	}
}

// GetSystemPrompt safely extracts the system prompt as a string from System field,
// supporting both plain string and structured content blocks array.
func (r *AnthropicInboundRequest) GetSystemPrompt() string {
	if r == nil {
		return ""
	}
	return ExtractAnthropicContentString(r.System)
}
