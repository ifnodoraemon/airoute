package api

import (
	"testing"

	"github.com/ifnodoraemon/airoute/internal/model"
)

// C-02: prompt-cache writes are invoiced at a premium, so the unary Anthropic
// path must read cache_creation_input_tokens the way the streaming parser does.
func TestNativeCacheCreationTokens(t *testing.T) {
	cases := []struct {
		name  string
		usage any
		want  int
	}{
		{"present", map[string]any{"cache_creation_input_tokens": float64(128)}, 128},
		{"absent", map[string]any{"cache_read_input_tokens": float64(9)}, 0},
		{"nil usage", nil, 0},
		{"wrong type", "not-an-object", 0},
		{"zero", map[string]any{"cache_creation_input_tokens": float64(0)}, 0},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			if got := nativeCacheCreationTokens(testCase.usage); got != testCase.want {
				t.Errorf("nativeCacheCreationTokens(%v) = %d, want %d", testCase.usage, got, testCase.want)
			}
		})
	}
}

// The canonical usage struct carries the same number for adapted upstreams.
func TestCanonicalCacheCreationTokens(t *testing.T) {
	usage := &model.Usage{
		PromptTokens:        100,
		PromptTokensDetails: &model.PromptTokensDetails{CachedTokens: 10, CacheCreationTokens: 64},
	}
	if got := usage.GetCacheCreationTokens(); got != 64 {
		t.Fatalf("GetCacheCreationTokens = %d, want 64", got)
	}
}
