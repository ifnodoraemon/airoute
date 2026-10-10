package provider

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/ifnodoraemon/airoute/internal/model"
)

// convertOpenAIToAnthropic converts OpenAI ChatCompletionRequest to Anthropic format.
func convertOpenAIToAnthropic(req *model.ChatCompletionRequest, channel *model.ChannelConfig) *AnthropicRequest {
	var systemParts []string
	var anthropicMsgs []AnthropicMessage

	for _, msg := range req.Messages {
		if strings.ToLower(msg.Role) == "system" {
			systemParts = append(systemParts, msg.GetContentString())
			continue
		}

		if strings.ToLower(msg.Role) == "tool" {
			// Convert tool response to user role with tool_result block
			toolResultBlock := map[string]any{
				"type":        "tool_result",
				"tool_use_id": msg.ToolCallID,
				"content":     msg.GetContentString(),
			}
			if len(anthropicMsgs) > 0 && anthropicMsgs[len(anthropicMsgs)-1].Role == "user" {
				if blocks, ok := anthropicMsgs[len(anthropicMsgs)-1].Content.([]any); ok {
					anthropicMsgs[len(anthropicMsgs)-1].Content = append(blocks, toolResultBlock)
				} else {
					anthropicMsgs[len(anthropicMsgs)-1].Content = []any{
						map[string]any{"type": "text", "text": fmt.Sprint(anthropicMsgs[len(anthropicMsgs)-1].Content)},
						toolResultBlock,
					}
				}
			} else {
				anthropicMsgs = append(anthropicMsgs, AnthropicMessage{
					Role:    "user",
					Content: []any{toolResultBlock},
				})
			}
			continue
		}

		if strings.ToLower(msg.Role) == "assistant" && len(msg.ToolCalls) > 0 {
			var blocks []any
			contentStr := msg.GetContentString()
			if contentStr != "" {
				blocks = append(blocks, map[string]any{
					"type": "text",
					"text": contentStr,
				})
			}
			for _, tc := range msg.ToolCalls {
				var parsedInput any
				if err := json.Unmarshal([]byte(tc.Function.Arguments), &parsedInput); err != nil {
					parsedInput = map[string]any{}
				}
				blocks = append(blocks, map[string]any{
					"type":  "tool_use",
					"id":    tc.ID,
					"name":  tc.Function.Name,
					"input": parsedInput,
				})
			}
			anthropicMsgs = append(anthropicMsgs, AnthropicMessage{
				Role:    "assistant",
				Content: blocks,
			})
			continue
		}

		parts := model.ParseMessageContent(msg.Content)
		hasImage := false
		for _, p := range parts {
			if p.Type == model.ContentPartImageURL {
				hasImage = true
				break
			}
		}

		if !hasImage {
			anthropicMsgs = append(anthropicMsgs, AnthropicMessage{
				Role:    msg.Role,
				Content: msg.GetContentString(),
			})
		} else {
			var blocks []any
			for _, p := range parts {
				switch p.Type {
				case model.ContentPartText:
					if p.Text != "" {
						blocks = append(blocks, map[string]any{
							"type": "text",
							"text": p.Text,
						})
					}
				case model.ContentPartImageURL:
					if p.ImageURL != nil && p.ImageURL.URL != "" {
						mime, b64 := model.ParseDataURI(p.ImageURL.URL)
						if mime == "" {
							mime = "image/jpeg"
						}
						blocks = append(blocks, map[string]any{
							"type": "image",
							"source": map[string]any{
								"type":       "base64",
								"media_type": mime,
								"data":       b64,
							},
						})
					}
				}
			}
			anthropicMsgs = append(anthropicMsgs, AnthropicMessage{
				Role:    msg.Role,
				Content: blocks,
			})
		}
	}

	var tools []AnthropicTool
	for _, t := range req.Tools {
		fnName, _ := t.Function["name"].(string)
		fnDesc, _ := t.Function["description"].(string)
		fnParams := t.Function["parameters"]
		if fnParams == nil {
			fnParams = map[string]any{"type": "object", "properties": map[string]any{}}
		}
		tools = append(tools, AnthropicTool{
			Name:        fnName,
			Description: fnDesc,
			InputSchema: fnParams,
		})
	}

	maxTokens := 4096
	if req.MaxTokens != nil && *req.MaxTokens > 0 {
		maxTokens = *req.MaxTokens
	}

	return &AnthropicRequest{
		Model:       channel.GetUpstreamModel(req.Model),
		Messages:    anthropicMsgs,
		System:      strings.Join(systemParts, "\n\n"),
		MaxTokens:   maxTokens,
		Temperature: req.Temperature,
		TopP:        req.TopP,
		Stream:      req.Stream,
		Tools:       tools,
	}
}
