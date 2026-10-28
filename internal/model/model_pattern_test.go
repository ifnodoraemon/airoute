package model

import "testing"

// The pattern language is shared by pricing (internal/billing) and the format
// validation rules (internal/config), so its semantics are pinned here once: a
// pattern an operator writes in either place must mean the same thing.
func TestMatchModelPattern(t *testing.T) {
	cases := []struct {
		pattern string
		model   string
		want    bool
	}{
		// exact
		{"gpt-4o", "gpt-4o", true},
		{"gpt-4o", "gpt-4o-mini", false},
		{"gpt-4o", "gpt", false},

		// prefix wildcard
		{"o1*", "o1", true},
		{"o1*", "o1-preview", true},
		{"o1*", "openai/o1", false},
		{"claude-*", "claude-3-5-sonnet-20241022", true},
		{"claude-*", "anthropic/claude-3-5-sonnet", false},

		// suffix wildcard
		{"*o1", "o1", true},
		{"*o1", "openai/o1", true},
		{"*o1", "o1-preview", false},
		{"*flash", "gemini-2.0-flash", true},

		// infix wildcard
		{"x*y", "xy", true},
		{"x*y", "xAy", true},
		{"x*y", "x/y", true},
		{"x*y", "xAyz", false},
		{"gpt-4o*mini", "gpt-4o-mini", true},
		{"gpt-4o*mini", "gpt-4o", false},

		// catch-all
		{"*", "gpt-4o", true},
		{"*", "openai/o1-preview", true},
		{"*", "", true},
		{"**", "anything", true},

		// a "*" spans the provider prefix separator
		{"openai/*", "openai/o1", true},
		{"openai/*", "azure/o1", false},
		{"*/o1", "openai/o1", true},
		{"*/o1", "openai/o3", false},

		// case-insensitive and whitespace tolerant (config/model values are
		// lowercased before lookup, but the helper must not depend on that)
		{"O1*", "o1-preview", true},
		{"  gpt-4o  ", " GPT-4O ", true},
		{"Claude-*", "claude-3", true},

		// no wildcard means exact, never a substring match
		{"o1", "o1-preview", false},
		{"4o", "gpt-4o", false},
	}
	for _, tc := range cases {
		if got := MatchModelPattern(tc.pattern, tc.model); got != tc.want {
			t.Errorf("MatchModelPattern(%q, %q) = %v, want %v", tc.pattern, tc.model, got, tc.want)
		}
	}
}
