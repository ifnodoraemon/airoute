package storage

import "math"

// UnitsPerCurrency is the number of integer sub-units (1e-9) in one currency unit.
const UnitsPerCurrency int64 = 1_000_000_000

// ToUnits converts a decimal currency amount into integer 1e-9 sub-units.
// Integer sub-units give 9 decimal places, far below any real per-token price,
// so conversions from the legacy REAL columns are effectively lossless.
func ToUnits(amount float64) int64 {
	if amount == 0 || math.IsNaN(amount) || math.IsInf(amount, 0) {
		return 0
	}
	return int64(math.Round(amount * float64(UnitsPerCurrency)))
}

// FromUnits converts integer 1e-9 sub-units back into a decimal amount.
func FromUnits(units int64) float64 {
	return float64(units) / float64(UnitsPerCurrency)
}
