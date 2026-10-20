package model

import "strings"

// RouteProvider belongs to one public model route, independently of the
// channel's capability declaration and default scheduling settings.
type RouteProvider struct {
	ChannelID   int64  `json:"channel_id"`
	Priority    int    `json:"priority"`
	Weight      int    `json:"weight"`
	MappedModel string `json:"mapped_model"`
}

// ModelRouteRule overrides automatic routing inferred from channel capabilities.
// A nil Providers list inherits capabilities; an empty list selects no channel.
// Disabled persists an explicit deletion so a channel edit cannot recreate it.
type ModelRouteRule struct {
	Disabled  bool
	Providers []RouteProvider
}

func (c *ChannelConfig) routeProvider(modelName string) (RouteProvider, bool, bool) {
	rule, pattern, exists := c.routeRule(modelName)
	if _, exact := c.RouteRules[modelName]; !exact && strings.HasPrefix(modelName, c.Name+"/") {
		base := strings.TrimPrefix(modelName, c.Name+"/")
		if baseRule, basePattern, found := c.routeRule(base); found {
			rule, pattern, exists, modelName = baseRule, basePattern, true, base
		}
	}
	if !exists || (!rule.Disabled && rule.Providers == nil) {
		return RouteProvider{}, false, false
	}
	if !rule.Disabled {
		for _, p := range rule.Providers {
			if p.ChannelID == c.ID {
				if p.MappedModel != "" {
					mapping := ChannelConfig{ModelMapping: map[string]string{pattern: p.MappedModel}}
					p.MappedModel = mapping.GetUpstreamModel(modelName)
				} else {
					p.MappedModel = modelName
				}
				return p, true, true
			}
		}
	}
	return RouteProvider{}, false, true
}

func (c *ChannelConfig) routeRule(name string) (ModelRouteRule, string, bool) {
	if rule, ok := c.RouteRules[name]; ok {
		return rule, name, true
	}
	// Match the most specific configured prefix. An exact deletion above always
	// overrides a wildcard route, so a broad capability cannot resurrect a model.
	pattern := ""
	for candidate := range c.RouteRules {
		if candidate == "*" && pattern == "" {
			pattern = candidate
		} else if strings.HasSuffix(candidate, "/*") && strings.HasPrefix(name, strings.TrimSuffix(candidate, "*")) && len(candidate) > len(pattern) {
			pattern = candidate
		}
	}
	if pattern != "" {
		return c.RouteRules[pattern], pattern, true
	}
	return ModelRouteRule{}, "", false
}

// ForModel returns a scheduling copy with this route's priority and weight.
// The shared channel and every other model's scheduling remain unchanged.
func (c *ChannelConfig) ForModel(modelName string) *ChannelConfig {
	copy := *c
	if p, allowed, _ := c.routeProvider(modelName); allowed {
		copy.Priority = p.Priority
		copy.Weight = p.Weight
	}
	return &copy
}
