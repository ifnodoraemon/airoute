package billing

import (
	"testing"

	"github.com/ifnodoraemon/airoute/internal/storage"
)

// TestWildcardPricingIsDeterministic guards against the previous behaviour where
// wildcard rules were evaluated in Go map order, making charges non-reproducible.
func TestWildcardPricingIsDeterministic(t *testing.T) {
	e := &BillingEngine{
		prices: map[string]*storage.ModelPriceRecord{},
		deflt:  &storage.ModelPriceRecord{Model: "*", PromptPrice: 9, CompletionPrice: 9},
	}

	specific := &storage.ModelPriceRecord{Model: "gpt-4o-*", GroupName: "default", PromptPrice: 1, CompletionPrice: 1}
	broad := &storage.ModelPriceRecord{Model: "gpt-*", GroupName: "default", PromptPrice: 5, CompletionPrice: 5}
	catchAll := &storage.ModelPriceRecord{Model: "*", GroupName: "default", PromptPrice: 9, CompletionPrice: 9}

	e.prices["default:gpt-4o-*"] = specific
	e.prices["default:gpt-*"] = broad
	e.prices["default:*"] = catchAll
	e.wildcard = []wildcardRule{
		{group: "default", prefix: "gpt-4o-", price: specific},
		{group: "default", prefix: "gpt-", price: broad},
		{group: "default", prefix: "", price: catchAll},
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
