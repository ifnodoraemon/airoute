package controlplane

import (
	"context"
	"errors"
	"testing"

	"github.com/ifnodoraemon/airoute/internal/storage"
)

// TestValidatePriceRecordRejectsNegativeRates covers the rule that stops a
// negative rate from reaching the billing engine. A negative price was previously
// stored as given, which would bill the account backwards.
func TestValidatePriceRecordRejectsNegativeRates(t *testing.T) {
	cases := []struct {
		name string
		rec  *storage.ModelPriceRecord
	}{
		{"prompt", &storage.ModelPriceRecord{Model: "m", PromptPrice: -0.01}},
		{"completion", &storage.ModelPriceRecord{Model: "m", CompletionPrice: -1}},
		{"cache read", &storage.ModelPriceRecord{Model: "m", CacheReadPrice: -5}},
		{"fixed", &storage.ModelPriceRecord{Model: "m", FixedPrice: -100}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidatePriceRecord(tc.rec)

			var verr *ValidationError
			if !errors.As(err, &verr) {
				t.Fatalf("err = %v, want *ValidationError", err)
			}
		})
	}
}

func TestValidatePriceRecordAcceptsZeroAndPositiveRates(t *testing.T) {
	// Zero is legitimate: a free model, or a dimension the operator does not charge
	// for.
	rec := &storage.ModelPriceRecord{Model: "m", PromptPrice: 0, CompletionPrice: 1.5, CacheReadPrice: 0, FixedPrice: 0}
	if err := ValidatePriceRecord(rec); err != nil {
		t.Fatalf("zero rates were rejected: %v", err)
	}
	if err := ValidatePriceRecord(nil); err != nil {
		t.Fatalf("a nil record should be a no-op: %v", err)
	}
}

// TestPricingServiceSaveRateRejectsNegativePrices pins the pricing-screen path.
func TestPricingServiceSaveRateRejectsNegativePrices(t *testing.T) {
	store := &fakePricingStore{}
	svc := NewPricingService(store, nil, nil, nil)

	err := svc.SaveRate(context.Background(), &storage.ModelPriceRecord{Model: "gpt-x", PromptPrice: -1})

	var verr *ValidationError
	if !errors.As(err, &verr) {
		t.Fatalf("err = %v, want *ValidationError", err)
	}
	if store.saveCalls != 0 {
		t.Errorf("an invalid rate reached the store (%d saves)", store.saveCalls)
	}
}

// TestRouteServiceRejectsNegativePrices pins the route-editor path, which builds
// its own price row rather than going through SaveRate.
func TestRouteServiceRejectsNegativePrices(t *testing.T) {
	store := newFakeRouteStore()
	sync := &fakeRouteSync{}
	svc := NewRouteService(store, sync, nil)

	err := svc.UpdateModelRoute(context.Background(), &UpdateModelRouteRequest{
		Model:       "gpt-x",
		PromptPrice: priceFloat(-0.5),
	})

	var verr *ValidationError
	if !errors.As(err, &verr) {
		t.Fatalf("err = %v, want *ValidationError", err)
	}
	if store.applyCalls != 0 {
		t.Errorf("an invalid price reached the store (%d applies)", store.applyCalls)
	}
	if sync.calls != 0 {
		t.Errorf("an invalid price reloaded the data plane (%d calls)", sync.calls)
	}
}

// TestRouteServiceRejectsANegativePriceInheritedFromTheStoredRow covers the subtle
// case: the request does not mention the price, but the row it merges onto already
// holds a negative value, so the merged record must still be refused.
func TestRouteServiceRejectsANegativePriceInheritedFromTheStoredRow(t *testing.T) {
	store := newFakeRouteStore()
	store.seedPrice(&storage.ModelPriceRecord{Model: "gpt-x", GroupName: "default", PromptPrice: -3})
	svc := NewRouteService(store, nil, nil)

	err := svc.UpdateModelRoute(context.Background(), &UpdateModelRouteRequest{
		Model:           "gpt-x",
		CompletionPrice: priceFloat(2),
	})

	var verr *ValidationError
	if !errors.As(err, &verr) {
		t.Fatalf("err = %v, want *ValidationError", err)
	}
	if store.applyCalls != 0 {
		t.Errorf("an inherited negative price reached the store (%d applies)", store.applyCalls)
	}
}

// TestRouteServiceAcceptsPositivePrices is the control: the validation must not
// reject the ordinary case.
func TestRouteServiceAcceptsPositivePrices(t *testing.T) {
	store := newFakeRouteStore()
	svc := NewRouteService(store, nil, nil)

	if err := svc.UpdateModelRoute(context.Background(), &UpdateModelRouteRequest{
		Model:           "gpt-x",
		PromptPrice:     priceFloat(1),
		CompletionPrice: priceFloat(2),
	}); err != nil {
		t.Fatalf("UpdateModelRoute: %v", err)
	}
	if store.lastBundle.Price == nil {
		t.Fatal("no price row was built")
	}
	if store.lastBundle.Price.PromptPrice != 1 || store.lastBundle.Price.CompletionPrice != 2 {
		t.Errorf("price = %+v", store.lastBundle.Price)
	}
}

// --- off-peak slot validation follows the mode ---------------------------

// invalidOffPeakSlots is rejected by the slot validator itself: the discount is
// outside (0,1) and day 9 is out of range, so both "Discount" and "Days" checks
// are exercised.
const invalidOffPeakSlots = `[{"start":"00:00","end":"08:00","discount":2,"days":[9]}]`

// overlappingOffPeakSlots is structurally valid but its two windows collide on the
// same day, which is the overlap rule the console must not be able to save.
const overlappingOffPeakSlots = `[{"start":"00:00","end":"08:00","discount":0.5,"days":[1]},{"start":"06:00","end":"10:00","discount":0.5,"days":[1]}]`

func boolPtr(v bool) *bool { return &v }

// TestPricingServiceValidatesOffPeakSlotsOnlyInCustomMode pins C-10 on the pricing
// screen: the engine evaluates OffPeakSlots only when OffPeakMode is "custom", so
// validation must follow the same rule instead of rejecting configuration that can
// never influence a price.
func TestPricingServiceValidatesOffPeakSlotsOnlyInCustomMode(t *testing.T) {
	cases := []struct {
		name    string
		mode    string
		slots   string
		wantErr bool
	}{
		{"night mode ignores its slots", "night", invalidOffPeakSlots, false},
		{"fixed mode ignores its slots", "fixed", invalidOffPeakSlots, false},
		{"custom mode validates discount and days", "custom", invalidOffPeakSlots, true},
		{"custom mode rejects overlapping windows", "custom", overlappingOffPeakSlots, true},
		{"custom mode accepts valid slots", "custom", `[{"start":"00:00","end":"08:00","discount":0.5,"days":[1,2,3]}]`, false},
		{"the mode check is case-insensitive", "CUSTOM", invalidOffPeakSlots, true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			store := &fakePricingStore{}
			svc := NewPricingService(store, nil, nil, nil)

			err := svc.SaveRate(context.Background(), &storage.ModelPriceRecord{
				Model:          "gpt-x",
				PromptPrice:    1,
				OffPeakEnabled: true,
				OffPeakMode:    tc.mode,
				OffPeakSlots:   tc.slots,
			})

			if tc.wantErr {
				var verr *ValidationError
				if !errors.As(err, &verr) {
					t.Fatalf("err = %v, want *ValidationError", err)
				}
				if store.saveCalls != 0 {
					t.Errorf("invalid slots reached the store (%d saves)", store.saveCalls)
				}
				return
			}
			if err != nil {
				t.Fatalf("SaveRate: %v", err)
			}
			if store.saveCalls != 1 {
				t.Errorf("saveCalls = %d, want 1", store.saveCalls)
			}
		})
	}
}

// TestRouteServiceValidatesOffPeakSlotsOnlyInCustomMode is the same contract on the
// route editor, which builds its own price row instead of going through SaveRate.
func TestRouteServiceValidatesOffPeakSlotsOnlyInCustomMode(t *testing.T) {
	cases := []struct {
		name    string
		mode    string
		slots   string
		wantErr bool
	}{
		{"night mode ignores its slots", "night", invalidOffPeakSlots, false},
		{"custom mode validates discount and days", "custom", invalidOffPeakSlots, true},
		{"custom mode rejects overlapping windows", "custom", overlappingOffPeakSlots, true},
		{"custom mode accepts valid slots", "custom", `[{"start":"00:00","end":"08:00","discount":0.5,"days":[1]}]`, false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			store := newFakeRouteStore()
			sync := &fakeRouteSync{}
			svc := NewRouteService(store, sync, nil)

			err := svc.UpdateModelRoute(context.Background(), &UpdateModelRouteRequest{
				Model:          "gpt-x",
				PromptPrice:    priceFloat(1),
				OffPeakEnabled: boolPtr(true),
				OffPeakMode:    tc.mode,
				OffPeakSlots:   tc.slots,
			})

			if tc.wantErr {
				var verr *ValidationError
				if !errors.As(err, &verr) {
					t.Fatalf("err = %v, want *ValidationError", err)
				}
				if store.applyCalls != 0 {
					t.Errorf("invalid slots reached the store (%d applies)", store.applyCalls)
				}
				return
			}
			if err != nil {
				t.Fatalf("UpdateModelRoute: %v", err)
			}
			if store.lastBundle == nil || store.lastBundle.Price == nil {
				t.Fatal("no price row was built")
			}
			if store.lastBundle.Price.OffPeakSlots != tc.slots {
				t.Errorf("stored slots = %q, want %q", store.lastBundle.Price.OffPeakSlots, tc.slots)
			}
		})
	}
}
