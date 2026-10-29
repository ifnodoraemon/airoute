package adapter

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/ifnodoraemon/airoute/internal/model"
	"github.com/ifnodoraemon/airoute/internal/provider"
)

// GeminiAdapter implements ProtocolAdapter for Google Gemini API.
type GeminiAdapter struct{}

// NewGeminiAdapter creates a new GeminiAdapter.
func NewGeminiAdapter() *GeminiAdapter {
	return &GeminiAdapter{}
}

func (a *GeminiAdapter) Protocol() string {
	return "gemini"
}

func (a *GeminiAdapter) ToCanonical(ctx context.Context, input any) (*model.ChatCompletionRequest, error) {
	req, ok := input.(*provider.GeminiRequest)
	if !ok {
		return nil, fmt.Errorf("invalid input type for GeminiAdapter: expected *provider.GeminiRequest")
	}

	var msgs []model.ChatMessage
	if req.SystemInstruction != nil {
		var sysText string
		for _, p := range req.SystemInstruction.Parts {
			sysText += p.Text
		}
		if sysText != "" {
			msgs = append(msgs, model.ChatMessage{
				Role:    "system",
				Content: sysText,
			})
		}
	}

	// Gemini function calls carry no id, but OpenAI and Anthropic tool messages
	// require the assistant's tool_call/use id on the matching result. Issue a
	// stable id per call and hand it to the functionResponse that answers it,
	// otherwise the canonical tool message has an empty tool_call_id and upstreams
	// reject the follow-up turn.
	pendingToolIDs := make(map[string][]string)
	issuedToolCalls := 0

	for _, c := range req.Contents {
		role := c.Role
		if role == "model" {
			role = "assistant"
		}
		var text string
		var toolCalls []model.ToolCall
		var contentParts []model.ContentPart
		var hasMultimodal bool
		var toolResults []model.ChatMessage

		for _, p := range c.Parts {
			if p.Text != "" {
				text += p.Text
				contentParts = append(contentParts, model.ContentPart{
					Type: model.ContentPartText,
					Text: p.Text,
				})
			}
			if p.InlineData != nil && p.InlineData.Data != "" {
				hasMultimodal = true
				mime := p.InlineData.MimeType
				if mime == "" {
					mime = "image/jpeg"
				}
				contentParts = append(contentParts, model.ContentPart{
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
				hasMultimodal = true
				contentParts = append(contentParts, model.ContentPart{
					Type:     model.ContentPartImageURL,
					ImageURL: &model.ImageURLPart{URL: p.FileData.FileURI},
				})
			}
			if p.FunctionCall != nil {
				idx := len(toolCalls)
				argsStr := "{}"
				if p.FunctionCall.Args != nil {
					if b, err := json.Marshal(p.FunctionCall.Args); err == nil {
						argsStr = string(b)
					}
				}
				issuedToolCalls++
				id := fmt.Sprintf("call_%d_%s", issuedToolCalls, p.FunctionCall.Name)
				if name := p.FunctionCall.Name; name != "" {
					pendingToolIDs[name] = append(pendingToolIDs[name], id)
				}
				toolCalls = append(toolCalls, model.ToolCall{
					Index: &idx,
					ID:    id,
					Type:  "function",
					Function: model.FunctionCall{
						Name:      p.FunctionCall.Name,
						Arguments: argsStr,
					},
				})
			}
			if p.FunctionResponse != nil {
				// One canonical tool message per response: Gemini puts every parallel
				// functionResponse of a turn in the same content, and collapsing them
				// into one message silently dropped all but the last result.
				toolName := p.FunctionResponse.Name
				toolCallID := ""
				// FIFO per function name: repeated calls to the same tool keep their
				// own ids, in the order Gemini answers them.
				if queue := pendingToolIDs[toolName]; len(queue) > 0 {
					toolCallID = queue[0]
					pendingToolIDs[toolName] = queue[1:]
				} else if toolName != "" {
					// Unpaired response (e.g. a conversation resumed elsewhere):
					// still produce a deterministic, non-empty id.
					toolCallID = "call_" + toolName
				}
				payload := fmt.Sprintf("%v", p.FunctionResponse.Response)
				if encoded, err := json.Marshal(p.FunctionResponse.Response); err == nil {
					payload = string(encoded)
				}
				toolResults = append(toolResults, model.ChatMessage{
					Role:       "tool",
					Content:    payload,
					ToolCallID: toolCallID,
					Name:       toolName,
				})
			}
		}

		if len(toolResults) > 0 {
			msgs = append(msgs, toolResults...)
			// Any non-tool content that shared the turn (rare) follows the results.
			if text != "" || hasMultimodal {
				var content any = text
				if hasMultimodal {
					content = contentParts
				}
				msgs = append(msgs, model.ChatMessage{Role: "user", Content: content})
			}
			continue
		}

		var content any = text
		if hasMultimodal {
			content = contentParts
		}

		msgs = append(msgs, model.ChatMessage{
			Role:      role,
			Content:   content,
			ToolCalls: toolCalls,
		})
	}

	var temp *float64
	var topP *float64
	var maxTokens *int
	if req.GenerationConfig != nil {
		temp = req.GenerationConfig.Temperature
		topP = req.GenerationConfig.TopP
		maxTokens = req.GenerationConfig.MaxOutputTokens
	}

	var tools []model.Tool
	for _, tc := range req.Tools {
		for _, fd := range tc.FunctionDeclarations {
			params := fd.Parameters
			if params == nil {
				params = map[string]any{"type": "object", "properties": map[string]any{}}
			}
			tools = append(tools, model.Tool{
				Type: "function",
				Function: map[string]any{
					"name":        fd.Name,
					"description": fd.Description,
					"parameters":  params,
				},
			})
		}
	}

	return &model.ChatCompletionRequest{
		Messages:    msgs,
		Temperature: temp,
		TopP:        topP,
		MaxTokens:   maxTokens,
		Tools:       tools,
	}, nil
}

func (a *GeminiAdapter) FromCanonical(ctx context.Context, resp *model.ChatCompletionResponse) (any, error) {
	if resp == nil {
		return nil, fmt.Errorf("empty canonical response")
	}

	var candidates []provider.GeminiCandidate
	for i, choice := range resp.Choices {
		var parts []provider.GeminiPart
		if choice.Message.GetContentString() != "" {
			parts = append(parts, provider.GeminiPart{Text: choice.Message.GetContentString()})
		}
		for _, tc := range choice.Message.ToolCalls {
			var args map[string]any
			if tc.Function.Arguments != "" {
				_ = json.Unmarshal([]byte(tc.Function.Arguments), &args)
			}
			parts = append(parts, provider.GeminiPart{
				FunctionCall: &provider.GeminiFunctionCall{
					Name: tc.Function.Name,
					Args: args,
				},
			})
		}
		c := provider.GeminiCandidate{
			Index: i,
		}
		c.Content.Role = "model"
		c.Content.Parts = parts
		finishReason := "STOP"
		if choice.FinishReason != nil {
			switch *choice.FinishReason {
			case "length":
				finishReason = "MAX_TOKENS"
			default:
				finishReason = "STOP"
			}
		}
		c.FinishReason = finishReason
		candidates = append(candidates, c)
	}

	pTokens := 0
	cTokens := 0
	tTokens := 0
	if resp.Usage != nil {
		pTokens = resp.Usage.PromptTokens
		// The canonical completion count already folds Gemini's thoughtsTokenCount
		// in (provider.geminiUsageToModel), matching what the upstream billed.
		cTokens = resp.Usage.CompletionTokens
		tTokens = resp.Usage.TotalTokens
	}
	// Never overwrite the upstream's own total with prompt+candidates: on thinking
	// models that would drop the reasoning tokens from the reported total and
	// contradict the completion count (BUG-GEMINI-THOUGHTS). Only synthesize a
	// total when the upstream did not report one.
	if tTokens < pTokens+cTokens {
		tTokens = pTokens + cTokens
	}

	return map[string]any{
		"candidates": candidates,
		"usageMetadata": provider.GeminiUsageMetadata{
			PromptTokenCount:     pTokens,
			CandidatesTokenCount: cTokens,
			TotalTokenCount:      tTokens,
		},
		"modelVersion": resp.Model,
	}, nil
}
