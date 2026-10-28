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
	// C-02: the cache-write count must be readable downstream (billing applies the
	// 1.25x write premium from it) while staying part of the prompt total.
	if got.GetCacheCreationTokens() != 50 {
		t.Fatalf("cache creation tokens = %d, want 50", got.GetCacheCreationTokens())
	}
}

func TestAnthropicUsageCacheCreationOnly(t *testing.T) {
	// A pure cache write (no cache read) must still produce the details breakdown;
	// the old mapping only allocated it when cache reads were present.
	got := anthropicUsageToModel(&AnthropicUsage{InputTokens: 10, OutputTokens: 2, CacheCreationInputTokens: 40})
	if got.PromptTokens != 50 {
		t.Fatalf("prompt tokens = %d, want 50 (10 input + 40 cache write)", got.PromptTokens)
	}
	if got.GetCacheCreationTokens() != 40 {
		t.Fatalf("cache creation tokens = %d, want 40", got.GetCacheCreationTokens())
	}
	if got.GetCachedTokens() != 0 {
		t.Fatalf("cached (read) tokens = %d, want 0", got.GetCachedTokens())
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
	// Gemini reports no cache-write count; the field must stay 0 instead of being
	// invented from the cached-content count.
	if got.GetCacheCreationTokens() != 0 {
		t.Fatalf("cache creation tokens = %d, want 0", got.GetCacheCreationTokens())
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

// TestGeminiUsageToModelFoldsThoughtsTokens is the BUG-GEMINI-THOUGHTS regression
// test: a thinking model reports the visible answer in candidatesTokenCount and
// the reasoning in thoughtsTokenCount, while totalTokenCount includes both.
// Completion must be the sum, so the canonical usage is self-consistent and the
// billing engine (which prices PromptTokens/CompletionTokens) charges for the
// reasoning output Google billed us for.
func TestGeminiUsageToModelFoldsThoughtsTokens(t *testing.T) {
	got := geminiUsageToModel(&GeminiUsageMetadata{
		PromptTokenCount:     100,
		CandidatesTokenCount: 50,
		ThoughtsTokenCount:   500,
		TotalTokenCount:      650,
	})
	if got == nil {
		t.Fatal("expected non-nil usage")
	}
	if got.CompletionTokens != 550 {
		t.Fatalf("completion tokens = %d, want 550 (candidates 50 + thoughts 500)", got.CompletionTokens)
	}
	if got.PromptTokens != 100 {
		t.Fatalf("prompt tokens = %d, want 100", got.PromptTokens)
	}
	// No double counting: the upstream total already contains the thoughts, and it
	// must agree with the parts it is composed of.
	if got.TotalTokens != 650 {
		t.Fatalf("total tokens = %d, want the upstream total 650", got.TotalTokens)
	}
	if got.PromptTokens+got.CompletionTokens != got.TotalTokens {
		t.Fatalf("prompt(%d)+completion(%d) = %d, want the total %d (usage must be self-consistent)",
			got.PromptTokens, got.CompletionTokens, got.PromptTokens+got.CompletionTokens, got.TotalTokens)
	}
}

// TestGeminiUsageToModelNormalizesInconsistentTotal guards upstream proxies that
// report a total smaller than its own parts: the canonical total must never
// contradict the billed counts.
func TestGeminiUsageToModelNormalizesInconsistentTotal(t *testing.T) {
	got := geminiUsageToModel(&GeminiUsageMetadata{
		PromptTokenCount:     100,
		CandidatesTokenCount: 50,
		ThoughtsTokenCount:   500,
		TotalTokenCount:      120, // truncated/incoherent upstream total
	})
	if got.TotalTokens != 650 {
		t.Fatalf("total tokens = %d, want 650 (prompt+completion wins over a smaller upstream total)", got.TotalTokens)
	}
}

// TestGeminiUsageToModelWithoutThoughtsUnchanged pins non-thinking traffic.
func TestGeminiUsageToModelWithoutThoughtsUnchanged(t *testing.T) {
	got := geminiUsageToModel(&GeminiUsageMetadata{
		PromptTokenCount:     1200,
		CandidatesTokenCount: 300,
		TotalTokenCount:      1500,
	})
	if got.CompletionTokens != 300 || got.TotalTokens != 1500 {
		t.Fatalf("unexpected usage without thoughts: %+v", got)
	}
}
