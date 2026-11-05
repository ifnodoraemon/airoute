package provider

import (
	"encoding/json"
	"github.com/ifnodoraemon/airoute/internal/model"
	"github.com/stretchr/testify/require"
	"testing"
)

func controlledThinkingRequest(mode, effort string) *model.ChatCompletionRequest {
	control, _ := json.Marshal(model.ThinkingControl{Mode: mode, Effort: effort})
	return &model.ChatCompletionRequest{Model: "alias", Extra: model.ExtraFields{"thinking_control": control}}
}
func TestThinkingUsesDifferentWireContractsForSameUpstreamModel(t *testing.T) {
	for _, test := range []struct{ profile, mode, effort, key, want string }{
		{"deepseek", "off", "", "thinking", `{"type":"disabled"}`},
		{"vllm-deepseek41", "off", "", "chat_template_kwargs", `{"thinking":false}`},
		{"vllm-deepseek41", "on", "xhigh", "chat_template_kwargs", `{"thinking":true,"reasoning_effort":75}`},
		{"qwen-toggle", "off", "", "enable_thinking", `false`},
		{"glm53", "on", "low", "reasoning_effort", `"low"`},
		{"ollama-toggle", "off", "", "reasoning_effort", `"none"`},
		{"ollama-toggle", "on", "", "reasoning_effort", `"medium"`},
		{"ollama-levels", "on", "high", "reasoning_effort", `"high"`},
	} {
		t.Run(test.profile+test.mode, func(t *testing.T) {
			ch := &model.ChannelConfig{Type: model.ProviderOpenAI, ModelMapping: map[string]string{"alias": "real-model"}, ModelCapabilities: map[string]model.ModelCapabilityConfig{"real-model": {Thinking: &model.ThinkingConfig{Profile: test.profile}}}}
			req := controlledThinkingRequest(test.mode, test.effort)
			before, _ := json.Marshal(req)
			prepared, err := PrepareThinkingRequest(req, ch)
			require.NoError(t, err)
			require.JSONEq(t, test.want, string(prepared.Extra[test.key]))
			require.NotContains(t, prepared.Extra, "thinking_control")
			after, _ := json.Marshal(req)
			require.JSONEq(t, string(before), string(after))
		})
	}
}
func TestGeminiThinkingControlPreservesNativeHistoryAndRequestsSummary(t *testing.T) {
	req := controlledThinkingRequest("on", "high")
	req.Extra["thinking_control"] = json.RawMessage(`{"mode":"on","effort":"high","include_summary":true}`)
	req.Native = &model.NativeRequest{Protocol: model.NativeGemini, Body: json.RawMessage(`{"contents":[{"role":"model","parts":[{"thoughtSignature":"signed","text":"prior"}]}],"generationConfig":{"seed":9007199254740993}}`)}
	ch := &model.ChannelConfig{Type: model.ProviderGemini, ModelMapping: map[string]string{"alias": "gemini-3.8-flash"}}
	prepared, err := PrepareThinkingRequest(req, ch)
	require.NoError(t, err)
	require.Contains(t, string(prepared.Native.Body), `9007199254740993`)
	require.Contains(t, string(prepared.Native.Body), `"thoughtSignature":"signed"`)
	require.Contains(t, string(prepared.Native.Body), `"includeThoughts":true`)
	require.Contains(t, string(prepared.Native.Body), `"thinkingLevel":"HIGH"`)
	require.NotContains(t, string(prepared.Native.Body), "thinking_control")
	require.NotContains(t, string(req.Native.Body), "thinkingConfig")
}
func TestThinkingCannotBeSilentlyDisabledOrBypassPolicy(t *testing.T) {
	ch := &model.ChannelConfig{Type: model.ProviderOpenAI, ModelCapabilities: map[string]model.ModelCapabilityConfig{"alias": {Thinking: &model.ThinkingConfig{Profile: "glm53"}}}}
	_, err := PrepareThinkingRequest(controlledThinkingRequest("off", ""), ch)
	require.ErrorContains(t, err, "disabling")
	_, err = PrepareThinkingRequest(controlledThinkingRequest("on", "medium"), ch)
	require.ErrorContains(t, err, "medium")
	req := &model.ChatCompletionRequest{Model: "alias", Extra: model.ExtraFields{"reasoning_effort": json.RawMessage(`"high"`)}}
	_, err = ResolveThinkingIntent(req, &model.ThinkingPolicy{DefaultMode: "on", AllowedEfforts: []string{"low"}})
	require.ErrorContains(t, err, "cannot bypass")
	legacy, err := PrepareThinkingRequest(req, ch)
	require.NoError(t, err)
	require.Same(t, req, legacy)
	req = controlledThinkingRequest("on", "low")
	req.Extra["reasoning_effort"] = json.RawMessage(`"high"`)
	_, err = PrepareThinkingRequest(req, ch)
	require.ErrorContains(t, err, "do not combine")
}
func TestAnthropicSignedThinkingSurvivesToolHistory(t *testing.T) {
	var history anthropicHistoryState
	for _, raw := range []string{
		`{"type":"content_block_start","index":0,"content_block":{"type":"thinking","thinking":"","signature":""}}`,
		`{"type":"content_block_delta","index":0,"delta":{"type":"thinking_delta","thinking":"reason"}}`,
		`{"type":"content_block_delta","index":0,"delta":{"type":"signature_delta","signature":"signed-block"}}`,
		`{"type":"content_block_start","index":1,"content_block":{"type":"tool_use","id":"call-1","name":"lookup","input":{}}}`,
		`{"type":"content_block_delta","index":1,"delta":{"type":"input_json_delta","partial_json":"{\"id\":9007199254740993}"}}`,
	} {
		history.Observe([]byte(raw))
	}
	message := model.ChatMessage{Role: "assistant", Content: "display only", Extra: history.Extra()}
	original := restoredAnthropicHistory(message)
	require.Contains(t, string(original), `"signature":"signed-block"`)
	require.Contains(t, string(original), `9007199254740993`)
	req := &model.ChatCompletionRequest{Messages: []model.ChatMessage{message, {Role: "tool", ToolCallID: "call-1", Content: "result"}}}
	converted := convertOpenAIToAnthropic(req, &model.ChannelConfig{})
	encoded, err := json.Marshal(converted)
	require.NoError(t, err)
	require.Contains(t, string(encoded), `"thinking":"reason"`)
	require.Contains(t, string(encoded), `"signature":"signed-block"`)
	require.NotContains(t, string(encoded), "display only")
}

func TestEmptyRoutePolicyKeepsLegacyNativeRequestUntouched(t *testing.T) {
	req := &model.ChatCompletionRequest{Model: "alias", Native: &model.NativeRequest{Protocol: model.NativeChat, Body: json.RawMessage(`{"model":"alias","custom_native_feature":{"id":9007199254740993}}`)}}
	policy := &model.ThinkingPolicy{DefaultMode: "auto"}
	frozen, err := ResolveThinkingIntent(req, policy)
	require.NoError(t, err)
	require.Same(t, req, frozen)
	ch := &model.ChannelConfig{RouteRules: map[string]model.ModelRouteRule{"alias": {ThinkingPolicy: policy}}}
	prepared, err := PrepareThinkingRequest(req, ch)
	require.NoError(t, err)
	require.Same(t, req, prepared)
}
