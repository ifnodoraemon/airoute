package provider

import "testing"

func TestAnthropicUsageToModel(t *testing.T) {
	u := &AnthropicUsage{
		InputTokens:              100,
		OutputTokens:             40,
		CacheReadInputTokens:     900,
		CacheCreationInputTokens: 50,
	}
	got := anthropicUsageToModel(u)
	if got == nil {
		t.Fatal("expected non-nil usage")
	}
	if got.PromptTokens != 1050 {
		t.Fatalf("prompt tokens = %d, want 1050 (input+cache read+cache creation)", got.PromptTokens)
	}
	if got.CompletionTokens != 40 {
		t.Fatalf("completion tokens = %d, want 40", got.CompletionTokens)
	}
	if got.TotalTokens != 1090 {
		t.Fatalf("total tokens = %d, want 1090", got.TotalTokens)
	}
	if got.PromptTokensDetails == nil || got.PromptTokensDetails.CachedTokens != 900 {
		t.Fatalf("cached tokens = %+v, want 900", got.PromptTokensDetails)
	}
}

func TestGeminiUsageToModel(t *testing.T) {
	u := &GeminiUsageMetadata{
		PromptTokenCount:        1200,
		CandidatesTokenCount:    300,
		TotalTokenCount:         1500,
		CachedContentTokenCount: 800,
	}
	got := geminiUsageToModel(u)
	if got == nil {
		t.Fatal("expected non-nil usage")
	}
	if got.PromptTokens != 1200 || got.CompletionTokens != 300 || got.TotalTokens != 1500 {
		t.Fatalf("unexpected usage: %+v", got)
	}
	if got.PromptTokensDetails == nil || got.PromptTokensDetails.CachedTokens != 800 {
		t.Fatalf("cached tokens = %+v, want 800", got.PromptTokensDetails)
	}
}

func TestUsageMappingNilSafe(t *testing.T) {
	if anthropicUsageToModel(nil) != nil {
		t.Fatal("anthropicUsageToModel(nil) should be nil")
	}
	if geminiUsageToModel(nil) != nil {
		t.Fatal("geminiUsageToModel(nil) should be nil")
	}
}
