package billing

import (
	"testing"
	"time"

	"github.com/ifnodoraemon/nano-gateway/internal/storage"
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

func TestValidateSlotsOverlap(t *testing.T) {
	// 1. Same day overlap -> error
	overlapJSON := `[
		{"name":"时段1","start":"00:00","end":"08:30","days":[1,2,3]},
		{"name":"时段2","start":"08:00","end":"12:00","days":[3,4,5]}
	]`
	err := ValidateSlotsOverlap(overlapJSON)
	if err == nil {
		t.Fatal("expected error for overlapping slots on Wednesday (day 3), got nil")
	}

	// 2. Different days with same time -> allowed (nil)
	diffDaysJSON := `[
		{"name":"时段1","start":"08:00","end":"12:00","days":[1,2,3,4,5]},
		{"name":"时段2","start":"08:00","end":"12:00","days":[6,7]}
	]`
	if err := ValidateSlotsOverlap(diffDaysJSON); err != nil {
		t.Fatalf("expected nil for different days, got %v", err)
	}

	// 3. Contiguous seamless boundary (08:30 end, 08:30 start) -> allowed (nil)
	contiguousJSON := `[
		{"name":"时段1","start":"00:00","end":"08:30","days":[]},
		{"name":"时段2","start":"08:30","end":"12:00","days":[]}
	]`
	if err := ValidateSlotsOverlap(contiguousJSON); err != nil {
		t.Fatalf("expected nil for contiguous boundary, got %v", err)
	}

	// 4. Cross-midnight overlap with early morning
	crossMidnightOverlap := `[
		{"name":"夜间","start":"22:00","end":"06:00","days":[]},
		{"name":"早间","start":"05:00","end":"09:00","days":[]}
	]`
	if err := ValidateSlotsOverlap(crossMidnightOverlap); err == nil {
		t.Fatal("expected error for cross-midnight overlapping morning, got nil")
	}
}
