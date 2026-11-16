package provider

import (
	"encoding/json"

	"github.com/ifnodoraemon/airoute/internal/model"
)

// Preserve the rest of a native document, especially signed history and unknown
// parameters. Gateway controls are removed before it can leave the process.
func prepareNativeThinking(req *model.ChatCompletionRequest, original *model.NativeRequest) error {
	var body map[string]json.RawMessage
	if err := json.Unmarshal(original.Body, &body); err != nil {
		return &UnsupportedTranslationError{Reason: "invalid native JSON document"}
	}
	delete(body, "thinking_control")
	var err error
	switch original.Protocol {
	case model.NativeChat:
		err = prepareNativeChatThinking(body, req)
	case model.NativeGemini:
		err = prepareNativeGeminiThinking(body, req.PreparedThinking)
	case model.NativeMessages:
		err = prepareNativeMessagesThinking(body, req.PreparedThinking)
	case model.NativeResponses:
		err = prepareNativeResponsesThinking(body, req.PreparedThinking)
	default:
		return &UnsupportedTranslationError{Reason: "thinking_control is not supported for this native protocol"}
	}
	if err != nil {
		return err
	}
	copy := *original
	copy.Body, _ = json.Marshal(body)
	req.Native = &copy
	return nil
}

func prepareNativeGeminiThinking(body map[string]json.RawMessage, prepared *model.PreparedThinking) error {
	if prepared.Profile.Wire != "gemini-level" && prepared.Profile.Wire != "gemini-budget" && prepared.Profile.Wire != "gemma" && prepared.Profile.Wire != "gemini-summary" {
		if prepared.Control.Mode != "auto" && prepared.Profile.Wire != "none" {
			return &UnsupportedTranslationError{Reason: "configured thinking profile does not support native Gemini controls"}
		}
		return nil
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
	return nil
}

func prepareNativeMessagesThinking(body map[string]json.RawMessage, prepared *model.PreparedThinking) error {
	if prepared.Profile.Wire != "claude-budget" && prepared.Profile.Wire != "claude-adaptive" {
		if prepared.Control.Mode != "auto" && prepared.Profile.Wire != "none" {
			return &UnsupportedTranslationError{Reason: "configured thinking profile does not support native Messages controls"}
		}
		return nil
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
	return nil
}

func prepareNativeResponsesThinking(body map[string]json.RawMessage, prepared *model.PreparedThinking) error {
	if prepared.Profile.Wire != "openai" {
		if prepared.Control.Mode != "auto" && prepared.Profile.Wire != "none" {
			return &UnsupportedTranslationError{Reason: "configured thinking profile does not support native Responses controls"}
		}
		return nil
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
	return nil
}

// prepareNativeChatThinking patches an OpenAI-compatible native chat body with
// the route's thinking policy so the request keeps passthrough fidelity (signed
// history, vendor fields) instead of being rebuilt by conversion. applyThinkingWire
// already computed the wire parameters into req.Extra; only the thinking keys are
// merged so unrelated native fields keep their original JSON representation.
func prepareNativeChatThinking(body map[string]json.RawMessage, req *model.ChatCompletionRequest) error {
	for _, key := range []string{"reasoning_effort", "thinking", "enable_thinking", "thinking_budget", "max_completion_tokens"} {
		if v, ok := req.Extra[key]; ok {
			body[key] = v
		}
	}
	if v, ok := req.Extra["chat_template_kwargs"]; ok {
		body["chat_template_kwargs"] = mergeNativeJSONObject(body["chat_template_kwargs"], v)
	}
	// max_tokens and max_completion_tokens are mutually exclusive upstream; the
	// openai wire moves the limit into max_completion_tokens for reasoning models.
	if _, ok := body["max_completion_tokens"]; ok {
		delete(body, "max_tokens")
	}
	if req.PreparedThinking.Control.Mode == "on" && req.PreparedThinking.Profile.Wire == "openai" {
		delete(body, "temperature")
		delete(body, "top_p")
		delete(body, "presence_penalty")
		delete(body, "frequency_penalty")
	}
	return nil
}

// mergeNativeJSONObject overlays patch onto base, preserving base keys the patch
// does not set. A non-object base is replaced by the patch.
func mergeNativeJSONObject(base, patch json.RawMessage) json.RawMessage {
	var baseMap, patchMap map[string]json.RawMessage
	if json.Unmarshal(patch, &patchMap) != nil {
		return patch
	}
	if json.Unmarshal(base, &baseMap) != nil || baseMap == nil {
		return patch
	}
	for k, v := range patchMap {
		baseMap[k] = v
	}
	out, err := json.Marshal(baseMap)
	if err != nil {
		return patch
	}
	return out
}
