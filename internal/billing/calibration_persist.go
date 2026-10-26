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
func SaveCalibration(repo *storage.Repository, calibrator *EstimateCalibrator) error {
	if repo == nil || calibrator == nil {
		return nil
	}
	snapshot := calibrator.Snapshot()
	if len(snapshot) == 0 {
		return nil
	}
	encoded, err := json.Marshal(snapshot)
	if err != nil {
		return err
	}
	return repo.SetSetting(calibrationSettingKey, string(encoded))
}
