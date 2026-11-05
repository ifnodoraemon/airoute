package provider

import (
	"bytes"
	"encoding/json"
	"fmt"
	"slices"
	"strings"

	"github.com/ifnodoraemon/airoute/internal/model"
)

// PrepareThinkingRequest applies console intent independently for each route
// candidate. Legacy SDK requests without thinking_control remain untouched.
func PrepareThinkingRequest(req *model.ChatCompletionRequest, channel *model.ChannelConfig) (*model.ChatCompletionRequest, error) {
	raw := thinkingControlRaw(req)
	policy := channel.ThinkingPolicyFor(req.Model)
	if !policy.Active() {
		policy = nil
	}
	control := model.ThinkingControl{Mode: "auto"}
	if req.ThinkingIntent != nil {
		control = *req.ThinkingIntent
	} else if len(raw) > 0 {
		var err error
		control, err = DecodeThinkingControl(raw)
		if err != nil {
			return nil, err
		}
	}
	if len(raw) == 0 && req.ThinkingIntent == nil && policy == nil {
		return req, nil
	}
	if len(raw) == 0 && req.ThinkingIntent == nil && hasNativeThinking(req) {
		if policy != nil && (policy.AllowedModes != nil || policy.AllowedEfforts != nil) {
			return nil, &UnsupportedTranslationError{Reason: "native thinking parameters cannot bypass route restrictions; use thinking_control"}
		}
		return req, nil
	}
	if len(raw) > 0 && hasNativeThinking(req) {
		return nil, &UnsupportedTranslationError{Reason: "do not combine thinking_control with provider-native thinking parameters"}
	}
	var err error
	control, err = policy.Resolve(control)
	if err != nil {
		return nil, &UnsupportedTranslationError{Reason: err.Error()}
	}
	profile, source := model.ResolveThinkingProfile(channel, req.Model)
	if !model.ThinkingProfileCompatible(model.ChatProviderType(channel, req), profile) {
		return nil, &UnsupportedTranslationError{Reason: "thinking profile is incompatible with channel protocol"}
	}
	allowedEffort := func(effort string) bool {
		for _, restriction := range []*model.ThinkingPolicy{req.ThinkingIntentPolicy, policy} {
			if restriction != nil && restriction.AllowedEfforts != nil && !slices.Contains(restriction.AllowedEfforts, effort) {
				return false
			}
		}
		return true
	}
	if control.Mode == "on" {
		if control.Effort != "" && !allowedEffort(control.Effort) {
			return nil, &UnsupportedTranslationError{Reason: "thinking effort is forbidden by the original route policy"}
		}
		restricted := (policy != nil && policy.AllowedEfforts != nil) || (req.ThinkingIntentPolicy != nil && req.ThinkingIntentPolicy.AllowedEfforts != nil)
		if control.Effort == "" && restricted {
			if allowedEffort(profile.DefaultEffort) {
				control.Effort = profile.DefaultEffort
			} else {
				for _, effort := range profile.Efforts {
					if allowedEffort(effort) {
						control.Effort = effort
						break
					}
				}
			}
			if control.Effort == "" {
				return nil, &UnsupportedTranslationError{Reason: "no upstream thinking effort satisfies route restrictions"}
			}
		}
	}
	if control.IncludeSummary != nil && *control.IncludeSummary && profile.Output != "summary" && profile.Output != "reasoning" && !(req.Native != nil && req.Native.Protocol == model.NativeResponses && profile.Wire == "openai") {
		return nil, &UnsupportedTranslationError{Reason: "upstream cannot return thinking content or a summary for this call"}
	}
	if err := profile.Validate(control); err != nil {
		return nil, &UnsupportedTranslationError{Reason: err.Error()}
	}
	if profile.ToolsRequireResponses && control.Mode != "off" && len(req.Tools) > 0 && (req.Native == nil || req.Native.Protocol != model.NativeResponses) {
		return nil, &UnsupportedTranslationError{Reason: "this model requires the Responses API for thinking with tools"}
	}
	copy := *req
	copy.Native = nil
	if req.Native != nil && req.Native.Protocol != model.NativeChat {
		copy.Native = req.Native
	}
	copy.Extra = make(model.ExtraFields, len(req.Extra))
	for k, v := range req.Extra {
		if k != "thinking_control" {
			copy.Extra[k] = v
		}
	}
	copy.PreparedThinking = &model.PreparedThinking{Control: control, Profile: profile, Source: source, UpstreamModel: channel.GetUpstreamModel(req.Model)}
	// Default mode preserves the model's policy. Summary visibility is independent
	// from enabling/disabling internal thinking.
	on, off := control.Mode == "on", control.Mode == "off"
	effort := control.Effort
	if effort == "" && on {
		effort = profile.DefaultEffort
	}
	set := func(key string, value any) { copy.Extra[key], _ = json.Marshal(value) }
	switch profile.Wire {
	case "openai":
		if off {
			effort = "none"
		}
		if on || off {
			set("reasoning_effort", effort)
		}
		if on || off {
			// Reasoning models use the combined answer+reasoning output limit.
			if copy.MaxTokens != nil {
				set("max_completion_tokens", *copy.MaxTokens)
				copy.MaxTokens = nil
			}
			if on {
				copy.Temperature, copy.TopP, copy.PresencePenalty, copy.FrequencyPenalty = nil, nil, nil, nil
			}
		}
	case "deepseek":
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
	case "qwen":
		if on || off {
			set("enable_thinking", on)
		}
		if on && control.Effort != "" {
			set("thinking_budget", profile.Budgets[control.Effort])
		}
	case "glm":
		if on {
			set("reasoning_effort", effort)
		}
	case "deepseek-template":
		if on || off {
			var kwargs map[string]json.RawMessage
			if raw := copy.Extra["chat_template_kwargs"]; len(raw) > 0 && json.Unmarshal(raw, &kwargs) != nil {
				return nil, &UnsupportedTranslationError{Reason: "chat_template_kwargs must be an object"}
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
	case "ollama":
		if on || off {
			set("think", on)
		}
	case "ollama-level":
		if on {
			set("think", effort)
		}
	case "template":
		if on || off {
			var kwargs map[string]json.RawMessage
			if raw := copy.Extra["chat_template_kwargs"]; len(raw) > 0 {
				if err := json.Unmarshal(raw, &kwargs); err != nil {
					return nil, &UnsupportedTranslationError{Reason: "chat_template_kwargs must be an object"}
				}
			}
			if kwargs == nil {
				kwargs = make(map[string]json.RawMessage)
			}
			kwargs["enable_thinking"], _ = json.Marshal(on)
			set("chat_template_kwargs", kwargs)
		}
	case "gemini-level", "gemini-budget", "gemma", "gemini-summary":
		if !model.IsGeminiProvider(channel.Type) {
			// Google's OpenAI-compatible endpoint uses this documented extension.
			config := geminiThinkingFields(copy.PreparedThinking)
			if len(config) > 0 {
				var extra map[string]json.RawMessage
				if raw := copy.Extra["extra_body"]; len(raw) > 0 {
					if err := json.Unmarshal(raw, &extra); err != nil {
						return nil, &UnsupportedTranslationError{Reason: "extra_body must be an object"}
					}
				}
				if extra == nil {
					extra = make(map[string]json.RawMessage)
				}
				var google map[string]json.RawMessage
				if len(extra["google"]) > 0 && json.Unmarshal(extra["google"], &google) != nil {
					return nil, &UnsupportedTranslationError{Reason: "extra_body.google must be an object"}
				}
				if google == nil {
					google = make(map[string]json.RawMessage)
				}
				google["thinking_config"], _ = json.Marshal(config)
				extra["google"], _ = json.Marshal(google)
				set("extra_body", extra)
			}
		}
	case "claude-budget":
		if on {
			budget := profile.Budgets[effort]
			maxTokens := 4096
			if copy.MaxTokens != nil {
				maxTokens = *copy.MaxTokens
			}
			if budget >= maxTokens {
				return nil, &UnsupportedTranslationError{Reason: fmt.Sprintf("thinking budget %d must be less than max_tokens (%d); raise the output limit or lower effort", budget, maxTokens)}
			}
			copy.Temperature, copy.TopP = nil, nil
		}
	case "claude-adaptive":
		if on {
			copy.Temperature, copy.TopP = nil, nil
		}
	}
	if req.Native != nil && req.Native.Protocol != model.NativeChat {
		if err := prepareNativeThinking(&copy, req.Native); err != nil {
			return nil, err
		}
	}
	return &copy, nil
}

func DecodeThinkingControl(raw json.RawMessage) (model.ThinkingControl, error) {
	var control model.ThinkingControl
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if len(bytes.TrimSpace(raw)) == 0 || bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return control, &UnsupportedTranslationError{Reason: "thinking_control must be an object"}
	}
	if err := decoder.Decode(&control); err != nil {
		return control, &UnsupportedTranslationError{Reason: "invalid thinking_control: " + err.Error()}
	}
	if control.Mode == "" {
		control.Mode = "auto"
	}
	return control, nil
}

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

func hasNativeThinking(req *model.ChatCompletionRequest) bool {
	for _, key := range []string{"reasoning_effort", "reasoning", "thinking", "enable_thinking", "think", "thinking_budget"} {
		if len(req.Extra[key]) > 0 {
			return true
		}
	}
	if raw := req.Extra["chat_template_kwargs"]; len(raw) > 0 {
		var object map[string]json.RawMessage
		if json.Unmarshal(raw, &object) == nil && (len(object["enable_thinking"]) > 0 || len(object["thinking"]) > 0 || len(object["reasoning_effort"]) > 0) {
			return true
		}
	}
	if raw := req.Extra["extra_body"]; len(raw) > 0 {
		var object struct {
			Google struct {
				Thinking json.RawMessage `json:"thinking_config"`
			} `json:"google"`
		}
		if json.Unmarshal(raw, &object) == nil && len(object.Google.Thinking) > 0 {
			return true
		}
	}
	if req.Native != nil && req.Native.Protocol != model.NativeChat {
		var envelope struct {
			Thinking         json.RawMessage `json:"thinking"`
			Reasoning        json.RawMessage `json:"reasoning"`
			GenerationConfig struct {
				Thinking json.RawMessage `json:"thinkingConfig"`
			} `json:"generationConfig"`
		}
		if json.Unmarshal(req.Native.Body, &envelope) == nil {
			return len(envelope.Thinking) > 0 || len(envelope.Reasoning) > 0 || len(envelope.GenerationConfig.Thinking) > 0
		}
	}

	return false
}

// Preserve the rest of a native document, especially signed history and unknown
// parameters. Gateway controls are removed before it can leave the process.
func prepareNativeThinking(req *model.ChatCompletionRequest, original *model.NativeRequest) error {
	var body map[string]json.RawMessage
	if err := json.Unmarshal(original.Body, &body); err != nil {
		return &UnsupportedTranslationError{Reason: "invalid native JSON document"}
	}
	delete(body, "thinking_control")
	prepared := req.PreparedThinking
	switch original.Protocol {
	case model.NativeGemini:
		if prepared.Profile.Wire != "gemini-level" && prepared.Profile.Wire != "gemini-budget" && prepared.Profile.Wire != "gemma" && prepared.Profile.Wire != "gemini-summary" {
			if prepared.Control.Mode != "auto" && prepared.Profile.Wire != "none" {
				return &UnsupportedTranslationError{Reason: "configured thinking profile does not support native Gemini controls"}
			}
			break
		}
		var config map[string]json.RawMessage
		if raw := body["generationConfig"]; len(raw) > 0 && json.Unmarshal(raw, &config) != nil {
			return &UnsupportedTranslationError{Reason: "generationConfig must be an object"}
		}
		if config == nil {
			config = make(map[string]json.RawMessage)
		}
		nativeConfig := &GeminiGenerationConfig{}
		applyPreparedGeminiThinking(nativeConfig, prepared)
		if nativeConfig.ThinkingConfig != nil {
			config["thinkingConfig"], _ = json.Marshal(nativeConfig.ThinkingConfig)
		}
		body["generationConfig"], _ = json.Marshal(config)
	case model.NativeMessages:
		if prepared.Profile.Wire != "claude-budget" && prepared.Profile.Wire != "claude-adaptive" {
			if prepared.Control.Mode != "auto" && prepared.Profile.Wire != "none" {
				return &UnsupportedTranslationError{Reason: "configured thinking profile does not support native Messages controls"}
			}
			break
		}
		nativeConfig := &AnthropicRequest{}
		applyPreparedAnthropicThinking(nativeConfig, prepared)
		if nativeConfig.Thinking != nil {
			body["thinking"], _ = json.Marshal(nativeConfig.Thinking)
		}
		if nativeConfig.OutputConfig != nil {
			body["output_config"], _ = json.Marshal(nativeConfig.OutputConfig)
		}
		if prepared.Control.Mode == "on" {
			delete(body, "temperature")
			delete(body, "top_p")
		}
	case model.NativeResponses:
		if prepared.Profile.Wire != "openai" {
			if prepared.Control.Mode != "auto" && prepared.Profile.Wire != "none" {
				return &UnsupportedTranslationError{Reason: "configured thinking profile does not support native Responses controls"}
			}
			break
		}
		reasoning := map[string]string{}
		if prepared.Control.Mode == "off" {
			reasoning["effort"] = "none"
		} else if prepared.Control.Mode == "on" {
			effort := prepared.Control.Effort
			if effort == "" {
				effort = prepared.Profile.DefaultEffort
			}
			reasoning["effort"] = effort
		}
		if prepared.Control.IncludeSummary != nil && *prepared.Control.IncludeSummary {
			reasoning["summary"] = "auto"
		}
		if len(reasoning) > 0 {
			body["reasoning"], _ = json.Marshal(reasoning)
		}
		if prepared.Control.Mode == "on" {
			delete(body, "temperature")
			delete(body, "top_p")
		}
	default:
		return &UnsupportedTranslationError{Reason: "thinking_control is not supported for this native protocol"}
	}
	copy := *original
	copy.Body, _ = json.Marshal(body)
	req.Native = &copy
	return nil
}

func ResolveThinkingIntent(req *model.ChatCompletionRequest, policy *model.ThinkingPolicy) (*model.ChatCompletionRequest, error) {
	if !policy.Active() {
		policy = nil
	}
	raw := thinkingControlRaw(req)
	if len(raw) == 0 && policy != nil && hasNativeThinking(req) && (policy.AllowedModes != nil || policy.AllowedEfforts != nil) {
		return nil, &UnsupportedTranslationError{Reason: "native thinking parameters cannot bypass route restrictions; use thinking_control"}
	}
	if len(raw) == 0 && (policy == nil || hasNativeThinking(req)) {
		return req, nil
	}
	control := model.ThinkingControl{Mode: "auto"}
	if len(raw) > 0 {
		var err error
		control, err = DecodeThinkingControl(raw)
		if err != nil {
			return nil, err
		}
	}
	control, err := policy.Resolve(control)
	if err != nil {
		return nil, &UnsupportedTranslationError{Reason: err.Error()}
	}
	copy := *req
	copy.ThinkingIntent = &control
	copy.ThinkingIntentPolicy = policy
	return &copy, nil
}

// ThinkingRequestMode distinguishes native SDK settings from upstream defaults
// in audit metadata, without parsing or logging the provider's thought text.
func ThinkingRequestMode(req *model.ChatCompletionRequest) string {
	if req.PreparedThinking != nil {
		return req.PreparedThinking.Control.Mode
	}
	if hasNativeThinking(req) {
		return "native"
	}
	return "auto"
}

// Read only the small extension, avoiding copies of native image/audio contents.
func thinkingControlRaw(req *model.ChatCompletionRequest) json.RawMessage {
	if raw := req.Extra["thinking_control"]; len(raw) > 0 {
		return raw
	}
	if req.Native == nil || req.Native.Protocol == model.NativeChat {
		return nil
	}
	var envelope struct {
		Control json.RawMessage `json:"thinking_control"`
	}
	if json.Unmarshal(req.Native.Body, &envelope) == nil {
		return envelope.Control
	}
	return nil
}
