package api

import (
	"testing"

	"github.com/ifnodoraemon/airoute/internal/storage"
)

func TestNormalizeUnbillableCurrency(t *testing.T) {
	cases := []struct {
		name       string
		snapshot   *storage.UsagePricingSnapshot
		wantCost   float64
		wantSaved  float64
		wantReject bool
	}{
		{"nil snapshot keeps the charge", nil, 1.5, 0.2, false},
		{"empty currency is treated as CNY", &storage.UsagePricingSnapshot{Currency: ""}, 1.5, 0.2, false},
		{"CNY keeps the charge", &storage.UsagePricingSnapshot{Currency: "CNY"}, 1.5, 0.2, false},
		{"lower-case cny keeps the charge", &storage.UsagePricingSnapshot{Currency: "cny"}, 1.5, 0.2, false},
		{"USD zeroes the charge", &storage.UsagePricingSnapshot{Currency: "USD"}, 0, 0, true},
		{"legacy EUR zeroes the charge", &storage.UsagePricingSnapshot{Currency: "EUR"}, 0, 0, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cost, saved, rejected := normalizeUnbillableCurrency(1.5, 0.2, tc.snapshot)
			if cost != tc.wantCost || saved != tc.wantSaved || rejected != tc.wantReject {
				t.Fatalf("got cost=%v saved=%v rejected=%v, want %v/%v/%v",
					cost, saved, rejected, tc.wantCost, tc.wantSaved, tc.wantReject)
			}
		})
	}
}
