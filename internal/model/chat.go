package model

import (
	"encoding/json"
)

// ChatMessage represents a single chat completion message.
type ChatMessage struct {
	Role             string     `json:"role"`
	Content          any        `json:"content"` // can be string or structured content
	ReasoningContent string     `json:"reasoning_content,omitempty"`
	Name             string     `json:"name,omitempty"`
	ToolCalls        []ToolCall `json:"tool_calls,omitempty"`
	ToolCallID       string     `json:"tool_call_id,omitempty"`
	// Extra preserves JSON fields this struct does not model, so they survive a
	// same-protocol round trip instead of being silently dropped.
	Extra ExtraFields `json:"-"`
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
	// ParallelToolCalls is a standard chat-completions field (and a Responses
	// field the adapter maps); keeping it modeled means P0-01 passthrough and
	// the Responses conversion agree on who owns the key.
	ParallelToolCalls *bool `json:"parallel_tool_calls,omitempty"`

	// Protocol specifies the desired routing protocol (e.g. "openai_text", "openai_chat"). Internal metadata; never serialized.
	Protocol string `json:"-"`
	// AnthropicOriginal preserves native-only fields and signed content blocks when
	// a Messages request is routed to another Anthropic endpoint.
	AnthropicOriginal   *AnthropicInboundRequest `json:"-"`
	AnthropicNativeOnly bool                     `json:"-"`
	AnthropicBeta       string                   `json:"-"`
	// Extra preserves unmodeled top-level fields (response_format, seed,
	// logprobs, parallel_tool_calls, reasoning_effort, service_tier, ...).
	Extra ExtraFields `json:"-"`
}

// PromptTokensDetails provides token breakdown details including cache hits.
type PromptTokensDetails struct {
	CachedTokens int `json:"cached_tokens,omitempty"`
	AudioTokens  int `json:"audio_tokens,omitempty"`
	// CacheCreationTokens is how many of PromptTokens were written into the
	// provider's prompt cache this request (Anthropic's
	// cache_creation_input_tokens). They stay part of PromptTokens, because that
	// is how the provider invoices them; exposing the write count separately is
	// what lets billing apply the cache-write premium instead of the plain input
	// rate (C-02).
	CacheCreationTokens int `json:"cache_creation_tokens,omitempty"`
	// Extra preserves unmodeled token-detail fields.
	Extra ExtraFields `json:"-"`
}

// Usage reports token usage for the request.
type Usage struct {
	PromptTokens        int                  `json:"prompt_tokens"`
	CompletionTokens    int                  `json:"completion_tokens"`
	TotalTokens         int                  `json:"total_tokens"`
	PromptTokensDetails *PromptTokensDetails `json:"prompt_tokens_details,omitempty"`
	// Extra preserves unmodeled usage fields (completion_tokens_details, ...).
	Extra ExtraFields `json:"-"`
}

// GetCachedTokens returns the cached prompt tokens if available.
func (u *Usage) GetCachedTokens() int {
	if u == nil || u.PromptTokensDetails == nil {
		return 0
	}
	return u.PromptTokensDetails.CachedTokens
}

// GetCacheCreationTokens returns how many prompt tokens were written to the
// provider's prompt cache this request, if the upstream reported it. The count is
// a subset of PromptTokens, not an addition to it.
func (u *Usage) GetCacheCreationTokens() int {
	if u == nil || u.PromptTokensDetails == nil {
		return 0
	}
	return u.PromptTokensDetails.CacheCreationTokens
}

// GetReasoningTokens returns the reasoning/thinking tokens the upstream reported
// inside completion_tokens_details, if any.
//
// CompletionTokens already includes them (that is how the provider bills), so this
// is a breakdown for clients — the Responses API echoes it as
// output_tokens_details.reasoning_tokens. The value survives as an unmodeled
// usage member (Usage.Extra), which is why reading it here is enough to stop
// /v1/responses from always reporting zero (F-02).
func (u *Usage) GetReasoningTokens() int {
	if u == nil || len(u.Extra) == 0 {
		return 0
	}
	raw, ok := u.Extra["completion_tokens_details"]
	if !ok {
		return 0
	}
	var details struct {
		ReasoningTokens int `json:"reasoning_tokens"`
	}
	if err := json.Unmarshal(raw, &details); err != nil {
		return 0
	}
	return details.ReasoningTokens
}

// ChatCompletionChoice is an individual completion choice.
type ChatCompletionChoice struct {
	Index        int         `json:"index"`
	Message      ChatMessage `json:"message"`
	FinishReason *string     `json:"finish_reason"`
	// Extra preserves unmodeled choice fields (logprobs, ...).
	Extra ExtraFields `json:"-"`
}

// ChatCompletionResponse is the standard OpenAI non-streaming response.
type ChatCompletionResponse struct {
	ID      string                 `json:"id"`
	Object  string                 `json:"object"`
	Created int64                  `json:"created"`
	Model   string                 `json:"model"`
	Choices []ChatCompletionChoice `json:"choices"`
	Usage   *Usage                 `json:"usage,omitempty"`

	// Channel is the name of the upstream channel that actually served this
	// request. Internal-only metadata for audit logging; never serialized.
	Channel string `json:"-"`
	// NativeAnthropicResponse is used only for native Messages ingress/egress.
	NativeAnthropicResponse json.RawMessage `json:"-"`
	// Extra preserves unmodeled response fields (system_fingerprint,
	// service_tier, prompt_filter_results, ...).
	Extra ExtraFields `json:"-"`
}

// ChunkDelta is the incremental message delta in a stream.
type ChunkDelta struct {
	Role             string     `json:"role,omitempty"`
	Content          string     `json:"content,omitempty"`
	ReasoningContent string     `json:"reasoning_content,omitempty"`
	ToolCalls        []ToolCall `json:"tool_calls,omitempty"`
	// Extra preserves unmodeled delta fields (refusal, audio, function_call, ...).
	Extra ExtraFields `json:"-"`
}

// ChunkChoice is an individual choice in a streaming chunk.
type ChunkChoice struct {
	Index        int        `json:"index"`
	Delta        ChunkDelta `json:"delta"`
	FinishReason *string    `json:"finish_reason"`
	// Extra preserves unmodeled choice fields (logprobs, ...).
	Extra ExtraFields `json:"-"`
}

// ChatCompletionChunk is the standard OpenAI SSE streaming chunk.
type ChatCompletionChunk struct {
	ID      string        `json:"id"`
	Object  string        `json:"object"`
	Created int64         `json:"created"`
	Model   string        `json:"model"`
	Choices []ChunkChoice `json:"choices"`
	Usage   *Usage        `json:"usage,omitempty"`
	// Extra preserves unmodeled chunk fields (system_fingerprint,
	// service_tier, prompt_filter_results, ...).
	Extra ExtraFields `json:"-"`
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
	Chunk                *ChatCompletionChunk
	Raw                  []byte
	NativeAnthropicEvent []byte
	NativeAnthropicType  string
	// Channel is the name of the upstream channel serving the stream.
	// Populated by the dispatcher on the first event; internal-only.
	Channel string
	IsDone  bool
	Err     error
}
