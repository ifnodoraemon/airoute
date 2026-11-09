package provider

import (
	"strings"

	"github.com/ifnodoraemon/airoute/internal/model"
)

func geminiThinkingFields(prepared *model.PreparedThinking) map[string]any {
	fields := make(map[string]any)
	control, profile := prepared.Control, prepared.Profile
	if profile.Wire != "gemma" && control.IncludeSummary != nil {
		fields["include_thoughts"] = *control.IncludeSummary
	}
	if control.Mode == "auto" {
		return fields
	}
	effort := control.Effort
	if effort == "" {
		effort = profile.DefaultEffort
	}
	switch profile.Wire {
	case "gemini-level", "gemini-summary":
		if control.Mode == "off" {
			fields["thinking_budget"] = 0
		} else {
			fields["thinking_level"] = effort
		}
	case "gemini-budget":
		budget := profile.Budgets[effort]
		if control.Mode == "off" {
			budget = 0
		}
		fields["thinking_budget"] = budget
	case "gemma":
		if control.Mode == "off" {
			effort = "minimal"
		}
		fields["thinking_level"] = effort
	default:
		fields["thinking_level"] = effort
	}
	return fields
}

func applyPreparedGeminiThinking(cfg *GeminiGenerationConfig, prepared *model.PreparedThinking) {
	if prepared.Profile.Wire == "none" {
		return
	}
	fields := geminiThinkingFields(prepared)
	if len(fields) == 0 {
		return
	}
	cfg.ThinkingConfig = &GeminiThinkingConfig{}
	if level, ok := fields["thinking_level"].(string); ok {
		cfg.ThinkingConfig.ThinkingLevel = strings.ToUpper(level)
	}
	if budget, ok := fields["thinking_budget"].(int); ok {
		cfg.ThinkingConfig.ThinkingBudget = &budget
	}
	if include, ok := fields["include_thoughts"].(bool); ok {
		cfg.ThinkingConfig.IncludeThoughts = include
	}
}
