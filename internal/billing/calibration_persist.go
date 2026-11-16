package billing

import (
	"encoding/json"

	"github.com/ifnodoraemon/airoute/internal/storage"
	"github.com/ifnodoraemon/airoute/internal/telemetry"
)

// calibrationSettingKey is where the learned tokenizer ratios live.
const calibrationSettingKey = "billing_token_calibration"

// LoadCalibration seeds the calibrator from the persisted ratios.
//
// Ratios must outlive a process: they describe how each upstream tokenizer counts
// text relative to our local heuristic, which does not change when the gateway
// restarts, and two replicas must charge the same way. Learning them separately
// per replica (and resetting on every deploy) is the opposite of strict
// alignment with the provider invoice.
func LoadCalibration(repo *storage.Repository, calibrator *EstimateCalibrator) error {
	if repo == nil || calibrator == nil {
		return nil
	}
	raw := repo.GetSetting(calibrationSettingKey, "")
	if raw == "" {
		return nil
	}
	var states map[string]RatioState
	if err := json.Unmarshal([]byte(raw), &states); err != nil {
		return err
	}
	calibrator.Restore(states)
	telemetry.ScopedLogger("billing").Info("loaded token estimate calibration", "models", len(states))
	return nil
}

// SaveCalibration persists the current ratios so a restart keeps them.
//
// The write is a read-merge-write, not an overwrite. Every replica runs its own
// calibrator and saves on a 5-minute timer (plus every calibrationPersistEvery
// observations), so two replicas overwriting the whole map would each delete the
// models the other had learned; the merge previously only happened once, at
// startup. Merging by sample count keeps the better-observed ratio per model -
// the same rule Restore applies - and the merged view is also restored locally so
// a replica that has just started catches up without waiting for a restart.
//
// A stored value that cannot be parsed is never overwritten: it is the only
// cross-replica record of what was learned, so replacing it with this replica's
// partial view would destroy it. The error is returned and logged at WARN.
func SaveCalibration(repo *storage.Repository, calibrator *EstimateCalibrator) error {
	if repo == nil || calibrator == nil {
		return nil
	}

	local := calibrator.Snapshot()
	var merged map[string]RatioState
	// The read-merge-write runs under a row lock (MergeSetting) so two replicas
	// saving at once cannot drop each other's learned models.
	err := repo.MergeSetting(calibrationSettingKey, func(current string) (string, error) {
		stored, err := parseCalibrationStates(current)
		if err != nil {
			return "", err
		}
		merged = mergeCalibrationStates(stored, local)
		if len(local) == 0 {
			return current, nil
		}
		encoded, err := json.Marshal(merged)
		if err != nil {
			return "", err
		}
		return string(encoded), nil
	})
	if err != nil {
		telemetry.ScopedLogger("billing").Warn(
			"keeping stored token calibration: merge failed", "error", err.Error())
		return err
	}

	// Adopt the peer-learned models this replica was missing. Restore keeps any
	// locally better-observed sample, so the sample count only ever grows.
	calibrator.Restore(merged)
	return nil
}

// parseCalibrationStates decodes a persisted ratio snapshot, returning an empty
// map when nothing has been saved yet.
func parseCalibrationStates(raw string) (map[string]RatioState, error) {
	if raw == "" {
		return map[string]RatioState{}, nil
	}
	var states map[string]RatioState
	if err := json.Unmarshal([]byte(raw), &states); err != nil {
		return nil, err
	}
	if states == nil {
		states = map[string]RatioState{}
	}
	return states, nil
}

// mergeCalibrationStates unions two ratio snapshots, keeping the better-observed
// ratio per model. A peer that has seen more upstream reports wins, which is what
// keeps the two replicas charging the same way instead of each trusting its own.
func mergeCalibrationStates(stored, local map[string]RatioState) map[string]RatioState {
	merged := make(map[string]RatioState, len(stored)+len(local))
	for model, state := range stored {
		merged[model] = state
	}
	for model, state := range local {
		if previous, ok := merged[model]; ok && previous.Samples >= state.Samples {
			continue
		}
		merged[model] = state
	}
	return merged
}
