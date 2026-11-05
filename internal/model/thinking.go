package model

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"regexp"
	"slices"
	"strings"
)

// ThinkingProfile is a documented wire contract. Catalog matches are inferred,
// never evidence that a particular proxy implements the upstream contract.
type ThinkingProfile struct {
	ID                    string         `json:"id"`
	Label                 string         `json:"label"`
	Wire                  string         `json:"wire"`
	On                    bool           `json:"on"`
	Off                   bool           `json:"off"`
	Efforts               []string       `json:"efforts"`
	DefaultEffort         string         `json:"default_effort,omitempty"`
	Budgets               map[string]int `json:"budgets,omitempty"`
	Output                string         `json:"output"`
	SourceURL             string         `json:"source_url,omitempty"`
	ToolsRequireResponses bool           `json:"tools_require_responses,omitempty"`
}

// ModelCapabilityConfig is keyed by the actual upstream model on a channel.
// Aliases and route providers never own a second copy of these facts.
type ModelCapabilityConfig struct {
	Thinking *ThinkingConfig `json:"thinking,omitempty" yaml:"thinking,omitempty"`
}
type ThinkingConfig struct {
	Profile    string   `json:"profile,omitempty" yaml:"profile,omitempty"`
	CanDisable *bool    `json:"can_disable,omitempty" yaml:"can_disable,omitempty"`
	Efforts    []string `json:"efforts" yaml:"efforts,omitempty"`
}

type ThinkingCapability struct {
	SupportsDisable  *bool               `json:"supports_disable,omitempty"`
	SupportedEfforts []string            `json:"supported_efforts,omitempty"`
	Automatic        *ThinkingCapability `json:"automatic,omitempty"`
	Known            bool                `json:"known"`
	On               bool                `json:"on"`
	Off              bool                `json:"off"`
	Efforts          []string            `json:"efforts"`
	Output           string              `json:"output"`
	Source           string              `json:"source"`
	Profile          string              `json:"profile,omitempty"`
}

// ThinkingPolicy sets route defaults and restrictions, independently of facts.
// nil slices mean unrestricted; an explicit empty slice permits no values.
type ThinkingPolicy struct {
	DefaultMode    string   `json:"default_mode,omitempty"`
	DefaultEffort  string   `json:"default_effort,omitempty"`
	AllowedModes   []string `json:"allowed_modes"`
	AllowedEfforts []string `json:"allowed_efforts"`
}
type ThinkingControl struct {
	Mode           string `json:"mode"`
	Effort         string `json:"effort,omitempty"`
	IncludeSummary *bool  `json:"include_summary,omitempty"`
}
type PreparedThinking struct {
	Control       ThinkingControl
	Profile       ThinkingProfile
	Source        string
	UpstreamModel string
}

//go:embed thinking_profiles.json
var thinkingCatalogJSON []byte

type thinkingRule struct {
	Pattern, Provider, Profile string
	compiled                   *regexp.Regexp
}

type thinkingProfileCatalog struct {
	Profiles []ThinkingProfile `json:"profiles"`
	Rules    []thinkingRule    `json:"rules"`
}

var thinkingCatalog = func() thinkingProfileCatalog {
	var catalog thinkingProfileCatalog
	if err := json.Unmarshal(thinkingCatalogJSON, &catalog); err != nil {
		panic(err)
	}
	profiles := make(map[string]bool, len(catalog.Profiles))
	for _, profile := range catalog.Profiles {
		if profile.ID == "" || profiles[profile.ID] {
			panic("duplicate or empty thinking profile")
		}
		profiles[profile.ID] = true
	}
	for i := range catalog.Rules {
		if !profiles[catalog.Rules[i].Profile] {
			panic("thinking rule references unknown profile")
		}
		catalog.Rules[i].compiled = regexp.MustCompile(catalog.Rules[i].Pattern)
	}
	return catalog
}()

func ThinkingProfiles() []ThinkingProfile { return slices.Clone(thinkingCatalog.Profiles) }
func FindThinkingProfile(id string) (ThinkingProfile, bool) {
	for _, p := range thinkingCatalog.Profiles {
		if p.ID == id {
			return p, true
		}
	}
	return ThinkingProfile{}, false
}
func ValidateModelCapabilities(config map[string]ModelCapabilityConfig) error {
	for name, value := range config {
		if strings.TrimSpace(name) == "" {
			return fmt.Errorf("upstream model name cannot be empty")
		}
		if value.Thinking == nil {
			continue
		}
		profile, ok := FindThinkingProfile(value.Thinking.Profile)
		if value.Thinking.Profile != "" && !ok {
			return fmt.Errorf("model %q: unknown thinking profile %q", name, value.Thinking.Profile)
		}
		for _, effort := range value.Thinking.Efforts {
			if !slices.Contains([]string{"minimal", "low", "medium", "high", "xhigh", "max"}, effort) || (ok && !slices.Contains(profile.Efforts, effort)) {
				return fmt.Errorf("model %q: profile does not support effort %q", name, effort)
			}
		}
	}
	return nil
}
func (p *ThinkingPolicy) Validate() error {
	if p == nil {
		return nil
	}
	mode := p.DefaultMode
	if mode == "" {
		mode = "auto"
	}
	if !slices.Contains([]string{"auto", "on", "off"}, mode) {
		return fmt.Errorf("invalid default thinking mode %q", mode)
	}
	for _, v := range p.AllowedModes {
		if !slices.Contains([]string{"auto", "on", "off"}, v) {
			return fmt.Errorf("invalid allowed thinking mode %q", v)
		}
	}
	if p.AllowedModes != nil && !slices.Contains(p.AllowedModes, mode) {
		return fmt.Errorf("default thinking mode must be allowed by route policy")
	}
	if p.AllowedEfforts != nil && mode == "auto" {
		return fmt.Errorf("set a route default thinking mode when restricting effort levels")
	}
	if p.DefaultEffort != "" && mode != "on" {
		return fmt.Errorf("default effort requires default mode on")
	}
	if p.DefaultEffort != "" && p.AllowedEfforts != nil && !slices.Contains(p.AllowedEfforts, p.DefaultEffort) {
		return fmt.Errorf("default thinking effort is not allowed by route policy")
	}
	for _, effort := range p.AllowedEfforts {
		if !slices.Contains([]string{"minimal", "low", "medium", "high", "xhigh", "max"}, effort) {
			return fmt.Errorf("invalid thinking effort %q", effort)
		}
	}
	return nil
}
func (p *ThinkingPolicy) Resolve(control ThinkingControl) (ThinkingControl, error) {
	if control.Mode == "" {
		control.Mode = "auto"
	}
	if p == nil {
		return control, nil
	}
	if control.Mode == "auto" && p.DefaultMode != "" {
		control.Mode = p.DefaultMode
	}
	if control.Mode == "on" && control.Effort == "" {
		control.Effort = p.DefaultEffort
	}
	if p.AllowedModes != nil && !slices.Contains(p.AllowedModes, control.Mode) {
		return control, fmt.Errorf("thinking mode %q is forbidden by route policy", control.Mode)
	}
	if control.Effort != "" && p.AllowedEfforts != nil && !slices.Contains(p.AllowedEfforts, control.Effort) {
		return control, fmt.Errorf("thinking effort %q is forbidden by route policy", control.Effort)
	}
	return control, nil
}
func (c *ChannelConfig) ThinkingPolicyFor(name string) *ThinkingPolicy {
	rule, _, ok := c.routeRule(name)
	if _, exact := c.RouteRules[name]; !exact && strings.HasPrefix(name, c.Name+"/") {
		if baseRule, _, found := c.routeRule(strings.TrimPrefix(name, c.Name+"/")); found {
			rule, ok = baseRule, true
		}
	}
	if ok {
		return rule.ThinkingPolicy
	}
	return nil
}
func ResolveThinkingProfile(channel *ChannelConfig, publicModel string) (ThinkingProfile, string) {
	if channel == nil {
		return ThinkingProfile{}, "unknown"
	}
	return ResolveUpstreamThinkingProfile(channel, channel.GetUpstreamModel(publicModel))
}

// ResolveUpstreamThinkingProfile resolves an already mapped physical model.
// Admin catalogs use this to avoid applying a public alias mapping twice.
func ResolveUpstreamThinkingProfile(channel *ChannelConfig, name string) (ThinkingProfile, string) {
	profile, source := ResolveUpstreamThinkingContract(channel, name)
	if source == "invalid_configuration" {
		return profile, source
	}
	if config := upstreamThinkingConfig(channel, name); config != nil {
		return applyThinkingFacts(profile, config, source)
	}
	return profile, source
}

func upstreamThinkingConfig(channel *ChannelConfig, name string) *ThinkingConfig {
	if channel == nil {
		return nil
	}
	config, configured := channel.ModelCapabilities[name]
	if !configured {
		config = channel.ModelCapabilities["*"]
	}
	return config.Thinking
}

// ResolveUpstreamThinkingContract supplies the backend's available controls
// before operator narrowing. The console does not select wire adapters.
func ResolveUpstreamThinkingContract(channel *ChannelConfig, name string) (ThinkingProfile, string) {
	if channel == nil {
		return ThinkingProfile{}, "unknown"
	}
	if config := upstreamThinkingConfig(channel, name); config != nil && config.Profile != "" {
		profile, ok := FindThinkingProfile(config.Profile)
		if !ok {
			return ThinkingProfile{}, "invalid_configuration"
		}
		return profile, "configured"
	}
	return catalogThinkingProfile(channel.Type, name)
}

func catalogThinkingProfile(kind ProviderType, name string) (ThinkingProfile, string) {
	if strings.Contains(strings.ToLower(name), "gpt-") && strings.Contains(strings.ToLower(name), "-pro") {
		return ThinkingProfile{}, "unknown"
	}
	for _, rule := range thinkingCatalog.Rules {
		if rule.Provider != "" && !strings.Contains("|"+rule.Provider+"|", "|"+string(kind)+"|") {
			continue
		}
		if rule.compiled.MatchString(strings.ToLower(name)) {
			profile, _ := FindThinkingProfile(rule.Profile)
			return profile, "catalog"
		}
	}
	return ThinkingProfile{}, "unknown"
}
func (p ThinkingProfile) Capability(source string) ThinkingCapability {
	return ThinkingCapability{Known: p.ID != "" && p.ID != "unknown", On: p.On, Off: p.Off, Efforts: slices.Clone(p.Efforts), Output: p.Output, Source: source, Profile: p.ID}
}
func (p ThinkingProfile) Validate(control ThinkingControl) error {
	if !slices.Contains([]string{"", "auto", "on", "off"}, control.Mode) {
		return fmt.Errorf("thinking mode must be auto, on or off")
	}
	if control.Mode != "on" && control.Effort != "" {
		return fmt.Errorf("thinking effort requires mode on")
	}
	if control.Mode == "on" && len(p.Budgets) > 0 && control.Effort == "" && p.DefaultEffort == "" {
		return fmt.Errorf("thinking budget default is missing from channel configuration")
	}
	if control.Mode == "on" && !p.On {
		return fmt.Errorf("thinking cannot be enabled: upstream capability is unsupported or unknown")
	}
	if control.Mode == "off" && !p.Off {
		return fmt.Errorf("upstream does not support disabling thinking, or its capability is unknown")
	}
	if control.Effort != "" && !slices.Contains(p.Efforts, control.Effort) {
		return fmt.Errorf("upstream does not support thinking effort %q", control.Effort)
	}
	return nil
}

// ThinkingProfileCompatible checks the channel transport, independently of model
// naming. OpenAI-compatible relays may explicitly select Google's REST dialect.
func ThinkingProfileCompatible(kind ProviderType, profile ThinkingProfile) bool {
	if profile.Wire == "none" || profile.ID == "" || profile.ID == "unknown" {
		return true
	}
	if IsGeminiProvider(kind) {
		return strings.HasPrefix(profile.Wire, "gemini-") || profile.Wire == "gemma"
	}
	if kind == ProviderAnthropic {
		return strings.HasPrefix(profile.Wire, "claude-")
	}
	return !strings.HasPrefix(profile.Wire, "claude-")
}
func ValidateChannelThinking(kind ProviderType, config map[string]ModelCapabilityConfig) error {
	if err := ValidateModelCapabilities(config); err != nil {
		return err
	}
	for name, value := range config {
		if value.Thinking != nil {
			profile, _ := catalogThinkingProfile(kind, name)
			if value.Thinking.Profile != "" {
				profile, _ = FindThinkingProfile(value.Thinking.Profile)
			}
			if !ThinkingProfileCompatible(kind, profile) {
				return fmt.Errorf("model %q: thinking profile %q is incompatible with channel protocol %s", name, profile.ID, kind)
			}
			if err := validateThinkingFacts(profile, value.Thinking); err != nil {
				return fmt.Errorf("model %q: %w", name, err)
			}
		}
	}
	return nil
}

func (p *ThinkingPolicy) Active() bool {
	return p != nil && ((p.DefaultMode != "" && p.DefaultMode != "auto") || p.DefaultEffort != "" || p.AllowedModes != nil || p.AllowedEfforts != nil)
}
