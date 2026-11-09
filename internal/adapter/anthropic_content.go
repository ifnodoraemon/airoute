package adapter

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/ifnodoraemon/airoute/internal/model"
)

type anthropicMessageContent struct {
	text     []string
	thinking []string
	calls    []model.ToolCall
	parts    []model.ContentPart
	hasImage bool
	results  []model.ChatMessage
}

func anthropicMessageToCanonical(message model.AnthropicInboundMessage) []model.ChatMessage {
	blocks, ok := message.Content.([]any)
	if !ok {
		return []model.ChatMessage{{Role: message.Role, Content: message.Content}}
	}
	var content anthropicMessageContent
	for _, block := range blocks {
		if object, ok := block.(map[string]any); ok {
			content.appendBlock(object)
		}
	}
	reasoning := strings.Join(content.thinking, "\n")
	if len(content.results) > 0 {
		if len(content.text) > 0 {
			content.results = append(content.results, model.ChatMessage{Role: message.Role, Content: strings.Join(content.text, "\n"), ReasoningContent: reasoning})
		}
		return content.results
	}
	var body any = strings.Join(content.text, "\n")
	if content.hasImage {
		body = content.parts
	}
	return []model.ChatMessage{{Role: message.Role, Content: body, ReasoningContent: reasoning, ToolCalls: content.calls}}
}

func (content *anthropicMessageContent) appendBlock(m map[string]any) {
	kind, _ := m["type"].(string)
	switch kind {
	case "text":
		content.appendText(m)
	case "thinking":
		content.appendThinking(m)
	case "image":
		content.appendImage(m)
	case "tool_use":
		content.appendToolCall(m)
	case "tool_result":
		content.appendToolResult(m)
	}
}

func (content *anthropicMessageContent) appendText(m map[string]any) {
	t, _ := m["text"].(string)
	content.text = append(content.text, t)
	content.parts = append(content.parts, model.ContentPart{
		Type: "text",
		Text: t,
	})
}

func (content *anthropicMessageContent) appendThinking(m map[string]any) {
	if th, ok := m["thinking"].(string); ok && th != "" {
		content.thinking = append(content.thinking, th)
	}
}

func (content *anthropicMessageContent) appendImage(m map[string]any) {
	content.hasImage = true
	if src, ok := m["source"].(map[string]any); ok {
		// The Messages API accepts two source forms. Only the
		// base64 one used to be understood, so a remote image
		// (source.type="url") was dropped silently and the model
		// answered as if no picture had been sent (F-07).
		sourceType, _ := src["type"].(string)
		if sourceType == "url" {
			if remoteURL, _ := src["url"].(string); remoteURL != "" {
				content.parts = append(content.parts, model.ContentPart{
					Type:     model.ContentPartImageURL,
					ImageURL: &model.ImageURLPart{URL: remoteURL},
				})
			}
			return
		}
		mediaType, _ := src["media_type"].(string)
		data, _ := src["data"].(string)
		if mediaType != "" && data != "" {
			content.parts = append(content.parts, model.ContentPart{
				Type: "image_url",
				ImageURL: &model.ImageURLPart{
					URL: fmt.Sprintf("data:%s;base64,%s", mediaType, data),
				},
			})
		}
	}
}

func (content *anthropicMessageContent) appendToolCall(m map[string]any) {
	id, _ := m["id"].(string)
	name, _ := m["name"].(string)
	argsBytes, _ := json.Marshal(m["input"])
	idx := len(content.calls)
	content.calls = append(content.calls, model.ToolCall{
		Index: &idx,
		ID:    id,
		Type:  "function",
		Function: model.FunctionCall{
			Name:      name,
			Arguments: string(argsBytes),
		},
	})
}

func (content *anthropicMessageContent) appendToolResult(m map[string]any) {
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
	content.results = append(content.results, model.ChatMessage{
		Role:       "tool",
		Content:    cText,
		ToolCallID: toolID,
	})
}
