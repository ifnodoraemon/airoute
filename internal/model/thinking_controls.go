package model

import (
	"regexp"
	"slices"
	"strings"
)

// ThinkingEfforts is the complete gateway vocabulary. Disabling is a separate
// capability; neither "none" nor automatic selection is an effort level.
func ThinkingEfforts() []string {
	return []string{"low", "medium", "high", "xhigh", "max", "ultra"}
}

var thinkingEffortPattern = regexp.MustCompile(`^[a-z][a-z0-9_-]{0,31}$`)

// Named passthrough levels can evolve without changing a gateway allowlist.
func ValidThinkingEffort(effort string) bool {
	return thinkingEffortPattern.MatchString(effort) && !slices.Contains([]string{"none", "auto", "on", "off", "adaptive", "minimal"}, effort)
}

func passthroughThinkingEffort(profile ThinkingProfile) bool {
	return slices.Contains([]string{"openai", "deepseek", "glm", "claude-adaptive", "gemini-level", "gemini-summary", "ollama-level"}, profile.Wire) || (profile.Wire == "deepseek-template" && len(profile.Budgets) == 0)
}

// ThinkingParameterProfile chooses an encodable request dialect, without
// declaring that a model supports its controls. Facts and probes decide that.
func ThinkingParameterProfile(kind ProviderType, name string) ThinkingProfile {
	profile, _ := catalogThinkingProfile(kind, name)
	if profile.ID != "" && profile.Wire != "none" && profile.Wire != "unknown" && ThinkingProfileCompatible(kind, profile) {
		return profile
	}
	id := "openai-toggle"
	switch {
	case IsGeminiProvider(kind):
		id = "gemini-levels"
	case kind == ProviderAnthropic:
		id = "claude-adaptive"
	case kind == ProviderDeepSeek || strings.Contains(strings.ToLower(name), "deepseek"):
		id = "deepseek"
	}
	profile, _ = FindThinkingProfile(id)
	return profile
}

// ThinkingTransportProfile describes encodable controls, independently of the
// catalog's model-specific defaults. It is for configuration and probing only;
// routing always uses ResolveThinkingProfile and the confirmed channel facts.
func ThinkingTransportProfile(profile ThinkingProfile) ThinkingProfile {
	profile.Efforts = slices.Clone(profile.Efforts)
	if profile.Wire != "" && profile.Wire != "none" && profile.Wire != "unknown" {
		profile.On, profile.Off = true, true
	}
	switch profile.Wire {
	case "openai", "deepseek", "glm", "claude-adaptive", "gemini-level", "gemini-summary", "ollama-level":
		profile.On, profile.Off, profile.Efforts = true, true, ThinkingEfforts()
	case "deepseek-template":
		if len(profile.Budgets) == 0 {
			profile.Efforts = ThinkingEfforts()
		}
	}
	return profile
}
