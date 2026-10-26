package billing

import (
	"testing"
	"time"

	"github.com/ifnodoraemon/airoute/internal/storage"
)

// An unpriced model falls back to the engine's default record. That fallback
// must be billed at its face rate: it used to carry OffPeakEnabled=true with a
// 50% custom discount (00:00-08:30 + whole weekend), so every model nobody had
// priced was silently billed at half price overnight and all weekend.
func TestUnpricedModelIsNotDiscounted(t *testing.T) {
	engine := NewEngine(nil) // no repository: only the hardcoded default applies

	shanghai := shanghaiLoc
	night := time.Date(2026, 10, 24, 3, 0, 0, 0, shanghai)   // Saturday 03:00
	weekendNoon := time.Date(2026, 10, 24, 12, 0, 0, 0, shanghai) // Saturday 12:00
	weekdayNoon := time.Date(2026, 10, 23, 12, 0, 0, 0, shanghai) // Friday 12:00

	for _, tc := range []struct {
		name string
		at   time.Time
	}{
		{"weeknight", night},
		{"weekend-noon", weekendNoon},
		{"weekday-noon", weekdayNoon},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cost, _, _, isOffPeak, discount, _ := engine.CalculateCostForUnits("never-configured", "default", 1_000_000, 0, 0, 1, tc.at)
			if isOffPeak {
				t.Errorf("unpriced model was treated as off-peak (discount %.2f)", discount)
			}
			if want := 2.0; cost != want {
				t.Errorf("cost = %v, want %v at the default face rate", cost, want)
			}
		})
	}
}

// The discount mechanism itself must still work for a rule that opts in.
func TestConfiguredOffPeakRuleStillDiscounts(t *testing.T) {
	engine := &BillingEngine{
		prices: map[string]*storage.ModelPriceRecord{
			"default:discounted": {
				Model: "discounted", GroupName: "default",
				PromptPrice: 2, CompletionPrice: 8, CacheReadPrice: 0.2, Currency: "CNY",
				OffPeakEnabled: true, OffPeakMode: "custom", OffPeakDiscount: 0.5,
				OffPeakStart: "00:00", OffPeakEnd: "08:30",
			},
		},
		groupMultipliers: map[string]float64{"default": 1.0},
		deflt:            &storage.ModelPriceRecord{Model: "*", PromptPrice: 2, CompletionPrice: 8, CacheReadPrice: 0.2, Currency: "CNY"},
	}
	at := time.Date(2026, 10, 23, 3, 0, 0, 0, shanghaiLoc) // Friday 03:00, inside the window
	cost, _, _, isOffPeak, discount, _ := engine.CalculateCostForUnits("discounted", "default", 1_000_000, 0, 0, 1, at)
	if !isOffPeak {
		t.Fatal("a rule that opts into off-peak was not discounted")
	}
	if discount != 0.5 {
		t.Fatalf("discount = %v, want 0.5", discount)
	}
	if want := 1.0; cost != want {
		t.Fatalf("cost = %v, want %v (2.0 at 50%%)", cost, want)
	}
}
