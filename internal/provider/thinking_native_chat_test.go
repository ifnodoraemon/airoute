package provider

import (
	"encoding/json"
	"testing"

	"github.com/ifnodoraemon/airoute/internal/model"
	"github.com/stretchr/testify/require"
)

// An active thinking policy on a native chat request must keep passthrough (the
// signed history and vendor fields survive) and patch the thinking wire params
// into the native body, instead of nil-ing Native and forcing a lossy conversion.
func TestNativeChatThinkingKeepsPassthroughAndPatchesBody(t *testing.T) {
	channel := &model.ChannelConfig{
		Type:         model.ProviderOpenAI,
		ModelMapping: map[string]string{"alias": "deployment"},
		ModelCapabilities: map[string]model.ModelCapabilityConfig{
			"deployment": {Thinking: &model.ThinkingConfig{Efforts: []string{"high"}}},
		},
	}
	maxTok := 100
	request := &model.ChatCompletionRequest{
		Model:     "alias",
		MaxTokens: &maxTok,
		Extra:     model.ExtraFields{"thinking_control": json.RawMessage(`{"mode":"on","effort":"high"}`)},
		Native: &model.NativeRequest{Protocol: model.NativeChat, Body: json.RawMessage(
			`{"model":"alias","messages":[{"role":"user","content":"hi"}],"max_tokens":100,"temperature":0.7,"top_p":0.9,"vendor_option":9007199254740993}`)},
	}
	prepared, err := PrepareThinkingRequest(request, channel)
	require.NoError(t, err)
	require.NotNil(t, prepared.Native, "NativeChat must keep passthrough when thinking is active")
	require.Equal(t, model.NativeChat, prepared.Native.Protocol)

	var body map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(prepared.Native.Body, &body))
	require.JSONEq(t, `"high"`, string(body["reasoning_effort"]))
	require.JSONEq(t, `100`, string(body["max_completion_tokens"]))
	require.NotContains(t, body, "max_tokens", "max_tokens and max_completion_tokens are mutually exclusive")
	require.NotContains(t, body, "temperature", "reasoning-on clears sampling params")
	require.NotContains(t, body, "top_p")
	require.NotContains(t, body, "thinking_control", "gateway control must not leak upstream")
	// Unknown vendor fields keep their exact JSON representation (large integer).
	require.JSONEq(t, `9007199254740993`, string(body["vendor_option"]))
	require.JSONEq(t, `[{"role":"user","content":"hi"}]`, string(body["messages"]))
}

// Thinking-off on a native chat request maps to reasoning_effort "none" and keeps
// the sampling params (only reasoning-on clears them).
func TestNativeChatThinkingOffPatchesNone(t *testing.T) {
	yes := true
	channel := &model.ChannelConfig{
		Type: model.ProviderOpenAI,
		ModelCapabilities: map[string]model.ModelCapabilityConfig{
			"unlisted": {Thinking: &model.ThinkingConfig{CanDisable: &yes}},
		},
	}
	request := &model.ChatCompletionRequest{
		Model:  "unlisted",
		Extra:  model.ExtraFields{"thinking_control": json.RawMessage(`{"mode":"off"}`)},
		Native: &model.NativeRequest{Protocol: model.NativeChat, Body: json.RawMessage(`{"model":"unlisted","messages":[],"temperature":0.5}`)},
	}
	prepared, err := PrepareThinkingRequest(request, channel)
	require.NoError(t, err)
	require.NotNil(t, prepared.Native)
	var body map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(prepared.Native.Body, &body))
	require.JSONEq(t, `"none"`, string(body["reasoning_effort"]))
	require.JSONEq(t, `0.5`, string(body["temperature"]), "thinking-off keeps sampling params")
}