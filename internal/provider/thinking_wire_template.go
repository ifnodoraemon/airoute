package provider

import (
	"encoding/json"

	"github.com/ifnodoraemon/airoute/internal/model"
)

func applyDeepSeekTemplateThinkingWire(req *model.ChatCompletionRequest) error {
	profile := req.PreparedThinking.Profile
	on := req.PreparedThinking.Control.Mode == "on"
	off := req.PreparedThinking.Control.Mode == "off"
	effort := preparedThinkingEffort(req.PreparedThinking)
	set := func(key string, value any) { req.Extra[key], _ = json.Marshal(value) }
	if on || off {
		var kwargs map[string]json.RawMessage
		if raw := req.Extra["chat_template_kwargs"]; len(raw) > 0 && json.Unmarshal(raw, &kwargs) != nil {
			return &UnsupportedTranslationError{Reason: "chat_template_kwargs must be an object"}
		}
		if kwargs == nil {
			kwargs = make(map[string]json.RawMessage)
		}
		kwargs["thinking"], _ = json.Marshal(on)
		if on && effort != "" {
			var value any = effort
			if numeric, ok := profile.Budgets[effort]; ok {
				value = numeric
			}
			kwargs["reasoning_effort"], _ = json.Marshal(value)
		}
		set("chat_template_kwargs", kwargs)
	}
	return nil
}

func applyTemplateThinkingWire(req *model.ChatCompletionRequest) error {
	on := req.PreparedThinking.Control.Mode == "on"
	off := req.PreparedThinking.Control.Mode == "off"
	set := func(key string, value any) { req.Extra[key], _ = json.Marshal(value) }
	if on || off {
		var kwargs map[string]json.RawMessage
		if raw := req.Extra["chat_template_kwargs"]; len(raw) > 0 {
			if err := json.Unmarshal(raw, &kwargs); err != nil {
				return &UnsupportedTranslationError{Reason: "chat_template_kwargs must be an object"}
			}
		}
		if kwargs == nil {
			kwargs = make(map[string]json.RawMessage)
		}
		kwargs["enable_thinking"], _ = json.Marshal(on)
		set("chat_template_kwargs", kwargs)
	}
	return nil
}
