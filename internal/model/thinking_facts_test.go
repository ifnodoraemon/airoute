package model

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func TestTwoThinkingFactsUseActualModelAndKeepAutomaticTransport(t *testing.T) {
	no := false
	for _, test := range []struct {
		name            string
		kind            ProviderType
		actual, profile string
		efforts         []string
	}{
		{"closed Gemini API", ProviderGemini, "gemini-3.8-flash", "gemini-levels", []string{"low", "high"}},
		{"open deployment", ProviderGPUStack, "deepseek-v4.1-flash", "vllm-deepseek41", []string{"low", "max"}},
		{"open weights through API", ProviderOpenAI, "aliyun/glm-5.3", "glm53", []string{"low", "max"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			facts := &ThinkingConfig{CanDisable: &no, Efforts: test.efforts}
			config := map[string]ModelCapabilityConfig{test.actual: {Thinking: facts}}
			require.NoError(t, ValidateChannelThinking(test.kind, config))
			channel := &ChannelConfig{Type: test.kind, ModelMapping: map[string]string{"alias": test.actual}, ModelCapabilities: config}
			profile, source := ResolveThinkingProfile(channel, "alias")
			require.Equal(t, test.profile, profile.ID)
			require.Equal(t, "configured", source)
			require.False(t, profile.Off)
			require.Equal(t, test.efforts, profile.Efforts)
			require.Error(t, profile.Validate(ThinkingControl{Mode: "off"}))
			raw, err := json.Marshal(facts)
			require.NoError(t, err)
			require.NotContains(t, string(raw), "profile")
			require.Contains(t, string(raw), `"can_disable":false`)
		})
	}
}

func TestThinkingFactsYAMLRoundTripPreservesFalseAndEmptyLevels(t *testing.T) {
	no := false
	for _, before := range []ThinkingConfig{
		{Profile: "deepseek"},
		{CanDisable: &no, Efforts: []string{}},
		{CanDisable: &no, Efforts: []string{"low", "high"}},
	} {
		raw, err := yaml.Marshal(before)
		require.NoError(t, err)
		var after ThinkingConfig
		require.NoError(t, yaml.Unmarshal(raw, &after))
		require.Equal(t, before, after)
	}
}

func TestEmptySelectableLevelsRetainFixedAdapterDefault(t *testing.T) {
	config := map[string]ModelCapabilityConfig{"aliyun/glm-5.3": {Thinking: &ThinkingConfig{Efforts: []string{}}}}
	require.NoError(t, ValidateChannelThinking(ProviderOpenAI, config))
	profile, _ := ResolveUpstreamThinkingProfile(&ChannelConfig{Type: ProviderOpenAI, ModelCapabilities: config}, "aliyun/glm-5.3")
	require.Empty(t, profile.Efforts)
	require.Equal(t, "max", profile.DefaultEffort)
	require.NoError(t, profile.Validate(ThinkingControl{Mode: "on"}))
	require.Error(t, profile.Validate(ThinkingControl{Mode: "on", Effort: "max"}))
}

func TestThinkingFactsCannotInventUnsupportedControls(t *testing.T) {
	yes := true
	for _, facts := range []*ThinkingConfig{{CanDisable: &yes}, {Efforts: []string{"medium"}}} {
		config := map[string]ModelCapabilityConfig{"aliyun/glm-5.3": {Thinking: facts}}
		require.Error(t, ValidateChannelThinking(ProviderOpenAI, config))
		profile, source := ResolveUpstreamThinkingProfile(&ChannelConfig{Type: ProviderOpenAI, ModelCapabilities: config}, "aliyun/glm-5.3")
		require.Empty(t, profile.ID)
		require.Equal(t, "invalid_configuration", source)
	}
	unknown := map[string]ModelCapabilityConfig{"custom-name": {Thinking: &ThinkingConfig{Efforts: []string{"high"}}}}
	require.Error(t, ValidateChannelThinking(ProviderGPUStack, unknown))
	unknown["custom-name"] = ModelCapabilityConfig{Thinking: &ThinkingConfig{Profile: "vllm-deepseek41", Efforts: []string{"high"}}}
	require.NoError(t, ValidateChannelThinking(ProviderGPUStack, unknown))
}
