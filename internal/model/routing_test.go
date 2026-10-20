package model

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRoutePolicyOverridesCapabilities(t *testing.T) {
	rules := map[string]ModelRouteRule{
		"vendor/*":       {Providers: []RouteProvider{{ChannelID: 1, Priority: 2, Weight: 19, MappedModel: "remote/*"}}},
		"vendor/deleted": {Disabled: true},
	}
	chosen := ChannelConfig{ID: 1, Name: "chosen", Models: []string{"*"}, Priority: 1, Weight: 10, RouteRules: rules}
	other := ChannelConfig{ID: 2, Name: "other", Models: []string{"*"}, RouteRules: rules}
	require.True(t, chosen.SupportsModel("vendor/model"))
	require.False(t, other.SupportsModel("vendor/model"))
	require.Equal(t, "remote/model", chosen.GetUpstreamModel("vendor/model"))
	require.Equal(t, 19, chosen.ForModel("vendor/model").Weight)
	require.Equal(t, 10, chosen.Weight)
	require.False(t, chosen.SupportsModel("vendor/deleted"))
	require.False(t, other.SupportsModel("vendor/deleted"))
	require.False(t, chosen.SupportsModel("chosen/vendor/deleted"), "channel prefixes must not bypass deletion")
	require.Equal(t, 19, chosen.ForModel("chosen/vendor/model").Weight)
	require.Equal(t, "remote/model", chosen.GetUpstreamModel("chosen/vendor/model"))
}
