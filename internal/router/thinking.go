package router

import (
	"context"
	"encoding/json"
	"github.com/ifnodoraemon/airoute/internal/model"
	"github.com/ifnodoraemon/airoute/internal/provider"
	"slices"
	"strings"
)

type ThinkingChoices struct {
	On      bool     `json:"on"`
	Off     bool     `json:"off"`
	Efforts []string `json:"efforts"`
}
type ThinkingAvailability struct {
	MinimumOutputTokens map[string]int `json:"minimum_output_tokens,omitempty"`
	ThinkingChoices
	Known           bool                  `json:"known"`
	Summary         bool                  `json:"summary"`
	Source          string                `json:"source"`
	Channels        int                   `json:"channels"`
	UnknownChannels int                   `json:"unknown_channels"`
	Policy          *model.ThinkingPolicy `json:"policy,omitempty"`
	Tools           ThinkingChoices       `json:"tools"`
}

// GetThinkingCapabilities exposes aggregate choices without exposing endpoints,
// credentials or channel identities to console customers. Choices describe at
// least one eligible provider; dispatch uses the same resolver before selection.
func (d *Dispatcher) GetThinkingCapabilities(names []string) map[string]ThinkingAvailability {
	result := make(map[string]ThinkingAvailability, len(names))
	for _, name := range names {
		channels := d.ListChannelsForModel(name, "chat")
		if len(channels) == 0 {
			channels = d.ListChannelsForModel(name, "")
		}
		availability := ThinkingAvailability{Policy: d.thinkingPolicyForModel(name), Channels: len(channels), ThinkingChoices: ThinkingChoices{Efforts: []string{}}, Tools: ThinkingChoices{Efforts: []string{}}}
		for _, channel := range channels {
			profile, source := model.ResolveThinkingProfile(channel, name)
			availability.Known = availability.Known || (profile.ID != "" && profile.ID != "unknown")
			availability.Summary = availability.Summary || (profile.Output == "summary" && strings.HasPrefix(profile.Wire, "gemini-"))
			if profile.ID == "" || profile.ID == "unknown" {
				availability.UnknownChannels++
			}
			if availability.Source == "" {
				availability.Source = source
			} else if availability.Source != source {
				availability.Source = "mixed"
			}
			if availability.Policy == nil {
				availability.Policy = channel.ThinkingPolicyFor(name)
			}
			for _, tools := range []bool{false, true} {
				choices := &availability.ThinkingChoices
				if tools {
					choices = &availability.Tools
				}
				eligible := func(control model.ThinkingControl) bool {
					raw, _ := json.Marshal(control)
					outputLimit := 131072
					req := &model.ChatCompletionRequest{Model: name, MaxTokens: &outputLimit, Extra: model.ExtraFields{"thinking_control": raw}}
					if tools {
						req.Tools = []model.Tool{{Type: "function", Function: map[string]any{"name": "capability_check"}}}
					}
					prepared, err := provider.PrepareThinkingRequest(req, channel)
					if err == nil && prepared.PreparedThinking != nil && prepared.PreparedThinking.Control.Mode == "on" && profile.Wire == "claude-budget" {
						effort := prepared.PreparedThinking.Control.Effort
						if effort == "" {
							effort = profile.DefaultEffort
						}
						key := control.Effort
						if key == "" {
							key = "default"
						}
						minimum := profile.Budgets[effort] + 1
						if availability.MinimumOutputTokens == nil {
							availability.MinimumOutputTokens = make(map[string]int)
						}
						availability.MinimumOutputTokens[key] = max(availability.MinimumOutputTokens[key], minimum)
					}
					return err == nil
				}
				// Resolve every candidate: output budgets may differ even when
				// a previous provider already supports the same mode.
				canEnable := eligible(model.ThinkingControl{Mode: "on"})
				canDisable := eligible(model.ThinkingControl{Mode: "off"})
				choices.On = choices.On || canEnable
				choices.Off = choices.Off || canDisable
				for _, effort := range profile.Efforts {
					if eligible(model.ThinkingControl{Mode: "on", Effort: effort}) && !slices.Contains(choices.Efforts, effort) {
						choices.Efforts = append(choices.Efforts, effort)
					}
				}
			}
		}
		result[name] = availability
	}
	return result
}

func beginThinkingTrace(ctx context.Context, req *model.ChatCompletionRequest, ch *model.ChannelConfig) *model.ThinkingTrace {
	trace := model.ThinkingTraceFromContext(ctx)
	profile, source := model.ResolveThinkingProfile(ch, req.Model)
	observation := model.ThinkingObservation{Mode: provider.ThinkingRequestMode(req), Source: source, Profile: profile.ID, UpstreamModel: ch.GetUpstreamModel(req.Model)}
	if prepared := req.PreparedThinking; prepared != nil {
		observation.Mode = prepared.Control.Mode
		observation.Effort = prepared.Control.Effort
		observation.IncludeSummary = prepared.Control.IncludeSummary
		if observation.Mode == "on" && observation.Effort == "" {
			observation.Effort = profile.DefaultEffort
		}
	}
	trace.Set(observation)
	return trace
}

// Route policy exists even when all providers bound to that route are disabled
// or gone. Read it independently so a no-provider fallback cannot lose policy.
func (d *Dispatcher) thinkingPolicyForModel(name string) *model.ThinkingPolicy {
	d.mu.RLock()
	defer d.mu.RUnlock()
	for i := range d.channels {
		if strings.HasPrefix(name, d.channels[i].Name+"/") {
			return d.channels[i].ThinkingPolicyFor(name)
		}
	}
	for i := range d.channels {
		if policy := d.channels[i].ThinkingPolicyFor(name); policy != nil {
			return policy
		}
	}
	return nil
}
