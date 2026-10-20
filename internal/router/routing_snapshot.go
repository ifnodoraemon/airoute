package router

import (
	"context"
	"sort"

	"github.com/ifnodoraemon/airoute/internal/model"
)

// RoutingSnapshot contains only active data-plane configuration. It deliberately
// omits credentials and does not run selectors, consume canaries or send traffic.
type RoutingSnapshot struct {
	Model      string        `json:"model"`
	Models     []string      `json:"models"`
	Steps      []RoutingStep `json:"steps"`
	ChainError string        `json:"chain_error,omitempty"`
}

type RoutingStep struct {
	Model         string            `json:"model"`
	Modality      string            `json:"modality"`
	FallbackModel string            `json:"fallback_model,omitempty"`
	Providers     []RoutingProvider `json:"providers"`
}

type RoutingProvider struct {
	ChannelID         int64           `json:"channel_id"`
	Name              string          `json:"name"`
	Type              string          `json:"type"`
	Priority          int             `json:"priority"`
	Weight            int             `json:"weight"`
	MappedModel       string          `json:"mapped_model"`
	Protocols         []string        `json:"protocols"`
	Eligible          bool            `json:"eligible"`
	ProviderSupported bool            `json:"provider_supported"`
	Breaker           BreakerSnapshot `json:"breaker"`
}

func (d *Dispatcher) GetRoutingSnapshot(selected string) RoutingSnapshot {
	routes := map[string]*ModelRouteDetail{}
	names := map[string]bool{}
	for _, route := range d.GetModelRoutes() {
		routes[route.Model] = route
		names[route.Model] = true
	}
	fallbacks := d.GetAllModelFallbacks()
	for from, to := range fallbacks {
		names[from] = true
		if to != "" {
			names[to] = true
		}
	}
	result := RoutingSnapshot{Models: []string{}, Steps: []RoutingStep{}}
	for name := range names {
		result.Models = append(result.Models, name)
	}
	sort.Strings(result.Models)
	if selected == "" && len(result.Models) > 0 {
		selected = result.Models[0]
	}
	result.Model = selected
	if selected == "" {
		return result
	}
	ctx := context.Background()
	for current := selected; current != ""; {
		step := RoutingStep{Model: current, Modality: InferModality(current), FallbackModel: fallbacks[current], Providers: []RoutingProvider{}}
		if route := routes[current]; route != nil {
			for _, info := range route.Providers {
				step.Providers = append(step.Providers, d.routingProvider(info))
			}
		}
		result.Steps = append(result.Steps, step)
		if step.FallbackModel == "" {
			break
		}
		next, err := nextFallbackContext(ctx, current, step.FallbackModel)
		if err != nil {
			result.ChainError = "兜底链存在循环或超过运行时允许的深度"
			break
		}
		ctx, current = next, step.FallbackModel
	}
	return result
}

func (d *Dispatcher) routingProvider(info *ModelProviderInfo) RoutingProvider {
	d.mu.RLock()
	_, supported := d.providers[model.ProviderType(info.ChannelType)]
	d.mu.RUnlock()
	breaker := d.circuitBreaker.Snapshot(info.ChannelName)
	return RoutingProvider{
		ChannelID: info.ChannelID, Name: info.ChannelName, Type: info.ChannelType,
		Priority: info.Priority, Weight: info.Weight, MappedModel: info.MappedModel,
		Protocols: info.Protocols, Breaker: breaker, ProviderSupported: supported,
		Eligible: supported && breaker.CanExecute,
	}
}
