package adapter

import (
	"fmt"

	"github.com/ifnodoraemon/airoute/internal/model"
	"github.com/ifnodoraemon/airoute/internal/provider"
)

// GeminiFromCanonical renders a chat response in the Gemini wire format.
func GeminiFromCanonical(resp *model.ChatCompletionResponse) (any, error) {
	if resp == nil {
		return nil, fmt.Errorf("empty canonical response")
	}

	var candidates []provider.GeminiCandidate
	for i, choice := range resp.Choices {
		candidates = append(candidates, geminiCanonicalCandidate(i, choice))
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
			PromptTokenCount:        pTokens,
			CandidatesTokenCount:    max(0, cTokens-geminiReasoningTokens(resp)),
			ThoughtsTokenCount:      geminiReasoningTokens(resp),
			CachedContentTokenCount: geminiCachedTokens(resp),
			TotalTokenCount:         tTokens,
		},
		"modelVersion": resp.Model,
	}, nil
}

func geminiCanonicalCandidate(i int, choice model.ChatCompletionChoice) provider.GeminiCandidate {
	var parts []provider.GeminiPart
	if choice.Message.ReasoningContent != "" {
		parts = append(parts, provider.GeminiPart{Text: choice.Message.ReasoningContent, Thought: true})
	}
	if choice.Message.GetContentString() != "" {
		parts = append(parts, provider.GeminiPart{Text: choice.Message.GetContentString()})
	}
	for _, tc := range choice.Message.ToolCalls {
		args := provider.GeminiFunctionArguments(tc.Function.Arguments)
		parts = append(parts, provider.GeminiPart{
			ThoughtSignature: tc.ThoughtSignature(),
			FunctionCall: &provider.GeminiFunctionCall{
				ID:   tc.ID,
				Name: tc.Function.Name,
				Args: args,
			},
		})
	}
	c := provider.GeminiCandidate{
		Index: max(i, choice.Index),
	}
	c.Content.Role = "model"
	c.Content.Parts = parts
	finishReason := "STOP"
	if choice.FinishReason != nil {
		switch *choice.FinishReason {
		case "length":
			finishReason = "MAX_TOKENS"
		case "content_filter":
			finishReason = "SAFETY"
		default:
			finishReason = "STOP"
		}
	}
	c.FinishReason = finishReason
	return c
}
