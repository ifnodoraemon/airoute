package provider

import (
	"github.com/ifnodoraemon/airoute/internal/model"
)

func applyPreparedAnthropicThinking(request *AnthropicRequest, prepared *model.PreparedThinking) {
	if prepared == nil {
		return
	}
	control, profile := prepared.Control, prepared.Profile
	if profile.Wire != "claude-budget" && profile.Wire != "claude-adaptive" {
		return
	}
	if control.Mode == "auto" {
		return
	}
	if control.Mode == "off" {
		request.Thinking = map[string]string{"type": "disabled"}
		return
	}
	effort := control.Effort
	if effort == "" {
		effort = profile.DefaultEffort
	}
	if profile.Wire == "claude-adaptive" {
		request.Thinking = map[string]string{"type": "adaptive"}
		request.OutputConfig = map[string]string{"effort": effort}
	} else if profile.Wire == "claude-budget" {
		request.Thinking = map[string]any{"type": "enabled", "budget_tokens": profile.Budgets[effort]}
	}
}
