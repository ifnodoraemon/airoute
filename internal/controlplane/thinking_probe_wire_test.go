package controlplane

import (
	"encoding/json"
	"testing"

	"github.com/ifnodoraemon/airoute/internal/model"
	"github.com/stretchr/testify/require"
)

func TestThinkingProbeUsesChannelParameterDialect(t *testing.T) {
	for _, test := range []struct {
		name, profile, path string
		kind                model.ProviderType
		protocols           []string
		mode, effort        string
		field               string
		want                string
	}{
		{"messages", "claude-adaptive", "/v1/messages", model.ProviderAnthropic, nil, "on", "xhigh", "output_config", `{"effort":"xhigh"}`},
		{"native messages relay", "claude-adaptive", "/v1/messages", model.ProviderCustom, []string{"anthropic_native"}, "on", "max", "output_config", `{"effort":"max"}`},
		{"gemini", "gemini-levels", "/v1beta/models/actual:generateContent", model.ProviderGemini, nil, "on", "high", "generationConfig", `{"maxOutputTokens":128,"thinkingConfig":{"thinkingLevel":"HIGH"}}`},
		{"gemini off", "gemini-levels", "/v1beta/models/actual:generateContent", model.ProviderGemini, nil, "off", "", "generationConfig", `{"maxOutputTokens":128,"thinkingConfig":{"thinkingBudget":0}}`},
		{"gemini rolling alias", "gemini-summary", "/v1beta/models/actual:generateContent", model.ProviderGemini, nil, "on", "high", "generationConfig", `{"maxOutputTokens":128,"thinkingConfig":{"thinkingLevel":"HIGH"}}`},
		{"template", "vllm-deepseek41", "/v1/chat/completions", model.ProviderGPUStack, nil, "on", "xhigh", "chat_template_kwargs", `{"thinking":true,"reasoning_effort":75}`},
		{"responses", "openai-max", "/v1/responses", model.ProviderOpenAI, []string{"openai_response"}, "on", "ultra", "reasoning", `{"effort":"ultra"}`},
	} {
		t.Run(test.name, func(t *testing.T) {
			profile, found := model.FindThinkingProfile(test.profile)
			require.True(t, found)
			profile = model.ThinkingTransportProfile(profile)
			channel := &model.ChannelConfig{Type: test.kind, BaseURL: "https://upstream.example", Protocols: test.protocols}
			target, body, err := buildThinkingProbeRequest(channel, "actual", profile, model.ThinkingControl{Mode: test.mode, Effort: test.effort})
			require.NoError(t, err)
			require.Equal(t, "https://upstream.example"+test.path, target)
			var fields map[string]json.RawMessage
			require.NoError(t, json.Unmarshal(body, &fields))
			require.JSONEq(t, test.want, string(fields[test.field]))
			require.NotContains(t, fields, "thinking_control")
			if profile.Wire == "openai" {
				invalid, parameter := invalidThinkingProbe(body, profile, test.kind)
				require.Equal(t, "effort", parameter)
				require.Contains(t, string(invalid), invalidProbeEffort)
				require.NotContains(t, string(invalid), "reasoning_effort")
			}
		})
	}
}

func TestThinkingProbeReadsResponsesUsageAndRejectsErrorEnvelopes(t *testing.T) {
	response := thinkingProbeResponse{status: 200, body: []byte(`{"output":[{"type":"reasoning"}],"usage":{"output_tokens":10,"output_tokens_details":{"reasoning_tokens":10}}}`)}
	valid, tokens, thought := response.observation()
	require.True(t, valid)
	require.True(t, thought)
	require.NotNil(t, tokens)
	require.Equal(t, 10, *tokens)
	response.body = []byte(`{"error":{"message":"failed"},"choices":[{}]}`)
	valid, _, _ = response.observation()
	require.False(t, valid)
}
