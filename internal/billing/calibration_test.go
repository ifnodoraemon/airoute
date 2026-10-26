package billing

import "testing"

// Until the upstream has reported enough samples the raw heuristic is kept: a
// single odd request must not move what every customer pays.
func TestCalibratorIgnoresNoiseBeforeEnoughSamples(t *testing.T) {
	c := NewEstimateCalibrator()
	for i := 0; i < calibrationMinSamples-1; i++ {
		c.Observe("m", 100, 150)
	}
	if got := c.Adjust("m", 100); got != 100 {
		t.Fatalf("adjust = %d, want the raw estimate before the sample minimum", got)
	}
}

// The upstream tokenizer counting 20% more tokens must pull estimates up by the
// same ratio once calibration is established.
func TestCalibratorConvergesToUpstreamTokenizer(t *testing.T) {
	c := NewEstimateCalibrator()
	for i := 0; i < 60; i++ {
		c.Observe("m", 100, 120)
	}
	ratio, samples := c.Ratio("m")
	if samples == 0 {
		t.Fatal("no samples recorded")
	}
	if ratio < 1.15 || ratio > 1.25 {
		t.Fatalf("ratio = %.3f, want ≈1.20", ratio)
	}
	got := c.Adjust("m", 1000)
	if got < 1150 || got > 1250 {
		t.Fatalf("adjusted = %d, want ≈1200", got)
	}
}

// A handful of pathological reports cannot swing the ratio out of its band.
func TestCalibratorClampsOutliers(t *testing.T) {
	c := NewEstimateCalibrator()
	for i := 0; i < 50; i++ {
		c.Observe("m", 100, 1_000_000)
	}
	ratio, _ := c.Ratio("m")
	if ratio > calibrationCeiling {
		t.Fatalf("ratio = %.3f, want it clamped to %.1f", ratio, calibrationCeiling)
	}

	c2 := NewEstimateCalibrator()
	for i := 0; i < 50; i++ {
		c2.Observe("m", 100, 1)
	}
	ratio2, _ := c2.Ratio("m")
	if ratio2 < calibrationFloor {
		t.Fatalf("ratio = %.3f, want it clamped to %.1f", ratio2, calibrationFloor)
	}
}

func TestCalibratorIgnoresDegenerateInput(t *testing.T) {
	c := NewEstimateCalibrator()
	c.Observe("", 100, 120)
	c.Observe("m", 0, 120)
	c.Observe("m", 100, 0)
	if ratio, samples := c.Ratio("m"); samples != 0 || ratio != 1.0 {
		t.Fatalf("degenerate observations were recorded: ratio=%.2f samples=%d", ratio, samples)
	}
	if got := c.Adjust("m", 0); got != 0 {
		t.Fatalf("adjust(0) = %d, want 0", got)
	}
}
