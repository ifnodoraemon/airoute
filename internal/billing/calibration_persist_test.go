package billing

import (
	"testing"

	"github.com/ifnodoraemon/airoute/internal/storage"
)

func TestCalibrationPersistsAcrossRestarts(t *testing.T) {
	db, err := storage.OpenDB(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	repo := storage.NewRepository(db)

	first := NewEstimateCalibrator()
	for i := 0; i < 60; i++ {
		first.Observe("m", 100, 120)
	}
	if err := SaveCalibration(repo, first); err != nil {
		t.Fatalf("save: %v", err)
	}

	// Simulate a restart: a fresh calibrator must inherit the learned ratio.
	restarted := NewEstimateCalibrator()
	if err := LoadCalibration(repo, restarted); err != nil {
		t.Fatalf("load: %v", err)
	}
	ratio, samples := restarted.Ratio("m")
	if samples != 60 {
		t.Fatalf("samples = %d, want 60 after a restart", samples)
	}
	if ratio < 1.15 || ratio > 1.25 {
		t.Fatalf("ratio = %.3f, want ≈1.20 to survive the restart", ratio)
	}
}

// A replica that has observed more samples must not be rolled back by a stale
// snapshot written by a peer.
func TestRestoreKeepsRicherLocalCalibration(t *testing.T) {
	calibrator := NewEstimateCalibrator()
	for i := 0; i < 100; i++ {
		calibrator.Observe("m", 100, 130)
	}
	calibrator.Restore(map[string]RatioState{"m": {Ratio: 0.5, Samples: 10}})
	ratio, samples := calibrator.Ratio("m")
	if samples != 100 {
		t.Fatalf("samples = %d, want the local 100", samples)
	}
	if ratio < 1.2 {
		t.Fatalf("ratio = %.3f, want the local value to win over the stale snapshot", ratio)
	}
}
