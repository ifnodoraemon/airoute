package provider

import "encoding/json"

// AnthropicTool represents a tool definition in Claude Messages API.
type AnthropicTool struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	InputSchema any    `json:"input_schema"`
}

// AnthropicRequest represents the Anthropic Messages API payload.
type AnthropicRequest struct {
	Model         string             `json:"model"`
	Messages      []AnthropicMessage `json:"messages"`
	System        any                `json:"system,omitempty"`
	MaxTokens     int                `json:"max_tokens"`
	Temperature   *float64           `json:"temperature,omitempty"`
	TopP          *float64           `json:"top_p,omitempty"`
	Stream        bool               `json:"stream,omitempty"`
	Tools         []any              `json:"tools,omitempty"`
	ToolChoice    any                `json:"tool_choice,omitempty"`
	StopSequences []string           `json:"stop_sequences,omitempty"`
	TopK          *int               `json:"top_k,omitempty"`
	Thinking      any                `json:"thinking,omitempty"`
	Metadata      any                `json:"metadata,omitempty"`
}

// AnthropicMessage represents a turn in Claude Messages API.
type AnthropicMessage struct {
	Role    string `json:"role"`
	Content any    `json:"content"` // can be string or []any of content blocks
}

// AnthropicContentBlock represents a multimodal or tool use content block.
type AnthropicContentBlock struct {
	Type  string          `json:"type"`
	Text  string          `json:"text,omitempty"`
	ID    string          `json:"id,omitempty"`
	Name  string          `json:"name,omitempty"`
	Input json.RawMessage `json:"input,omitempty"`
}

// AnthropicUsage represents token consumption stats.
// InputTokens excludes prompt-cache reads/creations; those are reported separately
// and must be added to obtain the total prompt token count.
type AnthropicUsage struct {
	InputTokens              int `json:"input_tokens"`
	OutputTokens             int `json:"output_tokens"`
	CacheReadInputTokens     int `json:"cache_read_input_tokens,omitempty"`
	CacheCreationInputTokens int `json:"cache_creation_input_tokens,omitempty"`
}

// AnthropicResponse represents Claude Messages API non-streaming response.
type AnthropicResponse struct {
	ID         string                  `json:"id"`
	Type       string                  `json:"type"`
	Role       string                  `json:"role"`
	Content    []AnthropicContentBlock `json:"content"`
	Model      string                  `json:"model"`
	StopReason string                  `json:"stop_reason"`
	Usage      AnthropicUsage          `json:"usage"`
}
