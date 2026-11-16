package provider

import "testing"

// Gemini 3 reports tool/code-execution prompt tokens separately in
// toolUsePromptTokenCount; dropping it under-billed that part of the prompt on
// both the passthrough and conversion paths.
func TestGeminiUsageIncludesToolUsePromptTokens(t *testing.T) {
	u := &GeminiUsageMetadata{PromptTokenCount: 100, ToolUsePromptTokenCount: 30, CandidatesTokenCount: 20, ThoughtsTokenCount: 5, TotalTokenCount: 155}
	got := geminiUsageToModel(u)
	if got.PromptTokens != 130 {
		t.Fatalf("PromptTokens = %d, want 130 (100 prompt + 30 tool-use)", got.PromptTokens)
	}
	if got.CompletionTokens != 25 {
		t.Fatalf("CompletionTokens = %d, want 25 (20 candidates + 5 thoughts)", got.CompletionTokens)
	}
	if got.TotalTokens != 155 {
		t.Fatalf("TotalTokens = %d, want 155", got.TotalTokens)
	}
}