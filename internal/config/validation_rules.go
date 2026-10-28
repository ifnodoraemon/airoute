package config

import (
	"strings"

	"github.com/ifnodoraemon/airoute/internal/model"
)

// ModelValidationRule defines per-model format validation rules.
type ModelValidationRule struct {
	Model               string `yaml:"model" json:"model"`                               // glob/prefix pattern, e.g. "claude-3-5-sonnet*", "o1*"
	Protocol            string `yaml:"protocol" json:"protocol"`                         // "openai", "anthropic", "gemini" (optional)
	Level               string `yaml:"level" json:"level"`                               // "off", "lenient", "strict"
	DisallowTemperature bool   `yaml:"disallow_temperature" json:"disallow_temperature"` // e.g. for o1/o3 reasoning models
}

// ModelValidationConfig defines format validation configurations.
type ModelValidationConfig struct {
	DefaultLevel string                `yaml:"default_level" json:"default_level"` // "off" | "lenient" | "strict" (default: "off")
	Rules        []ModelValidationRule `yaml:"rules" json:"rules"`
}

// GetValidationRule resolves the validation rule and level for a given model and protocol.
func (c *Config) GetValidationRule(modelName, protocol string) (string, *ModelValidationRule) {
	defaultLvl := "off"
	if c != nil && c.ModelValidation.DefaultLevel != "" {
		defaultLvl = c.ModelValidation.DefaultLevel
	}

	if c == nil || len(c.ModelValidation.Rules) == 0 {
		return defaultLvl, nil
	}

	modelLower := strings.ToLower(strings.TrimSpace(modelName))
	protoLower := strings.ToLower(strings.TrimSpace(protocol))

	for i := range c.ModelValidation.Rules {
		r := &c.ModelValidation.Rules[i]
		// Protocol match check if rule specifies protocol
		if r.Protocol != "" && protoLower != "" && !strings.EqualFold(r.Protocol, protoLower) {
			continue
		}

		// One pattern language for the whole repository: the glob implementation
		// lives in internal/model (MatchModelPattern) and is shared with pricing.
		// The rules used to carry their own prefix/suffix/exact matcher, which
		// drifted from pricing's definition (and rejected infix patterns like
		// "gpt-4o*mini" by treating them as exact names). An empty pattern still
		// means "all models", which is how these rules have always behaved.
		pat := strings.ToLower(strings.TrimSpace(r.Model))
		if pat == "" {
			pat = "*"
		}
		if model.MatchModelPattern(pat, modelLower) {
			lvl := r.Level
			if lvl == "" {
				lvl = defaultLvl
			}
			return lvl, r
		}
	}

	return defaultLvl, nil
}

// ResolveValidationLevel resolves the effective validation level and rule given model, protocol, and an optional key config.
// Individual API keys or explicit per-model rules can enable validation ("strict" or "lenient"), while defaulting to "off".
func (c *Config) ResolveValidationLevel(modelName, protocol string, keyCfg *model.APIKeyConfig) (string, *ModelValidationRule) {
	level, rule := c.GetValidationRule(modelName, protocol)
	if keyCfg != nil && keyCfg.FormatValidation != "" {
		level = strings.ToLower(strings.TrimSpace(keyCfg.FormatValidation))
	}
	return level, rule
}
