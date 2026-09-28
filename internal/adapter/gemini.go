package adapter

import (
	"context"
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

	for _, c := range req.Contents {
		role := c.Role
		if role == "model" {
			role = "assistant"
		}
		var text string
		var toolCalls []model.ToolCall
		for _, p := range c.Parts {
			if p.Text != "" {
				text += p.Text
			}
			if p.FunctionCall != nil {
				idx := len(toolCalls)
				toolCalls = append(toolCalls, model.ToolCall{
					Index: &idx,
					ID:    p.FunctionCall.Name,
					Type:  "function",
					Function: model.FunctionCall{
						Name:      p.FunctionCall.Name,
						Arguments: "{}",
					},
				})
			}
			if p.FunctionResponse != nil {
				role = "tool"
				text = fmt.Sprintf("%v", p.FunctionResponse.Response)
			}
		}
		msgs = append(msgs, model.ChatMessage{
			Role:      role,
			Content:   text,
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
			parts = append(parts, provider.GeminiPart{
				FunctionCall: &provider.GeminiFunctionCall{
					Name: tc.Function.Name,
				},
			})
		}
		c := provider.GeminiCandidate{
			Index: i,
		}
		c.Content.Role = "model"
		c.Content.Parts = parts
		c.FinishReason = "STOP"
		candidates = append(candidates, c)
	}

	pTokens := 0
	cTokens := 0
	if resp.Usage != nil {
		pTokens = resp.Usage.PromptTokens
		cTokens = resp.Usage.CompletionTokens
	}

	return map[string]any{
		"candidates": candidates,
		"usageMetadata": provider.GeminiUsageMetadata{
			PromptTokenCount:     pTokens,
			CandidatesTokenCount: cTokens,
			TotalTokenCount:      pTokens + cTokens,
		},
		"modelVersion": resp.Model,
	}, nil
}
