package storage_test

import (
	"testing"
	"time"

	"github.com/ifnodoraemon/airoute/internal/billing"
	"github.com/ifnodoraemon/airoute/internal/storage"
)

func newOffPeakRepo(t *testing.T) *storage.Repository {
	t.Helper()
	db, err := storage.OpenDB(":memory:")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return storage.NewRepository(db)
}

// Regression for the synthetic off-peak slot: normalizeModelPrice used to
// overwrite an operator's legacy single window with a hardcoded
// 00:00-08:30 @0.5 slot. billing.CustomOffPeakStrategy only honours
// off_peak_start/off_peak_end when the slot list is empty, so the injected slot
// silently moved the discount window (and its rate).
func TestLegacyOffPeakWindowSurvivesSave(t *testing.T) {
	repo := newOffPeakRepo(t)

	rec := &storage.ModelPriceRecord{
		Model:           "legacy-window-model",
		PromptPrice:     1,
		CompletionPrice: 2,
		OffPeakEnabled:  true,
		OffPeakMode:     "custom",
		OffPeakStart:    "22:00",
		OffPeakEnd:      "06:00",
		OffPeakDiscount: 0.25,
		OffPeakSlots:    "", // legacy single-window configuration
	}
	if err := repo.SaveModelPrice(rec); err != nil {
		t.Fatalf("save price: %v", err)
	}

	stored, err := repo.GetModelPriceExact("legacy-window-model", "default")
	if err != nil {
		t.Fatalf("reload price: %v", err)
	}
	if stored.OffPeakSlots != "" {
		t.Fatalf("save replaced the legacy window with synthetic slots %q", stored.OffPeakSlots)
	}
	if stored.OffPeakStart != "22:00" || stored.OffPeakEnd != "06:00" || stored.OffPeakDiscount != 0.25 {
		t.Fatalf("legacy window not preserved: start=%q end=%q discount=%.2f",
			stored.OffPeakStart, stored.OffPeakEnd, stored.OffPeakDiscount)
	}

	engine := billing.NewEngine(repo)
	cases := []struct {
		name     string
		hour     int
		wantOff  bool
		wantDisc float64
	}{
		{"inside configured window (23:00)", 23, true, 0.25},
		{"inside configured window (03:00)", 3, true, 0.25},
		{"outside configured window (12:00)", 12, false, 1.0},
		// The synthetic slot used to make 00:00-08:30 off-peak; 07:00 is inside
		// that range but outside the operator's 22:00-06:00 window.
		{"outside configured window (07:00)", 7, false, 1.0},
	}
	for _, tc := range cases {
		at := time.Date(2026, 10, 27, tc.hour, 0, 0, 0, time.FixedZone("CST", 8*3600))
		isOff, discount := engine.IsOffPeak(stored, at)
		if isOff != tc.wantOff || discount != tc.wantDisc {
			t.Errorf("%s: got isOff=%v discount=%.2f, want isOff=%v discount=%.2f",
				tc.name, isOff, discount, tc.wantOff, tc.wantDisc)
		}
	}
}

// An explicit slot list must still be stored verbatim and take precedence over
// the legacy window.
func TestExplicitOffPeakSlotsArePersistedVerbatim(t *testing.T) {
	repo := newOffPeakRepo(t)

	slots := `[{"start":"01:00","end":"05:00","discount":0.4}]`
	rec := &storage.ModelPriceRecord{
		Model:          "slotted-model",
		OffPeakEnabled: true,
		OffPeakMode:    "custom",
		OffPeakStart:   "00:00",
		OffPeakEnd:     "08:30",
		OffPeakSlots:   slots,
	}
	if err := repo.SaveModelPrice(rec); err != nil {
		t.Fatalf("save price: %v", err)
	}
	stored, err := repo.GetModelPriceExact("slotted-model", "default")
	if err != nil {
		t.Fatalf("reload price: %v", err)
	}
	if stored.OffPeakSlots != slots {
		t.Fatalf("slug list rewritten: got %q want %q", stored.OffPeakSlots, slots)
	}
}

// A zero discount factor means "no discount", not "apply the historical 50%
// default". The old normalization rewrote it to 0.5, so a rule enabled without
// an explicit coefficient silently billed half price.
func TestZeroOffPeakDiscountMeansNoDiscount(t *testing.T) {
	repo := newOffPeakRepo(t)
	rec := &storage.ModelPriceRecord{
		Model:           "zero-discount-model",
		PromptPrice:     1,
		CompletionPrice: 2,
		OffPeakEnabled:  true,
		OffPeakMode:     "custom",
		OffPeakStart:    "22:00",
		OffPeakEnd:      "06:00",
		OffPeakDiscount: 0,
	}
	if err := repo.SaveModelPrice(rec); err != nil {
		t.Fatalf("save price: %v", err)
	}
	stored, err := repo.GetModelPriceExact("zero-discount-model", "default")
	if err != nil {
		t.Fatalf("reload price: %v", err)
	}
	if stored.OffPeakDiscount != 1.0 {
		t.Fatalf("stored discount = %.2f, want 1.0 (no discount)", stored.OffPeakDiscount)
	}

	engine := billing.NewEngine(repo)
	at := time.Date(2026, 10, 27, 23, 0, 0, 0, time.FixedZone("CST", 8*3600))
	isOff, discount := engine.IsOffPeak(stored, at)
	if !isOff {
		t.Fatal("23:00 must still be inside the configured off-peak window")
	}
	if discount != 1.0 {
		t.Fatalf("discount = %.2f, want 1.0 (no discount)", discount)
	}
}
