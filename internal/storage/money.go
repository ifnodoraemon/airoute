package storage

import "math"

// NanoPerUnit is the number of nano-units (1e-9) in one currency unit.
const NanoPerUnit int64 = 1_000_000_000

// ToNano converts a decimal currency amount into integer nano-units.
// Nano-units give 9 decimal places, far below any real per-token price,
// so conversions from the legacy REAL columns are effectively lossless.
func ToNano(amount float64) int64 {
	if amount == 0 || math.IsNaN(amount) || math.IsInf(amount, 0) {
		return 0
	}
	return int64(math.Round(amount * float64(NanoPerUnit)))
}

// FromNano converts integer nano-units back into a decimal amount.
func FromNano(nano int64) float64 {
	return float64(nano) / float64(NanoPerUnit)
}
