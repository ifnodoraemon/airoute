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

// Two replicas write the same setting. Each save must merge the stored ratios
// with its own snapshot, otherwise whichever replica saves last deletes the
// models the other one learned.
func TestSaveCalibrationMergesReplicas(t *testing.T) {
	db, err := storage.OpenDB(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	repo := storage.NewRepository(db)

	replicaA := NewEstimateCalibrator()
	for i := 0; i < 40; i++ {
		replicaA.Observe("alpha-only", 100, 120)
	}
	for i := 0; i < 10; i++ {
		replicaA.Observe("shared", 100, 150)
	}
	replicaB := NewEstimateCalibrator()
	for i := 0; i < 80; i++ {
		replicaB.Observe("beta-only", 100, 200)
	}
	// The same model observed by both replicas: the richer sample set must win.
	for i := 0; i < 30; i++ {
		replicaB.Observe("shared", 100, 130)
	}

	if err := SaveCalibration(repo, replicaA); err != nil {
		t.Fatalf("save replica A: %v", err)
	}
	if err := SaveCalibration(repo, replicaB); err != nil {
		t.Fatalf("save replica B: %v", err)
	}
	// Round trip: replica A saving again must not drop replica B's models.
	if err := SaveCalibration(repo, replicaA); err != nil {
		t.Fatalf("re-save replica A: %v", err)
	}

	fresh := NewEstimateCalibrator()
	if err := LoadCalibration(repo, fresh); err != nil {
		t.Fatalf("load: %v", err)
	}
	if _, samples := fresh.Ratio("alpha-only"); samples != 40 {
		t.Fatalf("alpha-only samples = %d, want 40 (replica A's model was dropped)", samples)
	}
	if _, samples := fresh.Ratio("beta-only"); samples != 80 {
		t.Fatalf("beta-only samples = %d, want 80 (replica B's model was dropped)", samples)
	}
	ratio, samples := fresh.Ratio("shared")
	if samples != 30 {
		t.Fatalf("shared samples = %d, want the richer 30 from replica B", samples)
	}
	if ratio < 1.25 || ratio > 1.35 {
		t.Fatalf("shared ratio = %.3f, want replica B's ≈1.30", ratio)
	}
}

// An unreadable stored value must be left alone: it is the only cross-replica
// record of what was learned, so the save fails instead of replacing it.
func TestSaveCalibrationKeepsUnreadableStoredValue(t *testing.T) {
	db, err := storage.OpenDB(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	repo := storage.NewRepository(db)

	const corrupt = "{not-json"
	if err := repo.SetSetting(calibrationSettingKey, corrupt); err != nil {
		t.Fatal(err)
	}
	calibrator := NewEstimateCalibrator()
	for i := 0; i < 10; i++ {
		calibrator.Observe("m", 100, 110)
	}
	if err := SaveCalibration(repo, calibrator); err == nil {
		t.Fatal("expected an error instead of silently overwriting unreadable ratios")
	}
	if got := repo.GetSetting(calibrationSettingKey, ""); got != corrupt {
		t.Fatalf("stored ratios were overwritten: %q", got)
	}
}

// A replica that saves must pick up the models a peer had already learned, so the
// two replicas converge instead of waiting for a restart.
func TestSaveCalibrationAdoptsPeerSamples(t *testing.T) {
	db, err := storage.OpenDB(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	repo := storage.NewRepository(db)

	peer := NewEstimateCalibrator()
	for i := 0; i < 50; i++ {
		peer.Observe("peer-model", 100, 140)
	}
	if err := SaveCalibration(repo, peer); err != nil {
		t.Fatalf("save peer: %v", err)
	}

	local := NewEstimateCalibrator()
	if err := SaveCalibration(repo, local); err != nil {
		t.Fatalf("save local: %v", err)
	}
	if _, samples := local.Ratio("peer-model"); samples != 50 {
		t.Fatalf("peer-model samples = %d, want 50 adopted from the persisted snapshot", samples)
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
