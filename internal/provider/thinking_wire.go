package provider

import (
	"encoding/json"

	"github.com/ifnodoraemon/airoute/internal/model"
)

func applyThinkingWire(req *model.ChatCompletionRequest, channel *model.ChannelConfig) error {
	switch req.PreparedThinking.Profile.Wire {
	case "openai":
		return applyOpenAIThinkingWire(req)
	case "deepseek":
		return applyDeepSeekThinkingWire(req)
	case "qwen":
		return applyQwenThinkingWire(req)
	case "glm":
		return applyGLMThinkingWire(req)
	case "deepseek-template":
		return applyDeepSeekTemplateThinkingWire(req)
	case "ollama":
		return applyOllamaThinkingWire(req)
	case "ollama-level":
		return applyOllamaLevelThinkingWire(req)
	case "template":
		return applyTemplateThinkingWire(req)
	case "gemini-level", "gemini-budget", "gemma", "gemini-summary":
		return applyGeminiThinkingWire(req, channel)
	case "claude-budget":
		return applyClaudeBudgetThinkingWire(req)
	case "claude-adaptive":
		return applyClaudeAdaptiveThinkingWire(req)
	}
	return nil
}

func preparedThinkingEffort(prepared *model.PreparedThinking) string {
	effort := prepared.Control.Effort
	if effort == "" && prepared.Control.Mode == "on" {
		effort = prepared.Profile.DefaultEffort
	}
	return effort
}

func applyOpenAIThinkingWire(req *model.ChatCompletionRequest) error {
	on := req.PreparedThinking.Control.Mode == "on"
	off := req.PreparedThinking.Control.Mode == "off"
	effort := preparedThinkingEffort(req.PreparedThinking)
	set := func(key string, value any) { req.Extra[key], _ = json.Marshal(value) }
	if off {
		effort = "none"
	}
	if on || off {
		set("reasoning_effort", effort)
	}
	if on || off {
		// Reasoning models use the combined answer+reasoning output limit.
		if req.MaxTokens != nil {
			set("max_completion_tokens", *req.MaxTokens)
			req.MaxTokens = nil
		}
		if on {
			req.Temperature, req.TopP, req.PresencePenalty, req.FrequencyPenalty = nil, nil, nil, nil
		}
	}
	return nil
}

func applyDeepSeekThinkingWire(req *model.ChatCompletionRequest) error {
	on := req.PreparedThinking.Control.Mode == "on"
	off := req.PreparedThinking.Control.Mode == "off"
	effort := preparedThinkingEffort(req.PreparedThinking)
	set := func(key string, value any) { req.Extra[key], _ = json.Marshal(value) }
	if on || off {
		kind := "enabled"
		if off {
			kind = "disabled"
		}
		set("thinking", map[string]string{"type": kind})
	}
	if on && effort != "" {
		set("reasoning_effort", effort)
	}
	return nil
}

func applyQwenThinkingWire(req *model.ChatCompletionRequest) error {
	control := req.PreparedThinking.Control
	profile := req.PreparedThinking.Profile
	on := req.PreparedThinking.Control.Mode == "on"
	off := req.PreparedThinking.Control.Mode == "off"
	set := func(key string, value any) { req.Extra[key], _ = json.Marshal(value) }
	if on || off {
		set("enable_thinking", on)
	}
	if on && control.Effort != "" {
		set("thinking_budget", profile.Budgets[control.Effort])
	}
	return nil
}

func applyGLMThinkingWire(req *model.ChatCompletionRequest) error {
	on := req.PreparedThinking.Control.Mode == "on"
	off := req.PreparedThinking.Control.Mode == "off"
	effort := preparedThinkingEffort(req.PreparedThinking)
	set := func(key string, value any) { req.Extra[key], _ = json.Marshal(value) }
	if off {
		set("thinking", map[string]string{"type": "disabled"})
	}
	if on {
		set("reasoning_effort", effort)
	}
	return nil
}

func applyOllamaThinkingWire(req *model.ChatCompletionRequest) error {
	on := req.PreparedThinking.Control.Mode == "on"
	off := req.PreparedThinking.Control.Mode == "off"
	set := func(key string, value any) { req.Extra[key], _ = json.Marshal(value) }
	if on || off {
		value := "medium"
		if off {
			value = "none"
		}
		set("reasoning_effort", value)
	}
	return nil
}

func applyOllamaLevelThinkingWire(req *model.ChatCompletionRequest) error {
	on := req.PreparedThinking.Control.Mode == "on"
	off := req.PreparedThinking.Control.Mode == "off"
	effort := preparedThinkingEffort(req.PreparedThinking)
	set := func(key string, value any) { req.Extra[key], _ = json.Marshal(value) }
	if off {
		set("reasoning_effort", "none")
	} else if on {
		set("reasoning_effort", effort)
	}
	return nil
}
