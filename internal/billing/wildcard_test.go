package billing

import (
	"testing"

	"github.com/ifnodoraemon/airoute/internal/storage"
)

// TestWildcardPricingIsDeterministic guards against the previous behaviour where
// wildcard rules were evaluated in Go map order, making charges non-reproducible.
func TestWildcardPricingIsDeterministic(t *testing.T) {
	specific := &storage.ModelPriceRecord{Model: "gpt-4o-*", GroupName: "default", PromptPrice: 1, CompletionPrice: 1}
	broad := &storage.ModelPriceRecord{Model: "gpt-*", GroupName: "default", PromptPrice: 5, CompletionPrice: 5}
	catchAll := &storage.ModelPriceRecord{Model: "*", GroupName: "default", PromptPrice: 9, CompletionPrice: 9}

	e := &BillingEngine{
		prices: map[string]*storage.ModelPriceRecord{},
		deflt:  &storage.ModelPriceRecord{Model: "*", PromptPrice: 7, CompletionPrice: 7},
		// Pre-sorted most-specific first, exactly what ReloadPrices builds.
		wildcard: []wildcardRule{
			{group: "default", pattern: "gpt-4o-*", price: specific},
			{group: "default", pattern: "gpt-*", price: broad},
			{group: "default", pattern: "*", price: catchAll},
		},
	}

	// Repeated lookups must always select the most specific rule.
	for i := 0; i < 50; i++ {
		got := e.GetPriceWithGroup("gpt-4o-mini", "default")
		if got == nil || got.PromptPrice != 1 {
			t.Fatalf("iteration %d: expected specific gpt-4o-* price, got %+v", i, got)
		}
	}

	got := e.GetPriceWithGroup("gpt-3.5", "default")
	if got == nil || got.PromptPrice != 5 {
		t.Fatalf("expected broad gpt-* price, got %+v", got)
	}

	got = e.GetPriceWithGroup("claude-3", "default")
	if got == nil || got.PromptPrice != 9 {
		t.Fatalf("expected catch-all price, got %+v", got)
	}
}

// Pricing accepted only a trailing "*" and stored every other pattern as an exact
// model name, so "*o1" and "gpt-4o*mini" could be saved and never matched. The
// pattern language now matches the one the validation rules use.
func TestMatchModelPattern(t *testing.T) {
	cases := []struct {
		pattern string
		model   string
		want    bool
	}{
		{"gpt-4o", "gpt-4o", true},
		{"gpt-4o", "gpt-4o-mini", false},
		{"gpt-4o*", "gpt-4o-mini", true},
		{"gpt-4o*", "gpt-4o", true},
		{"*o1", "openai/o1", true},
		{"*o1", "o1", true},
		{"*o1", "o1-mini", false},
		{"gpt-4o*mini", "gpt-4o-mini", true},
		{"gpt-4o*mini", "gpt-4o-2024", false},
		{"*", "anything/at-all", true},
		{"claude-3-5-sonnet*", "claude-3-5-sonnet-20241022", true},
		{"GPT-4O*", "gpt-4o-mini", true}, // patterns and models are lowercased
	}
	for _, tc := range cases {
		if got := MatchModelPattern(tc.pattern, tc.model); got != tc.want {
			t.Errorf("MatchModelPattern(%q, %q) = %v, want %v", tc.pattern, tc.model, got, tc.want)
		}
	}
}

// The end-to-end case from the audit: a leading and an interior wildcard must price
// a request instead of silently falling back to the default rate card.
func TestWildcardPricingSupportsLeadingAndInfixPatterns(t *testing.T) {
	leading := &storage.ModelPriceRecord{Model: "*o1", GroupName: "default", PromptPrice: 3, CompletionPrice: 6}
	infix := &storage.ModelPriceRecord{Model: "gpt-4o*mini", GroupName: "default", PromptPrice: 8, CompletionPrice: 16}
	e := &BillingEngine{
		prices: map[string]*storage.ModelPriceRecord{},
		deflt:  &storage.ModelPriceRecord{Model: "*", PromptPrice: 2, CompletionPrice: 8},
		wildcard: []wildcardRule{
			{group: "default", pattern: "gpt-4o*mini", price: infix},
			{group: "default", pattern: "*o1", price: leading},
		},
	}

	got := e.GetPriceWithGroup("openai/o1", "default")
	if got == nil || got.PromptPrice != 3 {
		t.Fatalf("expected *o1 to price openai/o1, got %+v", got)
	}
	if source := e.ResolvePriceSource("openai/o1", "default"); source != PriceSourceWildcard {
		t.Fatalf("source = %q, want %q", source, PriceSourceWildcard)
	}

	got = e.GetPriceWithGroup("gpt-4o-mini", "default")
	if got == nil || got.PromptPrice != 8 {
		t.Fatalf("expected gpt-4o*mini to price gpt-4o-mini, got %+v", got)
	}

	if got = e.GetPriceWithGroup("claude-3", "default"); got == nil || got.PromptPrice != 2 {
		t.Fatalf("expected the default rate for an unmatched model, got %+v", got)
	}
}

// ReloadPrices must index every pattern the same way the resolver matches it, so a
// pattern saved through the admin API is reachable from the live engine.
func TestReloadPricesIndexesEveryWildcardForm(t *testing.T) {
	db, err := storage.OpenDB(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	repo := storage.NewRepository(db)
	for _, rec := range []*storage.ModelPriceRecord{
		{Model: "*o1", GroupName: "default", PromptPrice: 3, CompletionPrice: 6, Currency: "CNY"},
		{Model: "gpt-4o*mini", GroupName: "default", PromptPrice: 8, CompletionPrice: 16, Currency: "CNY"},
		{Model: "gpt-*", GroupName: "default", PromptPrice: 5, CompletionPrice: 5, Currency: "CNY"},
	} {
		if err := repo.SaveModelPrice(rec); err != nil {
			t.Fatalf("save %s: %v", rec.Model, err)
		}
	}
	engine := NewEngine(repo)

	for model, want := range map[string]float64{
		"openai/o1":     3,
		"gpt-4o-mini":   8,
		"gpt-4o":        5,
		"gpt-3.5-turbo": 5,
	} {
		if got := engine.GetPriceWithGroup(model, "default"); got == nil || got.PromptPrice != want {
			t.Errorf("%s: prompt price = %+v, want %v", model, got, want)
		}
	}
}
