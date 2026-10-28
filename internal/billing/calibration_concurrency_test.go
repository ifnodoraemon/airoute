package billing

import (
	"sync"
	"testing"
)

// BUG-DATA-04 regression: Adjust used to copy the ratioSample pointer under the
// read lock and then read its fields after unlocking, while Observe writes them
// under the write lock. Both run on the request path, so this is a real data race
// (`go test -race` reported calibration.go:78 write vs :118 read before the fix).
//
// The test is a normal regression case: it passes with and without -race now that
// Adjust snapshots the fields inside the lock.
func TestCalibratorAdjustIsRaceFree(t *testing.T) {
	cal := NewEstimateCalibrator()
	const model = "race-model"
	// Seed enough samples that Adjust no longer takes the early-return path.
	for i := 0; i < calibrationMinSamples+1; i++ {
		cal.Observe(model, 100, 120)
	}
	// Sanity check the seeded ratio is in effect and inside the clamp band.
	if got := cal.Adjust(model, 100); got < 100 || got > 250 {
		t.Fatalf("seeded Adjust(100) = %d, want within 100..250", got)
	}

	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		for i := 0; i < 5000; i++ {
			cal.Observe(model, 100, 100+i%50)
		}
	}()
	go func() {
		defer wg.Done()
		for i := 0; i < 5000; i++ {
			if got := cal.Adjust(model, 100); got < 1 {
				t.Errorf("Adjust returned %d, want >= 1", got)
			}
		}
	}()
	wg.Wait()
}

// Concurrent Observe/Adjust on many models exercises the same shared-sample path
// with map growth, which would surface a lock-order or re-use bug under -race.
func TestCalibratorConcurrentObserveAdjustAcrossModels(t *testing.T) {
	cal := NewEstimateCalibrator()
	const workers = 8

	var wg sync.WaitGroup
	for w := 0; w < workers; w++ {
		model := "model-" + string(rune('a'+w))
		wg.Add(2)
		go func() {
			defer wg.Done()
			for i := 0; i < 500; i++ {
				cal.Observe(model, 200, 180+i%40)
			}
		}()
		go func() {
			defer wg.Done()
			for i := 0; i < 500; i++ {
				if got := cal.Adjust(model, 200); got < 1 {
					t.Errorf("%s: Adjust returned %d, want >= 1", model, got)
				}
			}
		}()
	}
	wg.Wait()

	snapshot := cal.Snapshot()
	if len(snapshot) != workers {
		t.Fatalf("snapshot has %d models, want %d", len(snapshot), workers)
	}
	for model, state := range snapshot {
		if state.Samples == 0 || state.Ratio <= 0 {
			t.Fatalf("%s: snapshot %+v, want positive ratio and samples", model, state)
		}
	}
}
