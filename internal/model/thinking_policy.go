package model

import (
	"fmt"
	"slices"
	"strings"
)

func ValidateModelCapabilities(config map[string]ModelCapabilityConfig) error {
	for name, value := range config {
		if strings.TrimSpace(name) == "" {
			return fmt.Errorf("upstream model name cannot be empty")
		}
		if value.Thinking == nil {
			continue
		}
		if len(value.Thinking.Efforts) > 32 {
			return fmt.Errorf("model %q: at most 32 thinking efforts may be configured", name)
		}
		_, ok := FindThinkingProfile(value.Thinking.Profile)
		if value.Thinking.Profile != "" && !ok {
			return fmt.Errorf("model %q: unknown thinking profile %q", name, value.Thinking.Profile)
		}
		for _, effort := range value.Thinking.Efforts {
			if !ValidThinkingEffort(effort) {
				return fmt.Errorf("model %q: profile does not support effort %q", name, effort)
			}
		}
	}
	return nil
}
func (p *ThinkingPolicy) Validate() error {
	if p == nil {
		return nil
	}
	mode := p.DefaultMode
	if mode == "" {
		mode = "auto"
	}
	if !slices.Contains([]string{"auto", "on", "off"}, mode) {
		return fmt.Errorf("invalid default thinking mode %q", mode)
	}
	for _, v := range p.AllowedModes {
		if !slices.Contains([]string{"auto", "on", "off"}, v) {
			return fmt.Errorf("invalid allowed thinking mode %q", v)
		}
	}
	if p.AllowedModes != nil && !slices.Contains(p.AllowedModes, mode) {
		return fmt.Errorf("default thinking mode must be allowed by route policy")
	}
	if p.AllowedEfforts != nil && mode == "auto" {
		return fmt.Errorf("set a route default thinking mode when restricting effort levels")
	}
	if p.DefaultEffort != "" && mode != "on" {
		return fmt.Errorf("default effort requires default mode on")
	}
	if p.DefaultEffort != "" && !ValidThinkingEffort(p.DefaultEffort) {
		return fmt.Errorf("invalid default thinking effort %q", p.DefaultEffort)
	}
	if p.DefaultEffort != "" && p.AllowedEfforts != nil && !slices.Contains(p.AllowedEfforts, p.DefaultEffort) {
		return fmt.Errorf("default thinking effort is not allowed by route policy")
	}
	for _, effort := range p.AllowedEfforts {
		if !ValidThinkingEffort(effort) {
			return fmt.Errorf("invalid thinking effort %q", effort)
		}
	}
	return nil
}
func (p *ThinkingPolicy) Resolve(control ThinkingControl) (ThinkingControl, error) {
	if control.Mode == "" {
		control.Mode = "auto"
	}
	if p == nil {
		return control, nil
	}
	if control.Mode == "auto" && p.DefaultMode != "" {
		control.Mode = p.DefaultMode
	}
	if control.Mode == "on" && control.Effort == "" {
		control.Effort = p.DefaultEffort
	}
	if p.AllowedModes != nil && !slices.Contains(p.AllowedModes, control.Mode) {
		return control, fmt.Errorf("thinking mode %q is forbidden by route policy", control.Mode)
	}
	if control.Effort != "" && p.AllowedEfforts != nil && !slices.Contains(p.AllowedEfforts, control.Effort) {
		return control, fmt.Errorf("thinking effort %q is forbidden by route policy", control.Effort)
	}
	return control, nil
}
