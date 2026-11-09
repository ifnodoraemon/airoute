package provider

import (
	"slices"

	"github.com/ifnodoraemon/airoute/internal/model"
)

// PrepareThinkingRequest applies console intent independently for each route
// candidate. Legacy SDK requests without thinking_control remain untouched.
func PrepareThinkingRequest(req *model.ChatCompletionRequest, channel *model.ChannelConfig) (*model.ChatCompletionRequest, error) {
	policy := channel.ThinkingPolicyFor(req.Model)
	if !policy.Active() {
		policy = nil
	}
	control, active, err := resolveCandidateThinkingControl(req, policy)
	if err != nil {
		return nil, err
	}
	if !active {
		return req, nil
	}
	profile, source := model.ResolveThinkingProfile(channel, req.Model)
	if !model.ThinkingProfileCompatible(model.ChatProviderType(channel, req), profile) {
		return nil, &UnsupportedTranslationError{Reason: "thinking profile is incompatible with channel protocol"}
	}
	control, err = restrictThinkingEffort(req, control, profile, policy)
	if err != nil {
		return nil, err
	}
	if err := validateThinkingCandidate(req, control, profile); err != nil {
		return nil, err
	}
	copy := copyThinkingRequest(req, channel, control, profile, source)
	if err := applyThinkingWire(copy, channel); err != nil {
		return nil, err
	}
	if req.Native != nil && req.Native.Protocol != model.NativeChat {
		if err := prepareNativeThinking(copy, req.Native); err != nil {
			return nil, err
		}
	}
	return copy, nil
}

func resolveCandidateThinkingControl(req *model.ChatCompletionRequest, policy *model.ThinkingPolicy) (model.ThinkingControl, bool, error) {
	raw := thinkingControlRaw(req)
	control := model.ThinkingControl{Mode: "auto"}
	if req.ThinkingIntent != nil {
		control = *req.ThinkingIntent
	} else if len(raw) > 0 {
		var err error
		control, err = DecodeThinkingControl(raw)
		if err != nil {
			return control, false, err
		}
	}
	if len(raw) == 0 && req.ThinkingIntent == nil && policy == nil {
		return control, false, nil
	}
	if len(raw) == 0 && req.ThinkingIntent == nil && hasNativeThinking(req) {
		if policy != nil && (policy.AllowedModes != nil || policy.AllowedEfforts != nil) {
			return control, false, &UnsupportedTranslationError{Reason: "native thinking parameters cannot bypass route restrictions; use thinking_control"}
		}
		return control, false, nil
	}
	if len(raw) > 0 && hasNativeThinking(req) {
		return control, false, &UnsupportedTranslationError{Reason: "do not combine thinking_control with provider-native thinking parameters"}
	}
	var err error
	control, err = policy.Resolve(control)
	if err != nil {
		return control, false, &UnsupportedTranslationError{Reason: err.Error()}
	}
	return control, true, nil
}

func restrictThinkingEffort(req *model.ChatCompletionRequest, control model.ThinkingControl, profile model.ThinkingProfile, policy *model.ThinkingPolicy) (model.ThinkingControl, error) {
	allowedEffort := func(effort string) bool {
		for _, restriction := range []*model.ThinkingPolicy{req.ThinkingIntentPolicy, policy} {
			if restriction != nil && restriction.AllowedEfforts != nil && !slices.Contains(restriction.AllowedEfforts, effort) {
				return false
			}
		}
		return true
	}
	if control.Mode == "on" {
		if control.Effort != "" && !allowedEffort(control.Effort) {
			return control, &UnsupportedTranslationError{Reason: "thinking effort is forbidden by the original route policy"}
		}
		restricted := (policy != nil && policy.AllowedEfforts != nil) || (req.ThinkingIntentPolicy != nil && req.ThinkingIntentPolicy.AllowedEfforts != nil)
		if control.Effort == "" && restricted {
			if allowedEffort(profile.DefaultEffort) {
				control.Effort = profile.DefaultEffort
			} else {
				for _, effort := range profile.Efforts {
					if allowedEffort(effort) {
						control.Effort = effort
						break
					}
				}
			}
			if control.Effort == "" {
				return control, &UnsupportedTranslationError{Reason: "no upstream thinking effort satisfies route restrictions"}
			}
		}
	}
	return control, nil
}

func validateThinkingCandidate(req *model.ChatCompletionRequest, control model.ThinkingControl, profile model.ThinkingProfile) error {
	if control.IncludeSummary != nil && *control.IncludeSummary && profile.Output != "summary" && profile.Output != "reasoning" && !(req.Native != nil && req.Native.Protocol == model.NativeResponses && profile.Wire == "openai") {
		return &UnsupportedTranslationError{Reason: "upstream cannot return thinking content or a summary for this call"}
	}
	if err := profile.Validate(control); err != nil {
		return &UnsupportedTranslationError{Reason: err.Error()}
	}
	if profile.ToolsRequireResponses && control.Mode != "off" && len(req.Tools) > 0 && (req.Native == nil || req.Native.Protocol != model.NativeResponses) {
		return &UnsupportedTranslationError{Reason: "this model requires the Responses API for thinking with tools"}
	}
	return nil
}

func copyThinkingRequest(req *model.ChatCompletionRequest, channel *model.ChannelConfig, control model.ThinkingControl, profile model.ThinkingProfile, source string) *model.ChatCompletionRequest {
	copy := *req
	copy.Native = nil
	if req.Native != nil && req.Native.Protocol != model.NativeChat {
		copy.Native = req.Native
	}
	copy.Extra = make(model.ExtraFields, len(req.Extra))
	for k, v := range req.Extra {
		if k != "thinking_control" {
			copy.Extra[k] = v
		}
	}
	copy.PreparedThinking = &model.PreparedThinking{Control: control, Profile: profile, Source: source, UpstreamModel: channel.GetUpstreamModel(req.Model)}
	return &copy
}
