package controlplane

import (
	"context"
	"slices"
	"sync"

	"github.com/ifnodoraemon/airoute/internal/model"
)

type thinkingProbeEvidence struct {
	parameter string
	validated bool
	baseline  bool
}

func thinkingProbeControls(ch *model.ChannelConfig, name string, profile *model.ThinkingProfile) []model.ThinkingControl {
	controls := []model.ThinkingControl{{Mode: "off"}}
	for _, effort := range model.ThinkingEfforts() {
		controls = append(controls, model.ThinkingControl{Mode: "on", Effort: effort})
	}
	for _, effort := range configuredProbeEfforts(ch, name) {
		if model.ValidThinkingEffort(effort) && !slices.Contains(model.ThinkingEfforts(), effort) {
			controls = append(controls, model.ThinkingControl{Mode: "on", Effort: effort})
			profile.Efforts = append(profile.Efforts, effort)
		}
	}
	return controls
}

func (p *DownstreamProber) probeThinkingControls(ctx context.Context, ch *model.ChannelConfig, name string, profile model.ThinkingProfile, controls []model.ThinkingControl, evidence thinkingProbeEvidence) map[string]ThinkingProbeCheck {
	checks := make(map[string]ThinkingProbeCheck, len(controls))
	var mu sync.Mutex
	var group sync.WaitGroup
	limit := make(chan struct{}, 3)
	for _, control := range controls {
		group.Go(func() {
			select {
			case limit <- struct{}{}:
				defer func() { <-limit }()
			case <-ctx.Done():
				return
			}
			check := p.checkThinkingControl(ctx, ch, name, profile, control, evidence)
			key := control.Effort
			if control.Mode == "off" {
				key = "off"
			}
			mu.Lock()
			checks[key] = check
			mu.Unlock()
		})
	}
	group.Wait()
	return checks
}

func (result *ThinkingProbeResult) finishDiscovery(previous, profile model.ThinkingProfile, validated bool) {
	confirmed := false
	for _, check := range result.Checks {
		confirmed = confirmed || check.Status == "supported"
	}
	if !validated {
		result.Message = "已实际探测，未确认可调档位"
	}
	if previous.ID == profile.ID || !confirmed || result.Thinking == nil {
		return
	}
	// Replace a fixed/unknown preset only after successful probes. Never inherit
	// the new adapter's untested defaults as confirmed model capabilities.
	result.Thinking.Profile = profile.ID
	if result.Thinking.Efforts == nil {
		result.Thinking.Efforts = []string{}
	}
	if result.Thinking.CanDisable == nil {
		disabled := result.Checks["off"].Status == "supported"
		result.Thinking.CanDisable = &disabled
	}
	result.Capability.Profile = profile.ID
}
