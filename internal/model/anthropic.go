package model

// AnthropicInboundMessage represents an inbound message from Claude SDK.
type AnthropicInboundMessage struct {
	Role    string `json:"role"`
	Content any    `json:"content"` // can be string or content blocks
}

// AnthropicInboundRequest represents the incoming payload from Anthropic SDK.
type AnthropicInboundRequest struct {
	Model       string                    `json:"model"`
	Messages    []AnthropicInboundMessage `json:"messages"`
	System      string                    `json:"system,omitempty"`
	MaxTokens   int                       `json:"max_tokens"`
	Temperature *float64                  `json:"temperature,omitempty"`
	TopP        *float64                  `json:"top_p,omitempty"`
	Stream      bool                      `json:"stream,omitempty"`
	Tools       []any                     `json:"tools,omitempty"`
}
