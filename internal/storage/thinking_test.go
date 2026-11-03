package storage

import (
	"context"
	"github.com/ifnodoraemon/airoute/internal/model"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestThinkingConfigurationAndAuditRoundTrip(t *testing.T) {
	repo := newIndependentRouteRepo(t)
	channel := seedRouteChannel(t, repo, "thinking")
	channel.ModelCapabilities = map[string]model.ModelCapabilityConfig{"model-a": {Thinking: &model.ThinkingConfig{Profile: "deepseek", Efforts: []string{"low"}}}}
	require.NoError(t, repo.UpdateChannel(channel))
	policy := &model.ThinkingPolicy{DefaultMode: "on", DefaultEffort: "low", AllowedModes: []string{"on"}, AllowedEfforts: []string{"low"}}
	for _, name := range []string{"alias-a", "alias-b"} {
		require.NoError(t, repo.ApplyModelRouteBundle(context.Background(), &ModelRouteBundle{Model: name, Providers: []ModelRouteProvider{{ChannelID: channel.ID, MappedModel: "model-a", Weight: 1}}, ThinkingPolicy: policy}))
	}
	loaded, err := repo.ToModelChannels()
	require.NoError(t, err)
	require.Len(t, loaded, 1)
	for _, alias := range []string{"alias-a", "alias-b"} {
		profile, source := model.ResolveThinkingProfile(&loaded[0], alias)
		require.Equal(t, "deepseek", profile.ID)
		require.Equal(t, "configured", source)
		require.Equal(t, policy, loaded[0].ThinkingPolicyFor(alias))
	}
	// Saving route policy does not rewrite shared capability facts.
	unchanged, err := findChannel(t, repo, channel.ID)
	require.NoError(t, err)
	require.Equal(t, channel.ModelCapabilities, unchanged.ModelCapabilities)
	tokens := 0
	require.NoError(t, repo.RecordUsageLogContext(context.Background(), &UsageLogRecord{TraceID: "thinking-evidence", Model: "alias-a", Channel: channel.Name, StatusCode: 200, Thinking: &model.ThinkingObservation{Mode: "off", Source: "configured", ReasoningTokens: &tokens, UpstreamModel: "model-a"}}))
	logs, _, err := repo.ListUsageLogsWithFilterAndCount(LogFilter{TraceID: "thinking-evidence", Limit: 1})
	require.NoError(t, err)
	require.Len(t, logs, 1)
	require.NotNil(t, logs[0].Thinking.ReasoningTokens)
	require.Zero(t, *logs[0].Thinking.ReasoningTokens)
	// Migration replay remains safe, including databases upgraded in two replicas.
	require.NoError(t, addThinkingConfiguration(repo.db))
}

func TestDeletingRouteClearsItsThinkingPolicy(t *testing.T) {
	repo := newIndependentRouteRepo(t)
	require.NoError(t, repo.ApplyModelRouteBundle(context.Background(), &ModelRouteBundle{Model: "deleted", ThinkingPolicy: &model.ThinkingPolicy{DefaultMode: "on"}}))
	require.NoError(t, repo.DeleteModelRoutes(context.Background(), []string{"deleted"}))
	require.NoError(t, repo.ApplyModelRouteBundle(context.Background(), &ModelRouteBundle{Model: "deleted"}))
	rules, err := repo.ListModelRouteRules()
	require.NoError(t, err)
	require.Nil(t, rules["deleted"].ThinkingPolicy)
}
