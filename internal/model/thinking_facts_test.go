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

func TestThinkingDeclarationsCanExtendCatalogForNamedParameters(t *testing.T) {
	yes := true
	for _, facts := range []*ThinkingConfig{{CanDisable: &yes}, {Efforts: []string{"medium", "ultra", "extended"}}} {
		config := map[string]ModelCapabilityConfig{"aliyun/glm-5.3": {Thinking: facts}}
		require.NoError(t, ValidateChannelThinking(ProviderOpenAI, config))
		profile, source := ResolveUpstreamThinkingProfile(&ChannelConfig{Type: ProviderOpenAI, ModelCapabilities: config}, "aliyun/glm-5.3")
		require.Equal(t, "glm53", profile.ID)
		require.Equal(t, "configured", source)
	}
	unknown := map[string]ModelCapabilityConfig{"custom-name": {Thinking: &ThinkingConfig{Efforts: []string{"high"}}}}
	require.NoError(t, ValidateChannelThinking(ProviderGPUStack, unknown))
	profile, _ := ResolveUpstreamThinkingProfile(&ChannelConfig{Type: ProviderGPUStack, ModelCapabilities: unknown}, "custom-name")
	require.Equal(t, "openai", profile.Wire)
	require.Equal(t, []string{"high"}, profile.Efforts)
	unknown["custom-name"] = ModelCapabilityConfig{Thinking: &ThinkingConfig{Profile: "vllm-deepseek41", Efforts: []string{"high"}}}
	require.NoError(t, ValidateChannelThinking(ProviderGPUStack, unknown))
}

func TestThinkingDeclarationsRequireEncodableControls(t *testing.T) {
	for _, profile := range []string{"claude-budget", "vllm-deepseek41", "qwen-toggle"} {
		config := map[string]ModelCapabilityConfig{"model": {Thinking: &ThinkingConfig{Profile: profile, Efforts: []string{"ultra"}}}}
		require.Error(t, ValidateChannelThinking(ProviderCustom, config), profile)
	}
	for _, effort := range []string{"none", "auto", "off", "adaptive", "minimal", "High", "", "a b", "__invalid__"} {
		config := map[string]ModelCapabilityConfig{"model": {Thinking: &ThinkingConfig{Efforts: []string{effort}}}}
		require.Error(t, ValidateChannelThinking(ProviderCustom, config), effort)
	}
}

func TestManualThinkingFactsOverrideFixedAndUnknownPresets(t *testing.T) {
	yes := true
	for _, preset := range []string{"unknown", "unsupported", "reasoning-only"} {
		channel := &ChannelConfig{Type: ProviderOpenAI, ModelCapabilities: map[string]ModelCapabilityConfig{
			"vendor/deepseek-deployment": {Thinking: &ThinkingConfig{Profile: preset, CanDisable: &yes, Efforts: []string{"low", "ultra", "extended"}}},
		}}
		require.NoError(t, ValidateChannelThinking(channel.Type, channel.ModelCapabilities), preset)
		profile, source := ResolveThinkingProfile(channel, "vendor/deepseek-deployment")
		require.Equal(t, "configured", source)
		require.Equal(t, "deepseek", profile.Wire)
		require.True(t, profile.Off)
		require.Equal(t, []string{"low", "ultra", "extended"}, profile.Efforts)
	}
}

func TestManualThinkingFactsDoNotInheritUnconfirmedDialectDefaults(t *testing.T) {
	yes := true
	for _, facts := range []*ThinkingConfig{
		{Profile: "reasoning-only", CanDisable: &yes},
		{Efforts: []string{"ultra"}},
	} {
		channel := &ChannelConfig{Type: ProviderOpenAI, ModelCapabilities: map[string]ModelCapabilityConfig{"custom": {Thinking: facts}}}
		require.NoError(t, ValidateChannelThinking(channel.Type, channel.ModelCapabilities))
		profile, _ := ResolveThinkingProfile(channel, "custom")
		if facts.Efforts == nil {
			require.Empty(t, profile.Efforts)
		} else {
			require.False(t, profile.Off)
			require.Equal(t, facts.Efforts, profile.Efforts)
		}
	}
}

func TestNamedThinkingAdaptersAllowAllLevelsWithoutChangingCatalogFacts(t *testing.T) {
	for _, id := range []string{"openai-toggle", "deepseek", "glm53", "claude-adaptive", "gemini-levels", "gemini-summary", "ollama-levels"} {
		profile, ok := FindThinkingProfile(id)
		require.True(t, ok, id)
		original := profile.Capability("catalog")
		transport := ThinkingTransportProfile(profile)
		require.Equal(t, ThinkingEfforts(), transport.Efforts, id)
		for _, effort := range append(ThinkingEfforts(), "extended") {
			require.NoError(t, validateThinkingFacts(profile, &ThinkingConfig{Efforts: []string{effort}}), id)
		}
		require.Equal(t, original, profile.Capability("catalog"))
	}
}

func TestManualDisableCanOverrideEveryEncodableModelPreset(t *testing.T) {
	yes := true
	for _, profile := range ThinkingProfiles() {
		if profile.Wire == "none" || profile.Wire == "unknown" {
			continue
		}
		require.True(t, ThinkingTransportProfile(profile).Off, profile.ID)
		require.NoError(t, validateThinkingFacts(profile, &ThinkingConfig{CanDisable: &yes}), profile.ID)
	}
}

func TestThinkingCatalogExcludesRemovedMinimalEffort(t *testing.T) {
	require.NotContains(t, ThinkingEfforts(), "minimal")
	for _, profile := range ThinkingProfiles() {
		require.NotContains(t, profile.Efforts, "minimal", profile.ID)
		require.NotContains(t, ThinkingTransportProfile(profile).Efforts, "minimal", profile.ID)
	}
	for _, policy := range []*ThinkingPolicy{
		{DefaultMode: "on", DefaultEffort: "minimal"},
		{DefaultMode: "on", AllowedEfforts: []string{"low", "minimal"}},
	} {
		require.Error(t, policy.Validate())
	}
}

func TestClaudeCatalogIncludesFullDocumentedEfforts(t *testing.T) {
	for _, name := range []string{"claude-opus-4-7", "claude-sonnet-5", "claude-mythos-5-1"} {
		profile, _ := ResolveThinkingProfile(&ChannelConfig{Type: ProviderAnthropic}, name)
		require.Equal(t, []string{"low", "medium", "high", "xhigh", "max"}, profile.Efforts)
	}
}

func TestGeminiRollingAliasAllowsConfirmedDeclarations(t *testing.T) {
	yes := true
	channel := &ChannelConfig{Type: ProviderGemini, ModelCapabilities: map[string]ModelCapabilityConfig{"gemini-flash-latest": {Thinking: &ThinkingConfig{CanDisable: &yes, Efforts: []string{"low", "high"}}}}}
	require.NoError(t, ValidateChannelThinking(channel.Type, channel.ModelCapabilities))
	profile, source := ResolveThinkingProfile(channel, "gemini-flash-latest")
	require.True(t, profile.On)
	require.True(t, profile.Off)
	require.Equal(t, []string{"low", "high"}, profile.Efforts)
	require.Equal(t, "configured", source)
	channel.ModelCapabilities = nil
	automatic, _ := ResolveThinkingProfile(channel, "gemini-flash-latest")
	require.False(t, automatic.On)
	require.False(t, automatic.Off)
	require.Empty(t, automatic.Efforts)
}

func TestBackendContractRetainsAvailableControlsAfterNarrowing(t *testing.T) {
	no := false
	channel := &ChannelConfig{Type: ProviderGPUStack, ModelCapabilities: map[string]ModelCapabilityConfig{
		"custom-deployment": {Thinking: &ThinkingConfig{Profile: "vllm-deepseek41", CanDisable: &no, Efforts: []string{}}},
	}}
	actual, _ := ResolveUpstreamThinkingProfile(channel, "custom-deployment")
	require.False(t, actual.Off)
	require.Empty(t, actual.Efforts)
	available, source := ResolveUpstreamThinkingContract(channel, "custom-deployment")
	require.Equal(t, "configured", source)
	require.True(t, available.Off)
	require.Equal(t, []string{"low", "high", "xhigh", "max"}, available.Efforts)
}
