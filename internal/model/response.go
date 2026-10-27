package model

import (
	"encoding/json"
	"fmt"
	"time"
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

// ResponseUsage represents token consumption in a ResponseResult.
type ResponseUsage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
	TotalTokens  int `json:"total_tokens"`
}

// ResponseResult represents the standard OpenAI Responses API response payload.
type ResponseResult struct {
	ID        string               `json:"id"`
	Object    string               `json:"object"` // "response"
	CreatedAt int64                `json:"created_at"`
	Status    string               `json:"status"` // "completed"
	Model     string               `json:"model"`
	Output    []ResponseOutputItem `json:"output"`
	Usage     ResponseUsage        `json:"usage"`
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

	if r.Input != nil {
		switch v := r.Input.(type) {
		case string:
			messages = append(messages, ChatMessage{
				Role:    "user",
				Content: v,
			})
		case []any:
			for _, item := range v {
				itemBytes, _ := json.Marshal(item)
				var msg ChatMessage
				if err := json.Unmarshal(itemBytes, &msg); err == nil && msg.Role != "" {
					// A Responses input item carries Responses-specific members
					// (type/id/status). They are not valid chat message fields and
					// must not leak into the chat request we build from it.
					msg.Extra = nil
					messages = append(messages, msg)
				} else {
					var rawMap map[string]any
					if err := json.Unmarshal(itemBytes, &rawMap); err == nil {
						role, _ := rawMap["role"].(string)
						if role == "" {
							role = "user"
						}
						contentStr := ""
						if cStr, ok := rawMap["content"].(string); ok {
							contentStr = cStr
						} else if textStr, ok := rawMap["text"].(string); ok {
							contentStr = textStr
						}
						messages = append(messages, ChatMessage{
							Role:    role,
							Content: contentStr,
						})
					}
				}
			}
		default:
			// Fallback: serialize to string
			b, _ := json.Marshal(v)
			messages = append(messages, ChatMessage{
				Role:    "user",
				Content: string(b),
			})
		}
	}

	if len(messages) == 0 {
		return nil, fmt.Errorf("request must specify non-empty 'input'")
	}

	chatReq.Messages = messages
	return chatReq, nil
}

// ConvertChatResponseToResponseResult converts a ChatCompletionResponse into a ResponseResult.
func ConvertChatResponseToResponseResult(chatResp *ChatCompletionResponse) *ResponseResult {
	if chatResp == nil {
		return nil
	}

	resID := chatResp.ID
	if len(resID) > 9 && resID[:9] == "chatcmpl-" {
		resID = "resp_" + resID[9:]
	} else if resID == "" {
		resID = fmt.Sprintf("resp_%d", time.Now().UnixMicro())
	}

	var outputItems []ResponseOutputItem
	for i, choice := range chatResp.Choices {
		msgID := fmt.Sprintf("msg_%s_%d", resID, i)
		text := ""
		if str, ok := choice.Message.Content.(string); ok {
			text = str
		} else if choice.Message.Content != nil {
			b, _ := json.Marshal(choice.Message.Content)
			text = string(b)
		}

		// A pure tool-call turn has no assistant text; emitting an empty message
		// item would make agents append a blank assistant message. Keep the
		// message item only when there is text or when the turn produced no
		// function calls at all (so a genuinely empty answer stays visible).
		if text != "" || len(choice.Message.ToolCalls) == 0 {
			role := choice.Message.Role
			if role == "" {
				role = "assistant"
			}
			outputItems = append(outputItems, ResponseOutputItem{
				ID:     msgID,
				Type:   "message",
				Status: "completed",
				Role:   role,
				Content: []ResponseContentPart{
					{
						Type: "output_text",
						Text: text,
					},
				},
			})
		}
		outputItems = append(outputItems, chatToolCallsToResponseItems(
			fmt.Sprintf("%s_%d", resID, i), choice.Message.ToolCalls)...)
	}

	created := chatResp.Created
	if created == 0 {
		created = time.Now().Unix()
	}

	usage := ResponseUsage{}
	if chatResp.Usage != nil {
		usage.InputTokens = chatResp.Usage.PromptTokens
		usage.OutputTokens = chatResp.Usage.CompletionTokens
		usage.TotalTokens = chatResp.Usage.TotalTokens
	}

	return &ResponseResult{
		ID:        resID,
		Object:    "response",
		CreatedAt: created,
		Status:    "completed",
		Model:     chatResp.Model,
		Output:    outputItems,
		Usage:     usage,
	}
}
