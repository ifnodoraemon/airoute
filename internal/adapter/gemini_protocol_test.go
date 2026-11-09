package adapter

import (
	"testing"

	"github.com/ifnodoraemon/airoute/internal/model"
	"github.com/ifnodoraemon/airoute/internal/provider"
)

func TestGeminiToolIDsPairExplicitOutOfOrderResultsWithRemainingCalls(t *testing.T) {
	request := &provider.GeminiRequest{Contents: []provider.GeminiContent{
		{Role: "model", Parts: []provider.GeminiPart{
			{FunctionCall: &provider.GeminiFunctionCall{ID: "first", Name: "lookup"}, ThoughtSignature: "signature=="},
			{FunctionCall: &provider.GeminiFunctionCall{ID: "second", Name: "lookup"}},
		}},
		{Role: "user", Parts: []provider.GeminiPart{
			{FunctionResponse: &provider.GeminiFunctionResponse{ID: "second", Name: "lookup", Response: map[string]any{"x": 2}}},
			{FunctionResponse: &provider.GeminiFunctionResponse{Name: "lookup", Response: map[string]any{"x": 1}}},
		}},
	}}
	canonical, err := GeminiToCanonical(request)
	if err != nil {
		t.Fatal(err)
	}
	if canonical.Messages[1].ToolCallID != "second" || canonical.Messages[2].ToolCallID != "first" || canonical.Messages[0].ToolCalls[0].ThoughtSignature() != "signature==" {
		t.Fatalf("tool pairing/signature lost: %+v", canonical.Messages)
	}
	response := &model.ChatCompletionResponse{Choices: []model.ChatCompletionChoice{{Message: canonical.Messages[0]}}}
	converted, err := GeminiFromCanonical(response)
	if err != nil {
		t.Fatal(err)
	}
	candidates := converted.(map[string]any)["candidates"].([]provider.GeminiCandidate)
	if candidates[0].Content.Parts[0].ThoughtSignature != "signature==" || candidates[0].Content.Parts[0].FunctionCall.ID != "first" {
		t.Fatalf("Gemini reply lost signature or ID: %+v", candidates)
	}
}
