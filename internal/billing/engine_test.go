package billing

import (
	"testing"
	"time"

	"github.com/ifnodoraemon/airoute/internal/storage"
)

func TestBillingEngine_IsOffPeak_CustomMultiSlots(t *testing.T) {
	loc, _ := time.LoadLocation("Asia/Shanghai")
	engine := &BillingEngine{}

	rec := &storage.ModelPriceRecord{
		Model:           "custom-test",
		OffPeakEnabled:  true,
		OffPeakMode:     "custom",
		OffPeakDiscount: 0.5,
		WeekendAllDay:   false,
		// Slot 1: Weekdays 00:00 - 08:00, discount 0.5
		// Slot 2: Wednesday (day 3) flash sale 02:00 - 04:00, discount 0.2 (conflict with slot 1!)
		// Slot 3: Sunday (day 7) 14:00 - 18:00, discount 0.4
		OffPeakSlots: `[
			{"start":"00:00","end":"08:00","discount":0.5,"days":[1,2,3,4,5]},
			{"start":"02:00","end":"04:00","discount":0.2,"days":[3]},
			{"start":"14:00","end":"18:00","discount":0.4,"days":[7]}
		]`,
	}

	// 1. Monday (day 1) 03:00 -> matches Slot 1 -> 0.5 discount
	mon0300 := time.Date(2026, 9, 21, 3, 0, 0, 0, loc) // 2026-09-21 is Monday
	isOff, discount := engine.IsOffPeak(rec, mon0300)
	if !isOff || discount != 0.5 {
		t.Fatalf("expected off-peak 0.5 on Monday 03:00, got isOff=%v discount=%f", isOff, discount)
	}

	// 2. Wednesday (day 3) 03:00 -> conflicts Slot 1 (0.5) and Slot 2 (0.2) -> Best Discount Rule gives 0.2
	wed0300 := time.Date(2026, 9, 23, 3, 0, 0, 0, loc) // 2026-09-23 is Wednesday
	isOff, discount = engine.IsOffPeak(rec, wed0300)
	if !isOff || discount != 0.2 {
		t.Fatalf("expected conflict resolution to choose best discount 0.2 on Wednesday 03:00, got isOff=%v discount=%f", isOff, discount)
	}

	// 3. Wednesday (day 3) 05:00 -> only Slot 1 applies -> 0.5 discount
	wed0500 := time.Date(2026, 9, 23, 5, 0, 0, 0, loc)
	isOff, discount = engine.IsOffPeak(rec, wed0500)
	if !isOff || discount != 0.5 {
		t.Fatalf("expected off-peak 0.5 on Wednesday 05:00, got isOff=%v discount=%f", isOff, discount)
	}

	// 4. Wednesday 10:00 -> daytime peak, neither matches -> full price 1.0
	wed1000 := time.Date(2026, 9, 23, 10, 0, 0, 0, loc)
	isOff, discount = engine.IsOffPeak(rec, wed1000)
	if isOff || discount != 1.0 {
		t.Fatalf("expected peak (false, 1.0) on Wednesday 10:00, got isOff=%v discount=%f", isOff, discount)
	}

	// 5. Sunday (day 7) 03:00 -> Slot 1 only applies to days 1-5, so false
	sun0300 := time.Date(2026, 9, 27, 3, 0, 0, 0, loc) // 2026-09-27 is Sunday
	isOff, discount = engine.IsOffPeak(rec, sun0300)
	if isOff || discount != 1.0 {
		t.Fatalf("expected Sunday 03:00 to be peak (false, 1.0), got isOff=%v discount=%f", isOff, discount)
	}

	// 6. Sunday 15:00 -> matches Slot 3 -> 0.4 discount
	sun1500 := time.Date(2026, 9, 27, 15, 0, 0, 0, loc)
	isOff, discount = engine.IsOffPeak(rec, sun1500)
	if !isOff || discount != 0.4 {
		t.Fatalf("expected Sunday 15:00 off-peak 0.4, got isOff=%v discount=%f", isOff, discount)
	}
}

func TestBillingEngine_IsOffPeak_CrossMidnight(t *testing.T) {
	loc, _ := time.LoadLocation("Asia/Shanghai")
	engine := &BillingEngine{}

	rec := &storage.ModelPriceRecord{
		Model:          "cross-midnight-test",
		OffPeakEnabled: true,
		OffPeakMode:    "custom",
		// Cross-midnight: 22:00 to 06:00
		OffPeakSlots: `[{"start":"22:00","end":"06:00","discount":0.4}]`,
	}

	// 23:30 should be off-peak
	t2330 := time.Date(2026, 9, 23, 23, 30, 0, 0, loc)
	isOff, discount := engine.IsOffPeak(rec, t2330)
	if !isOff || discount != 0.4 {
		t.Fatalf("expected 23:30 to be off-peak 0.4, got isOff=%v discount=%f", isOff, discount)
	}

	// 04:30 should be off-peak
	t0430 := time.Date(2026, 9, 24, 4, 30, 0, 0, loc)
	isOff, discount = engine.IsOffPeak(rec, t0430)
	if !isOff || discount != 0.4 {
		t.Fatalf("expected 04:30 to be off-peak 0.4, got isOff=%v discount=%f", isOff, discount)
	}

	// 07:00 should not be off-peak
	t0700 := time.Date(2026, 9, 24, 7, 0, 0, 0, loc)
	isOff, discount = engine.IsOffPeak(rec, t0700)
	if isOff || discount != 1.0 {
		t.Fatalf("expected 07:00 to be peak, got isOff=%v discount=%f", isOff, discount)
	}
}

// Slots are validated through ValidateOffPeakSlots in "custom" mode: that is the
// only mode whose strategy reads them, so it is the mode a write path must pass
// for the overlap rules to run at all.
func TestValidateOffPeakSlotsOverlap(t *testing.T) {
	// 1. Same day overlap -> error
	overlapJSON := `[
		{"name":"时段1","start":"00:00","end":"08:30","discount":0.5,"days":[1,2,3]},
		{"name":"时段2","start":"08:00","end":"12:00","discount":0.6,"days":[3,4,5]}
	]`
	err := ValidateOffPeakSlots("custom", overlapJSON)
	if err == nil {
		t.Fatal("expected error for overlapping slots on Wednesday (day 3), got nil")
	}

	// 2. Different days with same time -> allowed (nil)
	diffDaysJSON := `[
		{"name":"时段1","start":"08:00","end":"12:00","discount":0.5,"days":[1,2,3,4,5]},
		{"name":"时段2","start":"08:00","end":"12:00","discount":0.5,"days":[6,7]}
	]`
	if err := ValidateOffPeakSlots("custom", diffDaysJSON); err != nil {
		t.Fatalf("expected nil for different days, got %v", err)
	}

	// 3. Contiguous seamless boundary (08:30 end, 08:30 start) -> allowed (nil)
	contiguousJSON := `[
		{"name":"时段1","start":"00:00","end":"08:30","discount":0.5,"days":[]},
		{"name":"时段2","start":"08:30","end":"12:00","discount":0.5,"days":[]}
	]`
	if err := ValidateOffPeakSlots("custom", contiguousJSON); err != nil {
		t.Fatalf("expected nil for contiguous boundary, got %v", err)
	}

	// 4. Cross-midnight overlap with early morning
	crossMidnightOverlap := `[
		{"name":"夜间","start":"22:00","end":"06:00","discount":0.5,"days":[]},
		{"name":"早间","start":"05:00","end":"09:00","discount":0.5,"days":[]}
	]`
	if err := ValidateOffPeakSlots("custom", crossMidnightOverlap); err == nil {
		t.Fatal("expected error for cross-midnight overlapping morning, got nil")
	}
}

// The write path used to accept slot values the engine cannot honour: a day
// outside 1-7 made the slot never match, and an out-of-range discount silently
// fell back to the row discount. Both are rejected now.
func TestValidateSlotsRejectsValuesTheEngineCannotHonour(t *testing.T) {
	cases := map[string]string{
		"day zero":            `[{"start":"00:00","end":"08:00","discount":0.5,"days":[0]}]`,
		"day past sunday":     `[{"start":"00:00","end":"08:00","discount":0.5,"days":[8]}]`,
		"zero discount":       `[{"start":"00:00","end":"08:00","discount":0,"days":[1]}]`,
		"negative discount":   `[{"start":"00:00","end":"08:00","discount":-0.5,"days":[1]}]`,
		"surcharge discount":  `[{"start":"00:00","end":"08:00","discount":1.5,"days":[1]}]`,
		"full-price slot":     `[{"start":"00:00","end":"08:00","discount":1,"days":[1]}]`,
		"equal start and end": `[{"start":"08:00","end":"08:00","discount":0.5}]`,
		"hour out of range":   `[{"start":"00:00","end":"25:00","discount":0.5}]`,
	}
	for name, slots := range cases {
		if err := ValidateOffPeakSlots("custom", slots); err == nil {
			t.Errorf("%s: expected a validation error, got nil", name)
		}
	}

	// A single valid slot is accepted even though there is nothing to overlap.
	if err := ValidateOffPeakSlots("custom", `[{"start":"00:00","end":"08:00","discount":0.5,"days":[1,2,3]}]`); err != nil {
		t.Fatalf("valid single slot rejected: %v", err)
	}
}

// Slots are only read by the "custom" strategy, so validation has to follow the
// mode the engine will evaluate them under.
func TestValidateOffPeakSlotsFollowsMode(t *testing.T) {
	overlapping := `[
		{"start":"00:00","end":"10:00","discount":0.5,"days":[1]},
		{"start":"05:00","end":"12:00","discount":0.6,"days":[1]}
	]`
	if err := ValidateOffPeakSlots("custom", overlapping); err == nil {
		t.Fatal("expected overlap to be rejected in custom mode")
	}
	// The engine ignores slots for these modes, so the same configuration is not an
	// error there (and the fields it does read are validated elsewhere).
	for _, mode := range []string{"night", "none", ""} {
		if err := ValidateOffPeakSlots(mode, overlapping); err != nil {
			t.Errorf("mode %q: slots are not read by the engine, got %v", mode, err)
		}
	}
	if err := ValidateOffPeakSlots("custom", ""); err != nil {
		t.Fatalf("empty slot list in custom mode should be valid, got %v", err)
	}
}

// An invalid slot discount must mean "no discount", exactly like the row-level
// clamp - not "fall back to the row discount", which turned a typo into half
// price (1.5 meaning "surcharge" and 0 meaning "no discount" both billed 0.5).
func TestBillingEngine_InvalidSlotDiscountIsNotFallenBack(t *testing.T) {
	engine := &BillingEngine{}
	at := time.Date(2026, 10, 27, 12, 0, 0, 0, time.FixedZone("CST", 8*3600))
	for _, discount := range []string{"1.5", "0"} {
		rec := &storage.ModelPriceRecord{
			Model:           "slot-discount",
			OffPeakEnabled:  true,
			OffPeakMode:     "custom",
			OffPeakDiscount: 0.5, // the row discount an invalid slot used to inherit
			OffPeakSlots:    `[{"start":"11:00","end":"13:00","discount":` + discount + `}]`,
		}
		isOff, got := engine.IsOffPeak(rec, at)
		if !isOff || got != 1.0 {
			t.Fatalf("slot discount %s: isOff=%v discount=%.2f, want true/1.0 (no discount)", discount, isOff, got)
		}
	}
}

func TestCalculateCostWithGroupMultipliers(t *testing.T) {
	loc, _ := time.LoadLocation("Asia/Shanghai")
	peakTime := time.Date(2026, 9, 23, 14, 0, 0, 0, loc)

	engine := &BillingEngine{
		prices: map[string]*storage.ModelPriceRecord{
			"default:gpt-base": {
				Model:           "gpt-base",
				GroupName:       "default",
				PromptPrice:     10.0,
				CompletionPrice: 20.0,
			},
			"vip:gpt-override": {
				Model:           "gpt-override",
				GroupName:       "vip",
				PromptPrice:     5.0,
				CompletionPrice: 10.0,
			},
			"default:gpt-override": {
				Model:           "gpt-override",
				GroupName:       "default",
				PromptPrice:     10.0,
				CompletionPrice: 20.0,
			},
		},
		groupMultipliers: map[string]float64{
			"default":    1.0,
			"vip":        0.8,
			"enterprise": 1.5,
		},
	}

	// 1M prompt + 1M completion on gpt-base:
	// Base cost = 10 + 20 = 30 CNY

	// Case 1: default group -> 30 CNY, saved 0
	cost, saved, _, isOff, _, _ := engine.CalculateCostForUnitsWithCacheCreation("gpt-base", "default", 1_000_000, 1_000_000, 0, 0, 1, peakTime)
	if isOff || cost != 30.0 || saved != 0.0 {
		t.Fatalf("expected cost=30.0, saved=0.0 for default, got cost=%f, saved=%f", cost, saved)
	}

	// Case 2: vip group on base model (no explicit vip rule) -> inherits base * 0.8 -> 24 CNY, saved 6 CNY
	costVip, savedVip, _, _, _, _ := engine.CalculateCostForUnitsWithCacheCreation("gpt-base", "vip", 1_000_000, 1_000_000, 0, 0, 1, peakTime)
	if costVip != 24.0 || savedVip != 6.0 {
		t.Fatalf("expected cost=24.0, saved=6.0 for vip multiplier, got cost=%f, saved=%f", costVip, savedVip)
	}

	// Case 3: enterprise group on base model -> inherits base * 1.5 -> 45 CNY, saved 0
	costEnt, savedEnt, _, _, _, _ := engine.CalculateCostForUnitsWithCacheCreation("gpt-base", "enterprise", 1_000_000, 1_000_000, 0, 0, 1, peakTime)
	if costEnt != 45.0 || savedEnt != 0.0 {
		t.Fatalf("expected cost=45.0, saved=0.0 for enterprise, got cost=%f, saved=%f", costEnt, savedEnt)
	}

	// Case 4: explicit vip override exists -> uses explicit override rate without multiplying again
	costOverride, _, _, _, _, _ := engine.CalculateCostForUnitsWithCacheCreation("gpt-override", "vip", 1_000_000, 1_000_000, 0, 0, 1, peakTime)
	if costOverride != 15.0 {
		t.Fatalf("expected cost=15.0 for explicit vip override (5 + 10), got %f", costOverride)
	}
}

// Defensive clamp for rows constructed outside the storage normalizer (legacy
// rows, tests, config imports): a zero factor must not become a 50% discount.
func TestBillingEngine_ZeroDiscountIsNoDiscount(t *testing.T) {
	engine := &BillingEngine{}
	rec := &storage.ModelPriceRecord{
		Model:           "zero-discount",
		OffPeakEnabled:  true,
		OffPeakMode:     "custom",
		OffPeakStart:    "00:00",
		OffPeakEnd:      "23:59",
		OffPeakDiscount: 0,
	}
	at := time.Date(2026, 10, 27, 12, 0, 0, 0, time.FixedZone("CST", 8*3600))
	isOff, discount := engine.IsOffPeak(rec, at)
	if !isOff || discount != 1.0 {
		t.Fatalf("isOff=%v discount=%.2f, want true/1.0", isOff, discount)
	}
}
