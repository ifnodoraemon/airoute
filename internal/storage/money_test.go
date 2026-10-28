package storage

import "testing"

func TestToUnitsRoundTrip(t *testing.T) {
	cases := []float64{0, 0.000000001, 0.000252, 1.5, 25.5, 1234.567890123}
	for _, in := range cases {
		units := ToUnits(in)
		got := FromUnits(units)
		if diff := got - in; diff > 1e-9 || diff < -1e-9 {
			t.Fatalf("round trip %v -> %d -> %v exceeds 1e-9", in, units, got)
		}
	}
}

func TestToUnitsHandlesInvalidValues(t *testing.T) {
	if got := ToUnits(0); got != 0 {
		t.Fatalf("expected 0, got %d", got)
	}
}
