package provider

import (
	"bytes"
	"encoding/json"

	"github.com/ifnodoraemon/airoute/internal/model"
)

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
