package provider

import (
	"encoding/json"
	"fmt"

	"github.com/ifnodoraemon/airoute/internal/model"
)

func applyGeminiThinkingWire(req *model.ChatCompletionRequest, channel *model.ChannelConfig) error {
	set := func(key string, value any) { req.Extra[key], _ = json.Marshal(value) }
	if !model.IsGeminiProvider(channel.Type) {
		// Google's OpenAI-compatible endpoint uses this documented extension.
		config := geminiThinkingFields(req.PreparedThinking)
		if len(config) > 0 {
			var extra map[string]json.RawMessage
			if raw := req.Extra["extra_body"]; len(raw) > 0 {
				if err := json.Unmarshal(raw, &extra); err != nil {
					return &UnsupportedTranslationError{Reason: "extra_body must be an object"}
				}
			}
			if extra == nil {
				extra = make(map[string]json.RawMessage)
			}
			var google map[string]json.RawMessage
			if len(extra["google"]) > 0 && json.Unmarshal(extra["google"], &google) != nil {
				return &UnsupportedTranslationError{Reason: "extra_body.google must be an object"}
			}
			if google == nil {
				google = make(map[string]json.RawMessage)
			}
			google["thinking_config"], _ = json.Marshal(config)
			extra["google"], _ = json.Marshal(google)
			set("extra_body", extra)
		}
	}
	return nil
}

func applyClaudeBudgetThinkingWire(req *model.ChatCompletionRequest) error {
	profile := req.PreparedThinking.Profile
	on := req.PreparedThinking.Control.Mode == "on"
	effort := preparedThinkingEffort(req.PreparedThinking)
	if on {
		budget := profile.Budgets[effort]
		maxTokens := 4096
		if req.MaxTokens != nil {
			maxTokens = *req.MaxTokens
		}
		if budget >= maxTokens {
			return &UnsupportedTranslationError{Reason: fmt.Sprintf("thinking budget %d must be less than max_tokens (%d); raise the output limit or lower effort", budget, maxTokens)}
		}
		req.Temperature, req.TopP = nil, nil
	}
	return nil
}

func applyClaudeAdaptiveThinkingWire(req *model.ChatCompletionRequest) error {
	on := req.PreparedThinking.Control.Mode == "on"
	if on {
		req.Temperature, req.TopP = nil, nil
	}
	return nil
}
