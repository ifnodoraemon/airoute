package config

import (
	"strings"
	"testing"

	"github.com/ifnodoraemon/airoute/internal/model"
)

// legacyRuleMatches is the matcher the validation rules used before the glob was
// unified with pricing (internal/model.MatchModelPattern). It is kept here, in the
// test, as the reference: every pattern that used to work must still resolve to the
// same rule, otherwise an operator's existing config silently changes behaviour.
func legacyRuleMatches(pat, modelLower string) bool {
	pat = strings.ToLower(strings.TrimSpace(pat))
	if pat == "*" || pat == "" {
		return true
	}
	if strings.HasSuffix(pat, "*") {
		return strings.HasPrefix(modelLower, strings.TrimSuffix(pat, "*"))
	}
	if strings.HasPrefix(pat, "*") {
		return strings.HasSuffix(modelLower, strings.TrimPrefix(pat, "*"))
	}
	return pat == modelLower
}

// The validation rules now share one glob implementation with pricing. This pins
// equivalence with the previous behaviour for every pattern shape the old matcher
// actually supported (exact, prefix, suffix, catch-all, empty), and documents the
// one intended widening: infix patterns like "gpt-4o*mini" used to be treated as
// literal names and now match.
func TestValidationRuleGlobMatchesLegacyBehaviour(t *testing.T) {
	cfg := &Config{}
	cfg.ModelValidation.DefaultLevel = "off"

	// Rule order matters: the first matching rule wins, as before.
	for _, rule := range []ModelValidationRule{
		{Model: "claude-*", Protocol: "anthropic", Level: "strict"},
		{Model: "o1*", Protocol: "openai", Level: "strict", DisallowTemperature: true},
		{Model: "*flash", Level: "lenient"},
		{Model: "gpt-4o", Level: "lenient"},
		{Model: "", Level: "off"}, // empty pattern: catch-all, last so it never wins
	} {
		cfg.ModelValidation.Rules = append(cfg.ModelValidation.Rules, rule)
	}

	patterns := []string{"claude-*", "o1*", "*flash", "gpt-4o", "*", ""}
	models := []string{
		"claude-3-5-sonnet-20241022",
		"anthropic/claude-3-5-sonnet",
		"o1",
		"o1-preview",
		"openai/o1-preview",
		"gemini-2.0-flash",
		"gpt-4o",
		"gpt-4o-mini",
		"unlisted-model",
	}

	for _, pat := range patterns {
		for _, name := range models {
			legacy := legacyRuleMatches(pat, strings.ToLower(name))
			// The production path: GetValidationRule maps an empty pattern to "*",
			// so mirror that normalisation before calling the shared matcher.
			normalized := strings.ToLower(strings.TrimSpace(pat))
			if normalized == "" {
				normalized = "*"
			}
			unified := model.MatchModelPattern(normalized, name)
			if legacy != unified {
				t.Errorf("pattern %q vs model %q: legacy=%v unified=%v", pat, name, legacy, unified)
			}
		}
	}

	// End-to-end through GetValidationRule: the shapes above solve to the same rule
	// as the legacy matcher would have selected.
	cases := []struct {
		model    string
		protocol string
		wantLvl  string
		wantRule string
	}{
		{"claude-3-5-sonnet", "anthropic", "strict", "claude-*"},
		{"o1-preview", "openai", "strict", "o1*"},
		{"gemini-2.0-flash", "", "lenient", "*flash"},
		{"gpt-4o", "", "lenient", "gpt-4o"},
		// No rule matches: default level, no rule reported.
		{"unlisted-model", "", "off", ""},
	}
	for _, tc := range cases {
		lvl, rule := cfg.GetValidationRule(tc.model, tc.protocol)
		if lvl != tc.wantLvl {
			t.Errorf("GetValidationRule(%q) level = %q, want %q", tc.model, lvl, tc.wantLvl)
		}
		gotRule := ""
		if rule != nil {
			gotRule = rule.Model
		}
		if gotRule != tc.wantRule {
			t.Errorf("GetValidationRule(%q) rule = %q, want %q", tc.model, gotRule, tc.wantRule)
		}
	}
}

// TestValidationRuleInfixPatternIsHonoured documents the intended widening: the old
// matcher treated "gpt-4o*mini" as a literal model name (so the rule never fired),
// while the shared glob language - the one pricing already documents - matches it.
func TestValidationRuleInfixPatternIsHonoured(t *testing.T) {
	cfg := &Config{}
	cfg.ModelValidation.Rules = []ModelValidationRule{
		{Model: "gpt-4o*mini", Level: "strict"},
	}
	if lvl, rule := cfg.GetValidationRule("gpt-4o-mini", "openai"); lvl != "strict" || rule == nil {
		t.Fatalf("infix pattern did not match: level=%q rule=%v", lvl, rule)
	}
	if lvl, rule := cfg.GetValidationRule("gpt-4o", "openai"); lvl != "off" || rule != nil {
		t.Fatalf("infix pattern matched a model it should not: level=%q rule=%v", lvl, rule)
	}
}
