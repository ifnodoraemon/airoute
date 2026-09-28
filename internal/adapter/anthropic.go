package adapter

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/ifnodoraemon/airoute/internal/model"
)

// AnthropicAdapter implements ProtocolAdapter for Anthropic Claude Messages API.
type AnthropicAdapter struct{}

// NewAnthropicAdapter creates a new AnthropicAdapter.
func NewAnthropicAdapter() *AnthropicAdapter {
	return &AnthropicAdapter{}
}

func (a *AnthropicAdapter) Protocol() string {
	return "anthropic"
}

func (a *AnthropicAdapter) ToCanonical(ctx context.Context, input any) (*model.ChatCompletionRequest, error) {
	req, ok := input.(*model.AnthropicInboundRequest)
	if !ok {
		return nil, fmt.Errorf("invalid input type for AnthropicAdapter: expected *model.AnthropicInboundRequest")
	}

	var canonicalMsgs []model.ChatMessage
	if req.System != "" {
		canonicalMsgs = append(canonicalMsgs, model.ChatMessage{
			Role:    "system",
			Content: req.System,
		})
	}

	for _, msg := range req.Messages {
		if blocks, ok := msg.Content.([]any); ok {
			var textParts []string
			var toolCalls []model.ToolCall
			var hasToolResult bool
			var contentParts []model.ContentPart
			var hasImage bool

			for _, b := range blocks {
				m, isMap := b.(map[string]any)
				if !isMap {
					continue
				}
				bType, _ := m["type"].(string)
				switch bType {
				case "text":
					t, _ := m["text"].(string)
					textParts = append(textParts, t)
					contentParts = append(contentParts, model.ContentPart{
						Type: "text",
						Text: t,
					})
				case "image":
					hasImage = true
					if src, ok := m["source"].(map[string]any); ok {
						mediaType, _ := src["media_type"].(string)
						data, _ := src["data"].(string)
						if mediaType != "" && data != "" {
							contentParts = append(contentParts, model.ContentPart{
								Type: "image_url",
								ImageURL: &model.ImageURLPart{
									URL: fmt.Sprintf("data:%s;base64,%s", mediaType, data),
								},
							})
						}
					}
				case "tool_use":
					id, _ := m["id"].(string)
					name, _ := m["name"].(string)
					argsBytes, _ := json.Marshal(m["input"])
					idx := len(toolCalls)
					toolCalls = append(toolCalls, model.ToolCall{
						Index: &idx,
						ID:    id,
						Type:  "function",
						Function: model.FunctionCall{
							Name:      name,
							Arguments: string(argsBytes),
						},
					})
				case "tool_result":
					hasToolResult = true
					toolID, _ := m["tool_use_id"].(string)
					cText := ""
					if s, ok := m["content"].(string); ok {
						cText = s
					} else if bArr, ok := m["content"].([]any); ok {
						for _, item := range bArr {
							if itemMap, ok := item.(map[string]any); ok {
								if t, ok := itemMap["text"].(string); ok {
									cText += t
								}
							}
						}
					}
					canonicalMsgs = append(canonicalMsgs, model.ChatMessage{
						Role:       "tool",
						Content:    cText,
						ToolCallID: toolID,
					})
				}
			}

			if hasToolResult {
				continue
			}

			if hasImage {
				canonicalMsgs = append(canonicalMsgs, model.ChatMessage{
					Role:      msg.Role,
					Content:   contentParts,
					ToolCalls: toolCalls,
				})
			} else {
				canonicalMsgs = append(canonicalMsgs, model.ChatMessage{
					Role:      msg.Role,
					Content:   strings.Join(textParts, "\n"),
					ToolCalls: toolCalls,
				})
			}
		} else {
			canonicalMsgs = append(canonicalMsgs, model.ChatMessage{
				Role:    msg.Role,
				Content: msg.Content,
			})
		}
	}

	var maxTokens *int
	if req.MaxTokens > 0 {
		maxTokens = &req.MaxTokens
	}

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

	return &model.ChatCompletionRequest{
		Model:       req.Model,
		Messages:    canonicalMsgs,
		Temperature: req.Temperature,
		TopP:        req.TopP,
		MaxTokens:   maxTokens,
		Stream:      req.Stream,
		Tools:       canonicalTools,
	}, nil
}

func (a *AnthropicAdapter) FromCanonical(ctx context.Context, resp *model.ChatCompletionResponse) (any, error) {
	if resp == nil {
		return nil, fmt.Errorf("empty canonical response")
	}

	var contentBlocks []map[string]any
	stopReason := "end_turn"

	if len(resp.Choices) > 0 {
		choice := resp.Choices[0]
		if replyText := choice.Message.GetContentString(); replyText != "" {
			contentBlocks = append(contentBlocks, map[string]any{
				"type": "text",
				"text": replyText,
			})
		}
		for _, tc := range choice.Message.ToolCalls {
			var parsedArgs any
			if err := json.Unmarshal([]byte(tc.Function.Arguments), &parsedArgs); err != nil {
				parsedArgs = map[string]any{}
			}
			contentBlocks = append(contentBlocks, map[string]any{
				"type":  "tool_use",
				"id":    tc.ID,
				"name":  tc.Function.Name,
				"input": parsedArgs,
			})
		}

		if choice.FinishReason != nil {
			switch *choice.FinishReason {
			case "stop":
				stopReason = "end_turn"
			case "tool_calls", "function_call":
				stopReason = "tool_use"
			case "length":
				stopReason = "max_tokens"
			}
		}
	}

	inputTokens := 0
	outputTokens := 0
	if resp.Usage != nil {
		inputTokens = resp.Usage.PromptTokens
		outputTokens = resp.Usage.CompletionTokens
	}

	return map[string]any{
		"id":          resp.ID,
		"type":        "message",
		"role":        "assistant",
		"model":       resp.Model,
		"content":     contentBlocks,
		"stop_reason": stopReason,
		"usage": map[string]any{
			"input_tokens":  inputTokens,
			"output_tokens": outputTokens,
		},
	}, nil
}
