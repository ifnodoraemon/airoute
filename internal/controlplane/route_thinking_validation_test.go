package controlplane

import (
	"context"
	"errors"
	"testing"

	"github.com/ifnodoraemon/airoute/internal/model"
	"github.com/ifnodoraemon/airoute/internal/storage"
)

func thinkingRouteFixture() (*fakeRouteStore, []ModelProviderUpdate) {
	store := newFakeRouteStore()
	store.channels = []*storage.ChannelRecord{
		{ID: 1, Name: "gpu", Type: model.ProviderCustom, Status: "active", Models: []string{"actual-gpu"}, ModelCapabilities: map[string]model.ModelCapabilityConfig{"actual-gpu": {Thinking: &model.ThinkingConfig{Profile: "vllm-deepseek41"}}}},
		{ID: 2, Name: "proxy", Type: model.ProviderCustom, Status: "active", Models: []string{"actual-proxy"}, ModelCapabilities: map[string]model.ModelCapabilityConfig{"actual-proxy": {Thinking: &model.ThinkingConfig{Profile: "reasoning-only"}}}},
	}
	bindings := []ModelProviderUpdate{{ChannelID: 1, MappedModel: "actual-gpu"}, {ChannelID: 2, MappedModel: "actual-proxy"}}
	return store, bindings
}

func TestRouteThinkingOnlyAdvertisesSupportedEfforts(t *testing.T) {
	for _, tc := range []struct {
		name   string
		policy *model.ThinkingPolicy
		valid  bool
	}{
		{"four real GPU levels", &model.ThinkingPolicy{DefaultMode: "on", DefaultEffort: "low", AllowedEfforts: []string{"low", "high", "xhigh", "max"}}, true},
		{"minimal unsupported even with valid default", &model.ThinkingPolicy{DefaultMode: "on", DefaultEffort: "low", AllowedEfforts: []string{"minimal", "low"}}, false},
		{"medium unsupported", &model.ThinkingPolicy{DefaultMode: "on", AllowedEfforts: []string{"medium"}}, false},
		{"unsupported default", &model.ThinkingPolicy{DefaultMode: "on", DefaultEffort: "medium"}, false},
		{"on with native default", &model.ThinkingPolicy{DefaultMode: "on"}, true},
		{"empty range cannot enable thinking", &model.ThinkingPolicy{DefaultMode: "on", AllowedEfforts: []string{}}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			store, bindings := thinkingRouteFixture()
			err := NewRouteService(store, nil, nil).UpdateModelRoute(context.Background(), &UpdateModelRouteRequest{Model: "public-alias", ThinkingPolicy: tc.policy, ProviderUpdates: bindings})
			if tc.valid {
				if err != nil {
					t.Fatal(err)
				}
			} else {
				var invalid *ValidationError
				if !errors.As(err, &invalid) {
					t.Fatalf("expected validation error, got %v", err)
				}
				if store.applyCalls != 0 {
					t.Fatal("unsupported thinking policy was persisted")
				}
			}
		})
	}
}

func TestRouteThinkingUsesUnionOfActualChannelCapabilities(t *testing.T) {
	store := newFakeRouteStore()
	store.channels = []*storage.ChannelRecord{
		{ID: 1, Name: "google", Type: model.ProviderGemini, Status: "active", ModelCapabilities: map[string]model.ModelCapabilityConfig{"actual-google": {Thinking: &model.ThinkingConfig{Profile: "gemini-levels"}}}},
		{ID: 2, Name: "glm", Type: model.ProviderCustom, Status: "active", ModelCapabilities: map[string]model.ModelCapabilityConfig{"actual-glm": {Thinking: &model.ThinkingConfig{Profile: "glm53"}}}},
	}
	err := NewRouteService(store, nil, nil).UpdateModelRoute(context.Background(), &UpdateModelRouteRequest{Model: "alias", ProviderUpdates: []ModelProviderUpdate{{ChannelID: 1, MappedModel: "actual-google"}, {ChannelID: 2, MappedModel: "actual-glm"}}, ThinkingPolicy: &model.ThinkingPolicy{DefaultMode: "on", DefaultEffort: "high", AllowedEfforts: []string{"low", "medium", "high", "max"}}})
	if err != nil {
		t.Fatalf("valid union rejected: %v", err)
	}
	if store.applyCalls != 1 {
		t.Fatal("valid route was not saved")
	}
}

func TestRouteThinkingExcludesInactiveUnknownAndUnboundChannels(t *testing.T) {
	for _, status := range []string{"disabled", "inactive", "active"} {
		t.Run(status, func(t *testing.T) {
			store, bindings := thinkingRouteFixture()
			store.channels = append(store.channels, &storage.ChannelRecord{ID: 3, Name: "extra", Type: model.ProviderOpenAI, Status: status, ModelCapabilities: map[string]model.ModelCapabilityConfig{"extra-model": {Thinking: &model.ThinkingConfig{Profile: "openai-max"}}}})
			if status != "active" {
				bindings = append(bindings, ModelProviderUpdate{ChannelID: 3, MappedModel: "extra-model"})
			}
			err := NewRouteService(store, nil, nil).UpdateModelRoute(context.Background(), &UpdateModelRouteRequest{Model: "alias", ProviderUpdates: bindings, ThinkingPolicy: &model.ThinkingPolicy{DefaultMode: "on", AllowedEfforts: []string{"low", "medium"}}})
			var invalid *ValidationError
			if !errors.As(err, &invalid) || store.applyCalls != 0 {
				t.Fatalf("inactive or unbound capability counted: %v", err)
			}
		})
	}
}

func TestRouteThinkingValidatesPolicyOnlyUpdatesAgainstExistingBindings(t *testing.T) {
	store, bindings := thinkingRouteFixture()
	store.routes = map[string]model.ModelRouteRule{"alias": {Providers: []model.RouteProvider{{ChannelID: 1, MappedModel: "actual-gpu"}}}}
	svc := NewRouteService(store, nil, nil)
	err := svc.UpdateModelRoute(context.Background(), &UpdateModelRouteRequest{Model: "alias", ThinkingPolicy: &model.ThinkingPolicy{DefaultMode: "on", DefaultEffort: "medium"}})
	var invalid *ValidationError
	if !errors.As(err, &invalid) || store.applyCalls != 0 {
		t.Fatalf("policy-only update bypassed validation: %v", err)
	}
	err = svc.UpdateModelRoute(context.Background(), &UpdateModelRouteRequest{Model: "alias", ThinkingPolicy: &model.ThinkingPolicy{DefaultMode: "on", DefaultEffort: "high"}})
	if err != nil || store.lastBundle.Providers != nil {
		t.Fatalf("valid policy-only update changed bindings: %+v %v", store.lastBundle, err)
	}
	// A mapping-only update must preserve and validate the saved policy too.
	store.applyCalls = 0
	store.routes["alias"] = model.ModelRouteRule{Providers: []model.RouteProvider{{ChannelID: 1, MappedModel: "actual-gpu"}}, ThinkingPolicy: &model.ThinkingPolicy{DefaultMode: "on", DefaultEffort: "high"}}
	err = svc.UpdateModelRoute(context.Background(), &UpdateModelRouteRequest{Model: "alias", ProviderUpdates: bindings[1:]})
	if !errors.As(err, &invalid) || store.applyCalls != 0 {
		t.Fatalf("mapping-only update bypassed saved policy: %v", err)
	}
}

func TestRouteThinkingOnlyToggleAndSingleLevelModels(t *testing.T) {
	for _, tc := range []struct {
		name, profile, effort string
		valid                 bool
	}{
		{"toggle has model default", "reasoning-only", "", true},
		{"toggle does not invent low", "reasoning-only", "low", false},
		{"single high level", "gemma", "high", true},
		{"single level does not invent max", "gemma", "max", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			store := newFakeRouteStore()
			kind := model.ProviderCustom
			if tc.profile == "gemma" {
				kind = model.ProviderGemini
			}
			store.channels = []*storage.ChannelRecord{{ID: 1, Name: "channel", Type: kind, ModelCapabilities: map[string]model.ModelCapabilityConfig{"actual": {Thinking: &model.ThinkingConfig{Profile: tc.profile}}}}}
			err := NewRouteService(store, nil, nil).UpdateModelRoute(context.Background(), &UpdateModelRouteRequest{Model: "alias", ProviderUpdates: []ModelProviderUpdate{{ChannelID: 1, MappedModel: "actual"}}, ThinkingPolicy: &model.ThinkingPolicy{DefaultMode: "on", DefaultEffort: tc.effort}})
			if (err == nil) != tc.valid {
				t.Fatalf("valid=%v err=%v", tc.valid, err)
			}
		})
	}
}

func TestRouteThinkingDoesNotAdvertiseUnsupportedOffMode(t *testing.T) {
	store, bindings := thinkingRouteFixture()
	err := NewRouteService(store, nil, nil).UpdateModelRoute(context.Background(), &UpdateModelRouteRequest{Model: "alias", ProviderUpdates: bindings[1:], ThinkingPolicy: &model.ThinkingPolicy{DefaultMode: "auto", AllowedModes: []string{"auto", "off"}}})
	var invalid *ValidationError
	if !errors.As(err, &invalid) || store.applyCalls != 0 {
		t.Fatalf("unsupported off mode accepted: %v", err)
	}
}
