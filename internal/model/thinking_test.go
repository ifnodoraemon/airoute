package model

import (
	"encoding/json"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestThinkingFactsBelongToMappedChannelModel(t *testing.T) {
	ch := &ChannelConfig{ID: 1, Name: "relay", Type: ProviderOpenAI,
		Models:            []string{"route-a", "route-b"},
		ModelMapping:      map[string]string{"route-a": "real-model", "route-b": "real-model"},
		ModelCapabilities: map[string]ModelCapabilityConfig{"real-model": {Thinking: &ThinkingConfig{Profile: "deepseek", Efforts: []string{"low"}}}},
		RouteRules:        map[string]ModelRouteRule{"route-a": {ThinkingPolicy: &ThinkingPolicy{DefaultMode: "on", DefaultEffort: "low"}}, "route-b": {ThinkingPolicy: &ThinkingPolicy{DefaultMode: "off"}}},
	}
	for _, alias := range []string{"route-a", "route-b", "relay/route-a"} {
		profile, source := ResolveThinkingProfile(ch.ForModel(alias), alias)
		require.Equal(t, "deepseek", profile.ID)
		require.Equal(t, "configured", source)
		require.Equal(t, []string{"low"}, profile.Efforts)
	}
	require.Equal(t, "on", ch.ThinkingPolicyFor("route-a").DefaultMode)
	require.Equal(t, "off", ch.ThinkingPolicyFor("route-b").DefaultMode)
	other := *ch
	other.ModelCapabilities = map[string]ModelCapabilityConfig{"real-model": {Thinking: &ThinkingConfig{Profile: "unsupported"}}}
	profile, _ := ResolveThinkingProfile(&other, "route-a")
	require.False(t, profile.On)
	// Narrowing a channel's levels must not mutate the shared catalog.
	original, _ := FindThinkingProfile("deepseek")
	require.Equal(t, []string{"low", "high", "max"}, original.Efforts)
}
func TestEmptyConfiguredEffortsSurviveSerialization(t *testing.T) {
	before := ThinkingConfig{Profile: "deepseek", Efforts: []string{}}
	raw, err := json.Marshal(before)
	require.NoError(t, err)
	var after ThinkingConfig
	require.NoError(t, json.Unmarshal(raw, &after))
	require.NotNil(t, after.Efforts)
	require.Empty(t, after.Efforts)
}
func TestThinkingProtocolAndPrefixPolicyCannotBeGuessedFromAlias(t *testing.T) {
	ch := &ChannelConfig{Name: "relay", Type: ProviderGemini, ModelMapping: map[string]string{"gpt-5": "gemini-3.8-flash"}, RouteRules: map[string]ModelRouteRule{
		"*": {ThinkingPolicy: &ThinkingPolicy{DefaultMode: "off"}}, "gpt-5": {ThinkingPolicy: &ThinkingPolicy{DefaultMode: "on"}},
	}}
	profile, source := ResolveThinkingProfile(ch, "gpt-5")
	require.Equal(t, "gemini-levels", profile.ID)
	require.Equal(t, "catalog", source)
	require.Equal(t, "on", ch.ThinkingPolicyFor("relay/gpt-5").DefaultMode)
	require.False(t, ThinkingProfileCompatible(ProviderAnthropic, profile))
	require.Error(t, ValidateChannelThinking(ProviderGemini, map[string]ModelCapabilityConfig{"m": {Thinking: &ThinkingConfig{Profile: "deepseek"}}}))
	unknown, _ := ResolveThinkingProfile(&ChannelConfig{Type: ProviderCustom}, "made-up-model")
	require.Empty(t, unknown.ID)
	require.Error(t, unknown.Validate(ThinkingControl{Mode: "on"}))
}

func TestResolveAlreadyMappedThinkingModelDoesNotRemap(t *testing.T) {
	ch := &ChannelConfig{Type: ProviderCustom, ModelMapping: map[string]string{"public": "actual", "actual": "other"}, ModelCapabilities: map[string]ModelCapabilityConfig{"actual": {Thinking: &ThinkingConfig{Profile: "reasoning-only"}}, "other": {Thinking: &ThinkingConfig{Profile: "unsupported"}}}}
	profile, _ := ResolveThinkingProfile(ch, "public")
	actual, _ := ResolveUpstreamThinkingProfile(ch, "actual")
	if profile.ID != "reasoning-only" || actual.ID != profile.ID {
		t.Fatalf("actual model was remapped: %s / %s", profile.ID, actual.ID)
	}
}
