package adapter

import (
	"encoding/json"
	"fmt"

	"github.com/ifnodoraemon/airoute/internal/model"
	"github.com/ifnodoraemon/airoute/internal/provider"
)

type geminiConversation struct {
	pendingToolIDs  map[string][]string
	issuedToolCalls int
}
type geminiTurn struct {
	text       string
	reasoning  string
	calls      []model.ToolCall
	parts      []model.ContentPart
	multimodal bool
	results    []model.ChatMessage
}

func (conversation *geminiConversation) messages(c provider.GeminiContent) []model.ChatMessage {
	role := c.Role
	if role == "model" {
		role = "assistant"
	}
	var turn geminiTurn
	for _, part := range c.Parts {
		conversation.appendPart(&turn, part)
	}
	var msgs []model.ChatMessage
	if len(turn.results) > 0 {
		msgs = append(msgs, turn.results...)
		// Any non-tool content that shared the turn (rare) follows the results.
		if turn.text != "" || turn.multimodal {
			var content any = turn.text
			if turn.multimodal {
				content = turn.parts
			}
			msgs = append(msgs, model.ChatMessage{Role: "user", Content: content})
		}
		return msgs
	}

	var content any = turn.text
	if turn.multimodal {
		content = turn.parts
	}

	msgs = append(msgs, model.ChatMessage{
		ReasoningContent: turn.reasoning,
		Role:             role,
		Content:          content,
		ToolCalls:        turn.calls,
	})
	return msgs
}

func (conversation *geminiConversation) appendPart(turn *geminiTurn, p provider.GeminiPart) {
	if p.Thought {
		turn.reasoning += p.Text
	} else if p.Text != "" {
		turn.text += p.Text
		turn.parts = append(turn.parts, model.ContentPart{
			Type: model.ContentPartText,
			Text: p.Text,
		})
	}
	if p.InlineData != nil && p.InlineData.Data != "" {
		turn.multimodal = true
		mime := p.InlineData.MimeType
		if mime == "" {
			mime = "image/jpeg"
		}
		turn.parts = append(turn.parts, model.ContentPart{
			Type: model.ContentPartImageURL,
			ImageURL: &model.ImageURLPart{
				URL: fmt.Sprintf("data:%s;base64,%s", mime, p.InlineData.Data),
			},
		})
	}
	// A fileData part is Gemini's remote-resource form; keep the URI so
	// the outbound converter can forward it as a URL instead of
	// mislabelling it as inline base64 (F-07).
	if p.FileData != nil && p.FileData.FileURI != "" {
		turn.multimodal = true
		turn.parts = append(turn.parts, model.ContentPart{
			Type:     model.ContentPartImageURL,
			ImageURL: &model.ImageURLPart{URL: p.FileData.FileURI},
		})
	}
	if p.FunctionCall != nil {
		conversation.appendCall(turn, p)
	}
	if p.FunctionResponse != nil {
		conversation.appendResult(turn, p)
	}
}

func (conversation *geminiConversation) appendCall(turn *geminiTurn, p provider.GeminiPart) {
	idx := len(turn.calls)
	argsStr := "{}"
	if p.FunctionCall.Args != nil {
		if b, err := json.Marshal(p.FunctionCall.Args); err == nil {
			argsStr = string(b)
		}
	}
	conversation.issuedToolCalls++
	id := fmt.Sprintf("call_%d_%s", conversation.issuedToolCalls, p.FunctionCall.Name)
	if p.FunctionCall.ID != "" {
		id = p.FunctionCall.ID
	}
	if name := p.FunctionCall.Name; name != "" {
		conversation.pendingToolIDs[name] = append(conversation.pendingToolIDs[name], id)
	}
	turn.calls = append(turn.calls, model.ToolCall{
		Index: &idx,
		ID:    id,
		Type:  "function",
		Function: model.FunctionCall{
			Name:      p.FunctionCall.Name,
			Arguments: argsStr,
		},
	})
	turn.calls[len(turn.calls)-1].SetThoughtSignature(p.ThoughtSignature)
}

func (conversation *geminiConversation) appendResult(turn *geminiTurn, p provider.GeminiPart) {
	// One canonical tool message per response: Gemini puts every parallel
	// functionResponse of a turn in the same content, and collapsing them
	// into one message silently dropped all but the last result.
	toolName := p.FunctionResponse.Name
	toolCallID := takeGeminiToolID(conversation.pendingToolIDs, toolName, p.FunctionResponse.ID)
	payload := fmt.Sprintf("%v", p.FunctionResponse.Response)
	if encoded, err := json.Marshal(p.FunctionResponse.Response); err == nil {
		payload = string(encoded)
	}
	turn.results = append(turn.results, model.ChatMessage{
		Role:       "tool",
		Content:    payload,
		ToolCallID: toolCallID,
		Name:       toolName,
	})
}
