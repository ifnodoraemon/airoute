package api

import (
	"testing"

	"github.com/ifnodoraemon/airoute/internal/model"
)

func TestNativeReplyWithoutUsageStillBillsEstimatedOutput(t *testing.T) {
	request := &model.ChatCompletionRequest{Model: "m", Messages: []model.ChatMessage{{Role: "user", Content: "question"}}}
	_, output, _, source := resolveChatUsage(request, &model.ChatCompletionResponse{NativeOutputChars: 400})
	if output <= 0 || source != usageSourceEstimated {
		t.Fatalf("missing native usage became free output: output=%d source=%s", output, source)
	}
}

func TestGeminiEmbeddingEstimateDoesNotMasqueradeAsUpstreamUsage(t *testing.T) {
	request := &model.EmbeddingRequest{Model: "m", Input: "some embedding text"}
	tokens, source := resolveEmbeddingTokens(request, model.EmbeddingUsage{PromptTokens: 5, TotalTokens: 5, Estimated: true})
	if tokens <= 0 || source != usageSourceEstimated {
		t.Fatalf("local estimate mislabeled as upstream: tokens=%d source=%s", tokens, source)
	}
}
