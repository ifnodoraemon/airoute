package model

import (
	"encoding/json"
	"testing"
)

func TestCompatibleUsageFields(t *testing.T) {
	for _, tc := range []struct {
		name, raw                          string
		prompt, output, cached, cacheWrite int
	}{
		{"openai", `{"prompt_tokens":32,"completion_tokens":8,"prompt_tokens_details":{"cached_tokens":16}}`, 32, 8, 16, 0},
		{"sglang", `{"prompt_tokens":32,"completion_tokens":8,"cached_tokens":16}`, 32, 8, 16, 0},
		{"deepseek", `{"prompt_tokens":32,"completion_tokens":8,"prompt_cache_hit_tokens":16}`, 32, 8, 16, 0},
		{"responses", `{"input_tokens":32,"output_tokens":8,"input_tokens_details":{"cached_tokens":16}}`, 32, 8, 16, 0},
		{"anthropic", `{"input_tokens":12,"output_tokens":8,"cache_read_input_tokens":16,"cache_creation_input_tokens":4}`, 32, 8, 16, 4},
		{"anthropic details", `{"input_tokens":32,"output_tokens":8,"prompt_tokens_details":{"cache_creation_tokens":4}}`, 32, 8, 0, 4},
		{"total only", `{"total_tokens":40,"completion_tokens":8}`, 32, 8, 0, 0},
		{"cache absent", `{"prompt_tokens":32,"completion_tokens":8}`, 32, 8, 0, 0},
		{"canonical zero wins", `{"prompt_tokens":32,"completion_tokens":8,"prompt_tokens_details":{"cached_tokens":0},"cached_tokens":16}`, 32, 8, 0, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var usage Usage
			if err := json.Unmarshal([]byte(tc.raw), &usage); err != nil {
				t.Fatal(err)
			}
			if usage.PromptTokens != tc.prompt || usage.CompletionTokens != tc.output || usage.GetCachedTokens() != tc.cached || usage.TotalTokens != tc.prompt+tc.output {
				t.Fatalf("unexpected usage: %+v cache=%d", usage, usage.GetCachedTokens())
			}
			// C-02: cache writes are a *subset* of prompt tokens; exposing the count
			// must never change the prompt total (that would double-bill them).
			if got := usage.GetCacheCreationTokens(); got != tc.cacheWrite {
				t.Fatalf("cache creation tokens = %d, want %d (usage: %+v)", got, tc.cacheWrite, usage)
			}
			if tc.cacheWrite > 0 && tc.cacheWrite > usage.PromptTokens {
				t.Fatalf("cache creation tokens %d exceed prompt tokens %d", tc.cacheWrite, usage.PromptTokens)
			}
		})
	}
}
