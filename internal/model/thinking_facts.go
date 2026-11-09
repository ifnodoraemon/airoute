package model

import (
	"fmt"
	"slices"
)

// Keep an explicit empty level set distinct from an omitted one in YAML exports.
func (config ThinkingConfig) MarshalYAML() (any, error) {
	fields := make(map[string]any)
	if config.Profile != "" {
		fields["profile"] = config.Profile
	}
	if config.CanDisable != nil {
		fields["can_disable"] = *config.CanDisable
	}
	if config.Efforts != nil {
		fields["efforts"] = config.Efforts
	}
	return fields, nil
}

// Operator declarations may extend catalog facts, but every selected control
// must have a wire representation. Budget adapters require an explicit mapping.
func validateThinkingFacts(profile ThinkingProfile, config *ThinkingConfig) error {
	transport := ThinkingTransportProfile(profile)
	if config.CanDisable != nil && *config.CanDisable && !transport.Off {
		return fmt.Errorf("the model's parameter adapter does not support disabling thinking")
	}
	for _, effort := range config.Efforts {
		if !slices.Contains(transport.Efforts, effort) && !(passthroughThinkingEffort(profile) && ValidThinkingEffort(effort)) {
			return fmt.Errorf("the model's parameter adapter does not support effort %q", effort)
		}
	}
	return nil
}

func applyThinkingFacts(profile ThinkingProfile, config *ThinkingConfig, source string) (ThinkingProfile, string) {
	if err := validateThinkingFacts(profile, config); err != nil {
		return ThinkingProfile{}, "invalid_configuration"
	}
	if config.CanDisable != nil {
		profile.Off = *config.CanDisable
	}
	if config.Efforts != nil {
		profile.Efforts = slices.Clone(config.Efforts)
		// With no selectable levels, retain the adapter's fixed default for an
		// explicit on request. Do not fabricate an adjustable level in discovery.
		if len(profile.Efforts) > 0 && !slices.Contains(profile.Efforts, profile.DefaultEffort) {
			profile.DefaultEffort = profile.Efforts[0]
		}
	}
	if len(profile.Efforts) > 0 {
		profile.On = true
	}
	if config.Profile != "" || config.CanDisable != nil || config.Efforts != nil {
		source = "configured"
	}
	return profile, source
}
