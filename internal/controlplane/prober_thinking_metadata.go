package controlplane

import "github.com/ifnodoraemon/airoute/internal/model"

// capabilities.reasoning_effort is an explicit upstream model declaration,
// unlike supported_parameters, which only says that a parameter exists.
type thinkingModelMetadata struct {
	ID           string `json:"id"`
	Capabilities struct {
		ReasoningEffort []string `json:"reasoning_effort"`
	} `json:"capabilities"`
}

func metadataThinkingCapabilities(kind model.ProviderType, models []thinkingModelMetadata) map[string]model.ThinkingCapability {
	capabilities := map[string]model.ThinkingCapability{}
	for _, upstream := range models {
		if upstream.ID == "" || upstream.Capabilities.ReasoningEffort == nil {
			continue
		}
		off, valid := false, true
		efforts := []string{}
		for _, effort := range upstream.Capabilities.ReasoningEffort {
			if effort == "minimal" {
				continue
			}
			if effort == "none" {
				off = true
			} else if model.ValidThinkingEffort(effort) {
				efforts = append(efforts, effort)
			} else {
				valid = false
			}
		}
		if !valid {
			continue
		}
		channel := &model.ChannelConfig{Type: kind, ModelCapabilities: map[string]model.ModelCapabilityConfig{upstream.ID: {Thinking: &model.ThinkingConfig{CanDisable: &off, Efforts: efforts}}}}
		profile, source := model.ResolveUpstreamThinkingProfile(channel, upstream.ID)
		if source != "invalid_configuration" {
			capabilities[upstream.ID] = profile.Capability("metadata")
		}
	}
	return capabilities
}

func completeProbeThinking(result *ProbeResult) {
	if result.ThinkingCapabilities == nil {
		result.ThinkingCapabilities = map[string]model.ThinkingCapability{}
	}
	channel := &model.ChannelConfig{Type: result.Type}
	for _, name := range result.Models {
		if _, exists := result.ThinkingCapabilities[name]; !exists {
			profile, source := model.ResolveUpstreamThinkingProfile(channel, name)
			result.ThinkingCapabilities[name] = profile.Capability(source)
		}
	}
}
