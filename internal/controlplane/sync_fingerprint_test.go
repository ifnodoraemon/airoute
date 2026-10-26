package controlplane

import (
	"testing"

	"github.com/ifnodoraemon/airoute/internal/model"
)

// The periodic reload log is suppressed when the digest is unchanged. Usage
// counters are part of the key record but move with every billed request, so
// they must not participate: otherwise the digest changes on every reload and
// the INFO line returns every 10 seconds.
func TestSnapshotFingerprintIgnoresUsageCounters(t *testing.T) {
	before := &ConfigSnapshot{
		Channels:  []model.ChannelConfig{{Name: "ch", Models: []string{"m"}}},
		APIKeys:   []model.APIKeyConfig{{Key: "sk-1", TenantID: "t", RPM: 60, UsedCost: 1.5, UsedCostUnits: 1500000000, UserBalance: 10}},
		HasKeys:   true,
		Fallbacks: map[string]string{"a": "b"},
	}
	after := &ConfigSnapshot{
		Channels:  []model.ChannelConfig{{Name: "ch", Models: []string{"m"}}},
		APIKeys:   []model.APIKeyConfig{{Key: "sk-1", TenantID: "t", RPM: 60, UsedCost: 9.25, UsedCostUnits: 9250000000, UserBalance: 2}},
		HasKeys:   true,
		Fallbacks: map[string]string{"a": "b"},
	}
	if snapshotFingerprint(before) != snapshotFingerprint(after) {
		t.Fatal("fingerprint changed although only usage counters moved")
	}
}

// A real configuration change must still change the digest, otherwise the
// reload that applies it would be logged as a no-op.
func TestSnapshotFingerprintTracksConfiguration(t *testing.T) {
	base := &ConfigSnapshot{
		Channels: []model.ChannelConfig{{Name: "ch", Models: []string{"m"}}},
		APIKeys:  []model.APIKeyConfig{{Key: "sk-1", RPM: 60}},
		HasKeys:  true,
	}
	cases := map[string]func(*ConfigSnapshot){
		"rpm":       func(s *ConfigSnapshot) { s.APIKeys[0].RPM = 120 },
		"status":    func(s *ConfigSnapshot) { s.APIKeys[0].Status = "disabled" },
		"channel":   func(s *ConfigSnapshot) { s.Channels[0].Name = "renamed" },
		"key-added": func(s *ConfigSnapshot) { s.APIKeys = append(s.APIKeys, model.APIKeyConfig{Key: "sk-2"}) },
		"has-keys":  func(s *ConfigSnapshot) { s.HasKeys = false },
		"fallback":  func(s *ConfigSnapshot) { s.Fallbacks = map[string]string{"a": "c"} },
	}
	want := snapshotFingerprint(base)
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			changed := &ConfigSnapshot{
				Channels: []model.ChannelConfig{{Name: "ch", Models: []string{"m"}}},
				APIKeys:  []model.APIKeyConfig{{Key: "sk-1", RPM: 60}},
				HasKeys:  true,
			}
			mutate(changed)
			if snapshotFingerprint(changed) == want {
				t.Fatalf("configuration change %q was invisible to the fingerprint", name)
			}
		})
	}
}
