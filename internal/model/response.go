package model

import (
	"fmt"
)

// ResponseRequest represents the OpenAI Responses API (/v1/responses) request payload.
type ResponseRequest struct {
	Model              string            `json:"model"`
	Input              any               `json:"input"`                  // string or []ChatMessage or []map[string]any
	Instructions       string            `json:"instructions,omitempty"` // system instructions
	PreviousResponseID string            `json:"previous_response_id,omitempty"`
	User               string            `json:"user,omitempty"`
	Stream             bool              `json:"stream,omitempty"`
	Temperature        *float64          `json:"temperature,omitempty"`
	TopP               *float64          `json:"top_p,omitempty"`
	MaxOutputTokens    int               `json:"max_output_tokens,omitempty"`
	Tools              []any             `json:"tools,omitempty"`
	ToolChoice         any               `json:"tool_choice,omitempty"`
	ParallelToolCalls  *bool             `json:"parallel_tool_calls,omitempty"`
	Metadata           map[string]string `json:"metadata,omitempty"`
}

// ResponseContentPart represents a part of the content in a message item.
type ResponseContentPart struct {
	Type string `json:"type"` // "output_text" or "text"
	Text string `json:"text"`
}

// ResponseOutputItem represents an item in the response output array.
type ResponseOutputItem struct {
	ID      string                `json:"id"`
	Type    string                `json:"type"` // "message" or "function_call"
	Status  string                `json:"status"`
	Role    string                `json:"role,omitempty"`
	Content []ResponseContentPart `json:"content,omitempty"`

	// function_call fields (empty for message items).
	CallID    string `json:"call_id,omitempty"`
	Name      string `json:"name,omitempty"`
	Arguments string `json:"arguments,omitempty"`
}

// ResponseInputTokensDetails is the documented input-token breakdown.
type ResponseInputTokensDetails struct {
	CachedTokens int `json:"cached_tokens"`
}

// ResponseOutputTokensDetails is the documented output-token breakdown.
type ResponseOutputTokensDetails struct {
	ReasoningTokens int `json:"reasoning_tokens"`
}

// ResponseUsage represents token consumption in a ResponseResult.
//
// The detail blocks keep the unary payload shaped like the streaming one (which
// always emitted input_tokens_details): without them the same request reported a
// cached-token breakdown when streamed and dropped it when not.
type ResponseUsage struct {
	InputTokens         int                          `json:"input_tokens"`
	InputTokensDetails  *ResponseInputTokensDetails  `json:"input_tokens_details,omitempty"`
	OutputTokens        int                          `json:"output_tokens"`
	OutputTokensDetails *ResponseOutputTokensDetails `json:"output_tokens_details,omitempty"`
	TotalTokens         int                          `json:"total_tokens"`
}

// ResponseIncompleteDetails explains why a response is not complete.
type ResponseIncompleteDetails struct {
	Reason string `json:"reason"` // "max_output_tokens" or "content_filter"
}

// ResponseResult represents the standard OpenAI Responses API response payload.
type ResponseResult struct {
	ID                string                     `json:"id"`
	Object            string                     `json:"object"` // "response"
	CreatedAt         int64                      `json:"created_at"`
	Status            string                     `json:"status"` // "completed", "incomplete", ...
	IncompleteDetails *ResponseIncompleteDetails `json:"incomplete_details,omitempty"`
	Model             string                     `json:"model"`
	Output            []ResponseOutputItem       `json:"output"`
	Usage             ResponseUsage              `json:"usage"`
}

// Response status values.
const (
	ResponseStatusCompleted  = "completed"
	ResponseStatusIncomplete = "incomplete"
)

// responseStatusFromFinishReason maps an upstream chat finish_reason onto the
// Responses API status. The Responses protocol distinguishes a truncated answer
// (status "incomplete" + incomplete_details.reason) from a normal end, which is
// what lets an agent decide whether to continue or retry; a chat finish_reason is
// the only signal the gateway receives.
func responseStatusFromFinishReason(finishReason string) (string, *ResponseIncompleteDetails) {
	switch finishReason {
	case "length":
		return ResponseStatusIncomplete, &ResponseIncompleteDetails{Reason: "max_output_tokens"}
	case "content_filter":
		return ResponseStatusIncomplete, &ResponseIncompleteDetails{Reason: "content_filter"}
	default:
		return ResponseStatusCompleted, nil
	}
}

// IncompleteResponseStatus is the exported helper the streaming emitter uses so
// the unary and streaming paths agree on the mapping.
func IncompleteResponseStatus(finishReason string) (string, *ResponseIncompleteDetails) {
	return responseStatusFromFinishReason(finishReason)
}

// ToChatCompletionRequest converts a ResponseRequest into a canonical ChatCompletionRequest.
func (r *ResponseRequest) ToChatCompletionRequest() (*ChatCompletionRequest, error) {
	var maxTokens *int
	if r.MaxOutputTokens > 0 {
		maxTokens = &r.MaxOutputTokens
	}

	chatTools := responseToolsToChatTools(r.Tools)
	chatToolChoice := responseToolChoiceToChatToolChoice(r.ToolChoice)
	if len(chatTools) == 0 {
		// Built-in Responses tools (web_search, ...) have no chat equivalent. If
		// none of the requested tools survived the conversion, forwarding
		// tool_choice:"required" to an upstream that received no tools is a
		// guaranteed 400; drop the selector instead.
		chatToolChoice = nil
	}
	chatReq := &ChatCompletionRequest{
		Model:             r.Model,
		Stream:            r.Stream,
		Temperature:       r.Temperature,
		TopP:              r.TopP,
		MaxTokens:         maxTokens,
		Tools:             chatTools,
		ToolChoice:        chatToolChoice,
		ParallelToolCalls: r.ParallelToolCalls,
	}

	var messages []ChatMessage

	if r.Instructions != "" {
		messages = append(messages, ChatMessage{
			Role:    "system",
			Content: r.Instructions,
		})
	}

	inputMessages, err := ResponseInputToChatMessages(r.Input)
	if err != nil {
		return nil, err
	}
	messages = append(messages, inputMessages...)

	if len(messages) == 0 {
		return nil, fmt.Errorf("request must specify non-empty 'input'")
	}

	chatReq.Messages = messages
	return chatReq, nil
}
