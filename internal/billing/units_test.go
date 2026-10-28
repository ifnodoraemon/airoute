package billing

import (
	"github.com/ifnodoraemon/airoute/internal/storage"
	"testing"
	"time"
)

func TestUnitPricingUsesOneRateSnapshot(t *testing.T) {
	db, err := storage.OpenDB(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	r := storage.NewRepository(db)
	base := storage.ModelPriceRecord{Model: "unit-model", GroupName: "default", FixedPrice: 10, PromptPrice: 2, CompletionPrice: 4, Currency: "CNY"}
	for _, tc := range []struct {
		name, priceGroup string
		units            int
		offPeak          bool
		want             float64
	}{
		{"one discounted unit", "default", 1, false, 12.8},
		{"two discounted units", "default", 2, false, 20.8},
		{"override is not discounted twice", "vip", 2, false, 26},
		{"override with off-peak", "vip", 2, true, 13},
		{"base group with off-peak", "default", 2, true, 10.4},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_ = r.DeleteModelPriceWithGroup(base.Model, "vip")
			p := base
			p.GroupName = tc.priceGroup
			p.OffPeakEnabled = tc.offPeak
			p.WeekendAllDay = true
			p.OffPeakDiscount = .5
			if err := r.SaveModelPrice(&p); err != nil {
				t.Fatal(err)
			}
			e := NewEngine(r)
			cost, _, _, _, discount, snapshot := e.CalculateCostForUnitsWithCacheCreation(base.Model, "vip", 1_000_000, 1_000_000, 0, 0, tc.units, time.Date(2026, 10, 24, 12, 0, 0, 0, time.UTC))
			if cost != tc.want {
				t.Fatalf("want %v, got %v", tc.want, cost)
			}
			if snapshot == nil || snapshot.BillableUnits != tc.units || snapshot.OffPeakDiscount != discount {
				t.Fatalf("missing applied pricing snapshot: %+v", snapshot)
			}
		})
	}
}
