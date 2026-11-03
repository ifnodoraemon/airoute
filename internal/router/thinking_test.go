package router

import (
	"context"
	"encoding/json"
	"github.com/ifnodoraemon/airoute/internal/model"
	"github.com/ifnodoraemon/airoute/internal/provider"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestThinkingFiltersChannelsBeforeSchedulingAndPreservesFallbackIntent(t *testing.T) {
	rules := map[string]model.ModelRouteRule{
		"alias":    {ThinkingPolicy: &model.ThinkingPolicy{DefaultMode: "on", AllowedModes: []string{"on"}, AllowedEfforts: []string{"low"}}},
		"fallback": {ThinkingPolicy: &model.ThinkingPolicy{DefaultMode: "off"}},
	}
	d := NewDispatcher([]model.ChannelConfig{
		{ID: 1, Name: "capable", Type: model.ProviderOpenAI, Models: []string{"alias", "fallback"}, ModelMapping: map[string]string{"alias": "real-model", "fallback": "real-model"}, ModelCapabilities: map[string]model.ModelCapabilityConfig{"real-model": {Thinking: &model.ThinkingConfig{Profile: "deepseek"}}}, RouteRules: rules, Priority: 1, Weight: 1},
		{ID: 2, Name: "incapable", Type: model.ProviderOpenAI, Models: []string{"alias", "fallback"}, ModelCapabilities: map[string]model.ModelCapabilityConfig{"*": {Thinking: &model.ThinkingConfig{Profile: "unsupported"}}}, RouteRules: rules, Priority: 1, Weight: 1},
	})
	req, err := d.withThinkingIntent(&model.ChatCompletionRequest{Model: "alias"})
	require.NoError(t, err)
	require.Equal(t, "on", req.ThinkingIntent.Mode)
	channels, _ := d.candidateChannelsForRequest(context.Background(), req, "chat")
	require.Len(t, channels, 1)
	require.Equal(t, "capable", channels[0].Name)
	require.Empty(t, d.wrrWeights)
	// Cross-model fallback must preserve original requirements as well as enforcing
	// the fallback route's policy. Its default off cannot downgrade explicit on.
	fallback := *req
	fallback.Model = "fallback"
	prepared, err := provider.PrepareThinkingRequest(&fallback, channels[0])
	require.NoError(t, err)
	require.Equal(t, "on", prepared.PreparedThinking.Control.Mode)
	require.Equal(t, "low", prepared.PreparedThinking.Control.Effort)
	fallback.Extra = model.ExtraFields{"thinking_control": json.RawMessage(`{"mode":"off"}`)}
	_, err = provider.ResolveThinkingIntent(&fallback, rules["alias"].ThinkingPolicy)
	require.Error(t, err)
	availability := d.GetThinkingCapabilities([]string{"alias"})["alias"]
	require.True(t, availability.On)
	require.False(t, availability.Off)
	require.Equal(t, []string{"low"}, availability.Efforts)
}

func TestThinkingPolicySurvivesWhenPrimaryHasNoEligibleChannel(t *testing.T) {
	policy := &model.ThinkingPolicy{DefaultMode: "on", AllowedModes: []string{"on"}}
	d := NewDispatcher([]model.ChannelConfig{{ID: 1, Name: "only-fallback", Type: model.ProviderOpenAI, Models: []string{"backup"}, ModelCapabilities: map[string]model.ModelCapabilityConfig{"backup": {Thinking: &model.ThinkingConfig{Profile: "unsupported"}}}, RouteRules: map[string]model.ModelRouteRule{"primary": {Providers: []model.RouteProvider{}, ThinkingPolicy: policy}}}})
	req, err := d.withThinkingIntent(&model.ChatCompletionRequest{Model: "primary"})
	require.NoError(t, err)
	require.NotNil(t, req.ThinkingIntent)
	require.Equal(t, "on", req.ThinkingIntent.Mode)
	fallback := *req
	fallback.Model = "backup"
	channels, err := d.candidateChannelsForRequest(context.Background(), &fallback, "chat")
	require.Empty(t, channels)
	require.Error(t, err)
}
