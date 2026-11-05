package controlplane

import (
	"encoding/json"
	"fmt"

	"github.com/ifnodoraemon/airoute/internal/model"
	"github.com/ifnodoraemon/airoute/internal/provider"
	"github.com/ifnodoraemon/airoute/internal/storage"
)

// Validate both route defaults and every advertised choice against the actual
// mapped providers. Providers contribute a union: a level need not be supported
// by every channel, but at least one enabled channel must support it.
func (s *RouteService) validateThinkingDefault(req *UpdateModelRouteRequest, providers []storage.ModelRouteProvider) error {
	if req.ThinkingPolicy == nil && providers == nil {
		return nil
	}
	if req.ThinkingPolicy != nil && !req.ThinkingPolicy.Active() {
		return nil
	}
	rules, err := s.store.ListModelRouteRules()
	if err != nil {
		return &RouteStageError{Stage: RouteStageCollect, Err: err}
	}
	existing := rules[req.Model]
	policy := req.ThinkingPolicy
	if policy == nil {
		policy = existing.ThinkingPolicy
	}
	if !policy.Active() {
		return nil
	}
	if providers == nil {
		providers = existing.Providers
	}
	channels, err := s.store.ListChannels()
	if err != nil {
		return &RouteStageError{Stage: RouteStageCollect, Err: err}
	}
	candidates := make([]*model.ChannelConfig, 0, len(channels))
	for _, record := range channels {
		if record.Status != "" && record.Status != "active" {
			continue
		}
		ch := &model.ChannelConfig{ID: record.ID, Name: record.Name, Type: record.Type, Protocols: record.Protocols, Models: record.Models, ModelMapping: record.ModelMapping, ModelCapabilities: record.ModelCapabilities, RouteRules: map[string]model.ModelRouteRule{req.Model: {Providers: providers, ThinkingPolicy: policy}}}
		if ch.SupportsModel(req.Model) {
			candidates = append(candidates, ch)
		}
	}
	supports := func(control model.ThinkingControl) bool {
		for _, ch := range candidates {
			profile, _ := model.ResolveThinkingProfile(ch, req.Model)
			if model.ThinkingProfileCompatible(model.ChatProviderType(ch, &model.ChatCompletionRequest{}), profile) && profile.Validate(control) == nil {
				return true
			}
		}
		return false
	}
	for _, effort := range policy.AllowedEfforts {
		if !supports(model.ThinkingControl{Mode: "on", Effort: effort}) {
			return &ValidationError{Message: fmt.Sprintf("思考档位 %q 没有已启用的上游模型支持；请按渠道实际能力设置允许范围", effort)}
		}
	}
	for _, mode := range policy.AllowedModes {
		if mode != "auto" && !supports(model.ThinkingControl{Mode: mode}) {
			return &ValidationError{Message: fmt.Sprintf("思考模式 %q 没有已启用的上游模型支持", mode)}
		}
	}
	// Use the request adapter itself, including how it resolves a default effort
	// within the allowed range. Discovery must not apply one call's output limit.
	limit := 131072
	for _, ch := range candidates {
		request := &model.ChatCompletionRequest{Model: req.Model, MaxTokens: &limit, Extra: model.ExtraFields{"thinking_control": json.RawMessage(`{"mode":"auto"}`)}}
		if _, err := provider.PrepareThinkingRequest(request, ch); err == nil {
			return nil
		}
	}
	return &ValidationError{Message: fmt.Sprintf("没有渠道能满足路由的默认思考设置（%s %s）；请先确认映射后的上游模型能力", policy.DefaultMode, policy.DefaultEffort)}
}
