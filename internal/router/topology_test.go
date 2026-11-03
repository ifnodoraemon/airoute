package router

import (
	"testing"

	"github.com/ifnodoraemon/airoute/internal/model"
)

// B-09: the primary tier is the LOWEST configured priority, exactly like the
// scheduler groups priorities and treats group 0 as primary. The topology view used
// to hardcode "priority == 1", so an operator who numbered every channel 2 saw zero
// primary channels and zero weight share while routing used them as primary.
func TestModelRoutesPrimaryTierUsesLowestPriority(t *testing.T) {
	d := NewDispatcher([]model.ChannelConfig{
		{ID: 1, Name: "p2-a", Type: model.ProviderOpenAI, Status: "active", Models: []string{"m"}, Priority: 2, Weight: 3},
		{ID: 2, Name: "p2-b", Type: model.ProviderOpenAI, Status: "active", Models: []string{"m"}, Priority: 2, Weight: 1},
		{ID: 3, Name: "p3", Type: model.ProviderOpenAI, Status: "active", Models: []string{"m"}, Priority: 3, Weight: 9},
	})

	routes := d.GetModelRoutes()
	if len(routes) != 1 {
		t.Fatalf("routes = %d, want 1", len(routes))
	}
	route := routes[0]
	if route.PrimaryProvidersCount != 2 || route.FallbackProvidersCount != 1 {
		t.Fatalf("primary/fallback = %d/%d, want 2/1", route.PrimaryProvidersCount, route.FallbackProvidersCount)
	}
	if route.TotalPrimaryWeight != 4 {
		t.Fatalf("total primary weight = %d, want 4 (3+1)", route.TotalPrimaryWeight)
	}
	if !route.HasFallbackTier {
		t.Fatal("a priority-3 channel must be reported as a fallback tier")
	}
	percent := map[string]float64{}
	for _, info := range route.Providers {
		percent[info.ChannelName] = info.WeightPercent
	}
	if percent["p2-a"] != 75 || percent["p2-b"] != 25 {
		t.Fatalf("primary weight percents = %v, want p2-a=75 p2-b=25", percent)
	}
	if percent["p3"] != 0 {
		t.Fatalf("fallback-tier weight percent = %v, want 0", percent["p3"])
	}
}

// A model whose channels all share one (non-1) priority is a single-tier topology:
// every channel is primary and the lowest priority is the tier marker.
func TestModelRoutesSingleNonOnePriorityTier(t *testing.T) {
	d := NewDispatcher([]model.ChannelConfig{
		{ID: 1, Name: "only", Type: model.ProviderOpenAI, Status: "active", Models: []string{"m"}, Priority: 2, Weight: 5},
	})

	route := d.GetModelRoutes()[0]
	if route.PrimaryProvidersCount != 1 || route.FallbackProvidersCount != 0 {
		t.Fatalf("primary/fallback = %d/%d, want 1/0", route.PrimaryProvidersCount, route.FallbackProvidersCount)
	}
	if route.TotalPrimaryWeight != 5 {
		t.Fatalf("total primary weight = %d, want 5", route.TotalPrimaryWeight)
	}
	if route.HasFallbackTier {
		t.Fatal("a single-tier model must not report a fallback tier")
	}
	if got := route.Providers[0].WeightPercent; got != 100 {
		t.Fatalf("weight percent = %v, want 100", got)
	}
}

// Inactive channels are part of the configured primary tier but carry no traffic,
// so they must not contribute weigh; they still count as primary for the topology.
func TestModelRoutesInactivePrimaryChannelHasNoWeight(t *testing.T) {
	d := NewDispatcher([]model.ChannelConfig{
		{ID: 1, Name: "disabled", Type: model.ProviderOpenAI, Status: "disabled", Models: []string{"m"}, Priority: 2, Weight: 100},
		{ID: 2, Name: "active", Type: model.ProviderOpenAI, Status: "active", Models: []string{"m"}, Priority: 2, Weight: 5},
	})

	route := d.GetModelRoutes()[0]
	if route.PrimaryProvidersCount != 2 {
		t.Fatalf("primary providers = %d, want 2", route.PrimaryProvidersCount)
	}
	if route.TotalPrimaryWeight != 5 {
		t.Fatalf("total primary weight = %d, want 5 (inactive channel excluded)", route.TotalPrimaryWeight)
	}
}

// The snapshot renders the same provider list/priority the tier logic uses.
func TestRoutingSnapshotKeepsConfiguredPriorities(t *testing.T) {
	d := NewDispatcher([]model.ChannelConfig{
		{ID: 1, Name: "p2", Type: model.ProviderOpenAI, Status: "active", Models: []string{"m"}, Priority: 2, Weight: 1},
		{ID: 2, Name: "p3", Type: model.ProviderOpenAI, Status: "active", Models: []string{"m"}, Priority: 3, Weight: 1},
	})
	snapshot := d.GetRoutingSnapshot("m")
	if len(snapshot.Steps) != 1 || len(snapshot.Steps[0].Providers) != 2 {
		t.Fatalf("unexpected snapshot: %+v", snapshot)
	}
	if snapshot.Steps[0].Providers[0].Name != "p2" || snapshot.Steps[0].Providers[0].Priority != 2 {
		t.Fatalf("provider order/priority = %+v", snapshot.Steps[0].Providers[0])
	}
}
