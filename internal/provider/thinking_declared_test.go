package provider

import (
	"encoding/json"
	"testing"

	"github.com/ifnodoraemon/airoute/internal/model"
	"github.com/stretchr/testify/require"
)

func TestDeclaredThinkingLevelsUseActualModelAndExactWireValue(t *testing.T) {
	for _, effort := range []string{"ultra", "extended"} {
		t.Run(effort, func(t *testing.T) {
			channel := &model.ChannelConfig{Type: model.ProviderOpenAI, ModelMapping: map[string]string{"alias": "deployment"}, ModelCapabilities: map[string]model.ModelCapabilityConfig{"deployment": {Thinking: &model.ThinkingConfig{Efforts: []string{effort}}}}}
			control, _ := json.Marshal(model.ThinkingControl{Mode: "on", Effort: effort})
			request := &model.ChatCompletionRequest{Model: "alias", Extra: model.ExtraFields{"thinking_control": control}}
			prepared, err := PrepareThinkingRequest(request, channel)
			require.NoError(t, err)
			require.JSONEq(t, `"`+effort+`"`, string(prepared.Extra["reasoning_effort"]))
			require.Equal(t, "deployment", prepared.PreparedThinking.UpstreamModel)
			require.NotContains(t, request.Extra, "reasoning_effort")
			channel.RouteRules = map[string]model.ModelRouteRule{"alias": {ThinkingPolicy: &model.ThinkingPolicy{DefaultMode: "on", AllowedEfforts: []string{"high"}}}}
			_, err = PrepareThinkingRequest(request, channel)
			require.Error(t, err)
		})
	}
}

func TestDeclaredDisableHasWireRepresentation(t *testing.T) {
	yes := true
	for _, test := range []struct {
		kind model.ProviderType
		name string
		key  string
		want string
	}{
		{model.ProviderOpenAI, "glm-5.3", "thinking", `{"type":"disabled"}`},
		{model.ProviderOpenAI, "unlisted", "reasoning_effort", `"none"`},
	} {
		channel := &model.ChannelConfig{Type: test.kind, ModelCapabilities: map[string]model.ModelCapabilityConfig{test.name: {Thinking: &model.ThinkingConfig{CanDisable: &yes}}}}
		prepared, err := PrepareThinkingRequest(&model.ChatCompletionRequest{Model: test.name, Extra: model.ExtraFields{"thinking_control": json.RawMessage(`{"mode":"off"}`)}}, channel)
		require.NoError(t, err)
		require.JSONEq(t, test.want, string(prepared.Extra[test.key]))
	}
}

func TestManualDeclarationsOverrideFixedThinkingInActualRequests(t *testing.T) {
	yes := true
	channel := &model.ChannelConfig{Type: model.ProviderOpenAI, ModelMapping: map[string]string{"alias": "vendor/deepseek-deployment"}, ModelCapabilities: map[string]model.ModelCapabilityConfig{
		"vendor/deepseek-deployment": {Thinking: &model.ThinkingConfig{Profile: "reasoning-only", CanDisable: &yes, Efforts: []string{"ultra"}}},
	}}
	for _, test := range []struct{ control, thinking, effort string }{
		{`{"mode":"off"}`, `{"type":"disabled"}`, ""},
		{`{"mode":"on","effort":"ultra"}`, `{"type":"enabled"}`, `"ultra"`},
	} {
		prepared, err := PrepareThinkingRequest(&model.ChatCompletionRequest{Model: "alias", Extra: model.ExtraFields{"thinking_control": json.RawMessage(test.control)}}, channel)
		require.NoError(t, err)
		require.JSONEq(t, test.thinking, string(prepared.Extra["thinking"]))
		if test.effort != "" {
			require.JSONEq(t, test.effort, string(prepared.Extra["reasoning_effort"]))
		} else {
			require.NotContains(t, prepared.Extra, "reasoning_effort")
		}
	}
}

func TestDeclaredGeminiDisableDoesNotSendAnEnabledLevel(t *testing.T) {
	yes := true
	channel := &model.ChannelConfig{Type: model.ProviderGemini, ModelCapabilities: map[string]model.ModelCapabilityConfig{"gemini-3.8-flash": {Thinking: &model.ThinkingConfig{CanDisable: &yes}}}}
	request := &model.ChatCompletionRequest{Model: "gemini-3.8-flash", Extra: model.ExtraFields{"thinking_control": json.RawMessage(`{"mode":"off"}`)}, Native: &model.NativeRequest{Protocol: model.NativeGemini, Body: json.RawMessage(`{"contents":[{"parts":[{"text":"ping"}]}]}`)}}
	prepared, err := PrepareThinkingRequest(request, channel)
	require.NoError(t, err)
	var body struct {
		GenerationConfig struct {
			ThinkingConfig map[string]any `json:"thinkingConfig"`
		} `json:"generationConfig"`
	}
	require.NoError(t, json.Unmarshal(prepared.Native.Body, &body))
	require.EqualValues(t, 0, body.GenerationConfig.ThinkingConfig["thinkingBudget"])
	require.NotContains(t, body.GenerationConfig.ThinkingConfig, "thinkingLevel")
}
