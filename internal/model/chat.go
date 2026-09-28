package model

import (
	"encoding/json"
)

// ChatMessage represents a single chat completion message.
type ChatMessage struct {
	Role             string          `json:"role"`
	Content          any             `json:"content"` // can be string or structured content
	ReasoningContent string          `json:"reasoning_content,omitempty"`
	Name             string          `json:"name,omitempty"`
	ToolCalls        []ToolCall      `json:"tool_calls,omitempty"`
	ToolCallID       string          `json:"tool_call_id,omitempty"`
}

// GetContentString returns string content safely.
func (m *ChatMessage) GetContentString() string {
	if m.Content == nil {
		return ""
	}
	if s, ok := m.Content.(string); ok {
		return s
	}
	b, _ := json.Marshal(m.Content)
	return string(b)
}

// ToolCall represents a tool invocation.
type ToolCall struct {
	Index    *int         `json:"index,omitempty"`
	ID       string       `json:"id,omitempty"`
	Type     string       `json:"type,omitempty"`
	Function FunctionCall `json:"function"`
}

// FunctionCall represents the function invoked in a tool call.
type FunctionCall struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

// Tool defines a tool the model may call.
type Tool struct {
	Type     string         `json:"type"`
	Function map[string]any `json:"function"`
}

// StreamOptions controls streaming behavior (e.g., usage reporting).
type StreamOptions struct {
	IncludeUsage bool `json:"include_usage,omitempty"`
}

// ChatCompletionRequest is the standard OpenAI chat completion request payload.
type ChatCompletionRequest struct {
	Model            string         `json:"model"`
	Messages         []ChatMessage  `json:"messages"`
	Temperature      *float64       `json:"temperature,omitempty"`
	TopP             *float64       `json:"top_p,omitempty"`
	N                *int           `json:"n,omitempty"`
	Stream           bool           `json:"stream,omitempty"`
	StreamOptions    *StreamOptions `json:"stream_options,omitempty"`
	Stop             any            `json:"stop,omitempty"`
	MaxTokens        *int           `json:"max_tokens,omitempty"`
	PresencePenalty  *float64       `json:"presence_penalty,omitempty"`
	FrequencyPenalty *float64       `json:"frequency_penalty,omitempty"`
	User             string         `json:"user,omitempty"`
	Tools            []Tool         `json:"tools,omitempty"`
	ToolChoice       any            `json:"tool_choice,omitempty"`
}

// PromptTokensDetails provides token breakdown details including cache hits.
type PromptTokensDetails struct {
	CachedTokens int `json:"cached_tokens,omitempty"`
	AudioTokens  int `json:"audio_tokens,omitempty"`
}

// Usage reports token usage for the request.
type Usage struct {
	PromptTokens        int                  `json:"prompt_tokens"`
	CompletionTokens    int                  `json:"completion_tokens"`
	TotalTokens         int                  `json:"total_tokens"`
	PromptTokensDetails *PromptTokensDetails `json:"prompt_tokens_details,omitempty"`
}

// GetCachedTokens returns the cached prompt tokens if available.
func (u *Usage) GetCachedTokens() int {
	if u == nil || u.PromptTokensDetails == nil {
		return 0
	}
	return u.PromptTokensDetails.CachedTokens
}

// ChatCompletionChoice is an individual completion choice.
type ChatCompletionChoice struct {
	Index        int         `json:"index"`
	Message      ChatMessage `json:"message"`
	FinishReason *string     `json:"finish_reason"`
}

// ChatCompletionResponse is the standard OpenAI non-streaming response.
type ChatCompletionResponse struct {
	ID      string                 `json:"id"`
	Object  string                 `json:"object"`
	Created int64                  `json:"created"`
	Model   string                 `json:"model"`
	Choices []ChatCompletionChoice `json:"choices"`
	Usage   *Usage                 `json:"usage,omitempty"`
}

// ChunkDelta is the incremental message delta in a stream.
type ChunkDelta struct {
	Role             string     `json:"role,omitempty"`
	Content          string     `json:"content,omitempty"`
	ReasoningContent string     `json:"reasoning_content,omitempty"`
	ToolCalls        []ToolCall `json:"tool_calls,omitempty"`
}

// ChunkChoice is an individual choice in a streaming chunk.
type ChunkChoice struct {
	Index        int        `json:"index"`
	Delta        ChunkDelta `json:"delta"`
	FinishReason *string    `json:"finish_reason"`
}

// ChatCompletionChunk is the standard OpenAI SSE streaming chunk.
type ChatCompletionChunk struct {
	ID      string        `json:"id"`
	Object  string        `json:"object"`
	Created int64         `json:"created"`
	Model   string        `json:"model"`
	Choices []ChunkChoice `json:"choices"`
	Usage   *Usage        `json:"usage,omitempty"`
}

// ModelItem represents a model in the /v1/models response.
type ModelItem struct {
	ID      string `json:"id"`
	Object  string `json:"object"`
	Created int64  `json:"created"`
	OwnedBy string `json:"owned_by"`
}

// ModelListResponse represents the /v1/models response.
type ModelListResponse struct {
	Object string      `json:"object"`
	Data   []ModelItem `json:"data"`
}

// StreamEvent is an internal event passed over channels during streaming.
type StreamEvent struct {
	Chunk  *ChatCompletionChunk
	Raw    []byte
	IsDone bool
	Err    error
}

