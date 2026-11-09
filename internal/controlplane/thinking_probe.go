package controlplane

import (
	"context"
	"slices"
	"strings"
	"time"

	"github.com/ifnodoraemon/airoute/internal/model"
)

type ThinkingProbeCheck struct {
	Status string `json:"status"`
	Reason string `json:"reason"`
}

type ThinkingProbeResult struct {
	Model      string                        `json:"model"`
	Message    string                        `json:"message,omitempty"`
	Capability model.ThinkingCapability      `json:"capability"`
	Checks     map[string]ThinkingProbeCheck `json:"checks"`
	Thinking   *model.ThinkingConfig         `json:"thinking,omitempty"`
}

// ProbeThinking tests parameter handling on one physical model. An invalid enum
// is a negative control: accepting it prevents positive HTTP 200 responses from
// being advertised as proof of adjustable levels.
func (p *DownstreamProber) ProbeThinking(ctx context.Context, ch *model.ChannelConfig, name string) ThinkingProbeResult {
	ctx, cancel := context.WithTimeout(ctx, 40*time.Second)
	defer cancel()
	automatic := *ch
	if configured := ch.ModelCapabilities[name].Thinking; configured != nil && configured.Profile != "" {
		automatic.ModelCapabilities = map[string]model.ModelCapabilityConfig{name: {Thinking: &model.ThinkingConfig{Profile: configured.Profile}}}
	} else {
		automatic.ModelCapabilities = nil
	}
	base, source := model.ResolveUpstreamThinkingProfile(&automatic, name)
	result := ThinkingProbeResult{Model: name, Capability: base.Capability(source), Checks: map[string]ThinkingProbeCheck{}}
	profile := base
	if profile.ID == "" || profile.Wire == "none" || profile.Wire == "unknown" {
		profile = model.ThinkingParameterProfile(model.ChatProviderType(ch, &model.ChatCompletionRequest{}), name)
	}
	transport := model.ThinkingTransportProfile(profile)
	controls := thinkingProbeControls(ch, name, &transport)
	parameter, validated := p.thinkingNegativeControl(ctx, ch, name, transport)
	baseline := p.thinkingBaseline(ctx, ch, name, transport)
	evidence := thinkingProbeEvidence{parameter: parameter, validated: validated, baseline: baseline}
	result.Checks = p.probeThinkingControls(ctx, ch, name, transport, controls, evidence)
	result.collectConfirmed(transport)
	result.finishDiscovery(base, profile, validated)
	return result
}

func (p *DownstreamProber) thinkingNegativeControl(ctx context.Context, ch *model.ChannelConfig, name string, profile model.ThinkingProfile) (string, bool) {
	if len(profile.Budgets) > 0 || len(profile.Efforts) == 0 {
		return "", false
	}
	target, body, err := buildThinkingProbeRequest(ch, name, profile, model.ThinkingControl{Mode: "on", Effort: profile.Efforts[0]})
	if err != nil {
		return "", false
	}
	body, parameter := invalidThinkingProbe(body, profile, ch.Type)
	response, err := p.sendThinkingProbe(ctx, ch, target, body)
	return parameter, err == nil && response.rejects(parameter) && slices.ContainsFunc([]string{invalidProbeEffort, "supported values", "permitted", "should be", "must be one of"}, func(marker string) bool { return strings.Contains(strings.ToLower(string(response.body)), marker) })
}

func configuredProbeEfforts(ch *model.ChannelConfig, name string) []string {
	if config := ch.ModelCapabilities[name].Thinking; config != nil {
		return config.Efforts
	}
	return nil
}

func (p *DownstreamProber) thinkingBaseline(ctx context.Context, ch *model.ChannelConfig, name string, profile model.ThinkingProfile) bool {
	if !profile.On || !profile.Off || len(profile.Budgets) > 0 {
		return false
	}
	control := model.ThinkingControl{Mode: "on"}
	if slices.Contains(profile.Efforts, "high") {
		control.Effort = "high"
	} else if len(profile.Efforts) > 0 {
		control.Effort = profile.Efforts[0]
	}
	target, body, err := buildThinkingProbeRequest(ch, name, profile, control)
	if err != nil {
		return false
	}
	response, err := p.sendThinkingProbe(ctx, ch, target, body)
	valid, tokens, thought := response.observation()
	return err == nil && valid && (thought || (tokens != nil && *tokens > 0))
}

func (p *DownstreamProber) checkThinkingControl(ctx context.Context, ch *model.ChannelConfig, name string, profile model.ThinkingProfile, control model.ThinkingControl, evidence thinkingProbeEvidence) ThinkingProbeCheck {
	unknown := ThinkingProbeCheck{Status: "unknown", Reason: "上游未提供足够证据确认该选项"}
	if profile.Validate(control) != nil {
		unknown.Reason = "当前参数配置无法试探该选项"
		return unknown
	}
	if control.Mode == "on" && (len(profile.Budgets) > 0 || !evidence.validated) {
		unknown.Reason = "未确认上游校验档位，未执行该档位请求"
		return unknown
	}
	target, body, err := buildThinkingProbeRequest(ch, name, profile, control)
	if err != nil {
		return unknown
	}
	response, err := p.sendThinkingProbe(ctx, ch, target, body)
	if err != nil {
		unknown.Reason = "请求未完成，无法确认该选项"
		return unknown
	}
	if response.rejects(evidence.parameter) || (control.Mode == "off" && response.rejects("thinking")) {
		return ThinkingProbeCheck{Status: "unsupported", Reason: "上游明确拒绝该思考参数"}
	}
	valid, tokens, thought := response.observation()
	if !valid {
		return unknown
	}
	if control.Mode == "off" {
		if thought || (tokens != nil && *tokens > 0) {
			return ThinkingProbeCheck{Status: "unsupported", Reason: "关闭后上游仍返回思考内容或思考用量"}
		}
		if !evidence.baseline || tokens == nil || *tokens != 0 {
			return unknown
		}
	}
	return ThinkingProbeCheck{Status: "supported", Reason: "上游校验思考参数，并接受该选项；不保证不同档位产生不同思考用量"}
}

func (result *ThinkingProbeResult) collectConfirmed(transport model.ThinkingProfile) {
	facts := &model.ThinkingConfig{}
	if check := result.Checks["off"]; check.Status != "unknown" && check.Status != "" {
		value := check.Status == "supported"
		facts.CanDisable = &value
	}
	for _, effort := range transport.Efforts {
		if check := result.Checks[effort]; check.Status != "unknown" && check.Status != "" && facts.Efforts == nil {
			facts.Efforts = []string{}
		}
		if result.Checks[effort].Status == "supported" {
			facts.Efforts = append(facts.Efforts, effort)
		}
	}
	if facts.CanDisable == nil && facts.Efforts == nil {
		return
	}
	result.Thinking = facts
	confirmed := result.Capability
	if confirmed.Profile == "" {
		confirmed.Profile, confirmed.Output = transport.ID, transport.Output
	}
	if facts.CanDisable != nil {
		confirmed.Off = *facts.CanDisable
	}
	confirmed.Efforts = slices.DeleteFunc(slices.Clone(confirmed.Efforts), func(effort string) bool { return result.Checks[effort].Status == "unsupported" })
	for _, effort := range facts.Efforts {
		if !slices.Contains(confirmed.Efforts, effort) {
			confirmed.Efforts = append(confirmed.Efforts, effort)
		}
	}
	confirmed.On = confirmed.On || len(confirmed.Efforts) > 0
	confirmed.Known = confirmed.Known || confirmed.On || confirmed.Off
	confirmed.Source = "probe"
	result.Capability = confirmed
}
