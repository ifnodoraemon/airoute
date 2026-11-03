package api

import (
	"encoding/json"
	"fmt"
	"io"

	"github.com/gin-gonic/gin"
	"github.com/ifnodoraemon/airoute/internal/model"
	"github.com/ifnodoraemon/airoute/internal/provider"
)

func bindNativeJSON(c *gin.Context, target any, protocol string) (*model.NativeRequest, error) {
	body, err := io.ReadAll(c.Request.Body)
	if err != nil {
		return nil, err
	}
	decodeBody := body
	if protocol == model.NativeGemini || protocol == model.NativeGeminiEmbedding {
		decodeBody = provider.CanonicalGeminiJSON(body)
	}
	if err := json.Unmarshal(decodeBody, target); err != nil {
		return nil, err
	}
	headers := make(map[string]string)
	for _, name := range []string{"Anthropic-Version", "Anthropic-Beta", "OpenAI-Beta"} {
		if value := c.GetHeader(name); value != "" {
			headers[name] = value
		}
	}
	native := &model.NativeRequest{Protocol: protocol, Body: body, Headers: headers}
	if err := bindNativeState(c, native); err != nil {
		return nil, err
	}
	return native, nil
}

// responsesTranslation preserves the common structured-output and reasoning
// options. Features without a chat equivalent remain native-only.
func responsesTranslation(native *model.NativeRequest, req model.ResponseRequest, chat *model.ChatCompletionRequest) {
	if native.TranslationError != nil {
		return
	}
	var fields map[string]json.RawMessage
	_ = json.Unmarshal(native.Body, &fields)
	if chat.Extra == nil {
		chat.Extra = make(model.ExtraFields)
	}
	known := map[string]bool{"model": true, "input": true, "instructions": true, "previous_response_id": true, "user": true,
		"stream": true, "temperature": true, "top_p": true, "max_output_tokens": true, "tools": true,
		"tool_choice": true, "parallel_tool_calls": true, "metadata": true, "reasoning": true, "text": true, "service_tier": true}
	for name := range fields {
		if !known[name] {
			native.TranslationError = fmt.Errorf("responses field %q requires a native Responses upstream", name)
			return
		}
	}
	for _, tool := range req.Tools {
		if object, ok := tool.(map[string]any); ok && object["type"] != "function" {
			native.TranslationError = fmt.Errorf("responses tool %q requires a native Responses upstream", object["type"])
			return
		}
	}
	if raw := fields["service_tier"]; len(raw) > 0 {
		chat.Extra["service_tier"] = raw
	}
	if raw := fields["reasoning"]; len(raw) > 0 {
		var options map[string]json.RawMessage
		_ = json.Unmarshal(raw, &options)
		for name, value := range options {
			if name != "effort" {
				native.TranslationError = fmt.Errorf("reasoning.%s requires a native Responses upstream", name)
				return
			}
			chat.Extra["reasoning_effort"] = value
		}
	}
	if raw := fields["text"]; len(raw) > 0 {
		var text map[string]json.RawMessage
		_ = json.Unmarshal(raw, &text)
		if len(text) > 1 || (len(text) == 1 && text["format"] == nil) {
			native.TranslationError = fmt.Errorf("text options require a native Responses upstream")
			return
		}
		chat.Extra["response_format"] = chatResponseFormat(text["format"])
	}
}

func chatResponseFormat(raw json.RawMessage) json.RawMessage {
	var format map[string]json.RawMessage
	if json.Unmarshal(raw, &format) != nil || string(format["type"]) != `"json_schema"` {
		return raw
	}
	delete(format, "type")
	schema, _ := json.Marshal(format)
	body, _ := json.Marshal(map[string]json.RawMessage{"type": json.RawMessage(`"json_schema"`), "json_schema": schema})
	return body
}
