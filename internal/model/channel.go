package model

import "strings"

// ProviderType represents the upstream provider category.
type ProviderType string

const (
	ProviderOpenAI    ProviderType = "openai"
	ProviderAnthropic ProviderType = "anthropic"
	ProviderGemini    ProviderType = "gemini"
	ProviderSub2API   ProviderType = "sub2api"
	ProviderGPUStack  ProviderType = "gpustack"
	ProviderCustom    ProviderType = "custom"
	ProviderDeepSeek  ProviderType = "deepseek"
	ProviderVLLM      ProviderType = "vllm"
	ProviderSGLang    ProviderType = "sglang"
	ProviderOllama    ProviderType = "ollama"
)

// ChannelConfig defines an upstream endpoint configuration.
type ChannelConfig struct {
	// RouteRules is an immutable runtime snapshot loaded separately from channels.
	RouteRules     map[string]ModelRouteRule `yaml:"-" json:"-"`
	ID             int64                     `yaml:"id,omitempty" json:"id,omitempty"`
	Name           string                    `yaml:"name" json:"name"`
	Type           ProviderType              `yaml:"type" json:"type"`
	BaseURL        string                    `yaml:"base_url" json:"base_url"`
	APIKey         string                    `yaml:"api_key" json:"api_key"`
	Models         []string                  `yaml:"models" json:"models"`                                   // supported models in this channel
	ModelMapping   map[string]string         `yaml:"model_mapping,omitempty" json:"model_mapping,omitempty"` // incoming model -> actual upstream model
	Protocols      []string                  `yaml:"protocols,omitempty" json:"protocols,omitempty"`         // supported protocols: openai_chat, openai_text, anthropic_messages
	Priority       int                       `yaml:"priority" json:"priority"`                               // lower number means higher priority (e.g. 1 is primary, 2 is fallback)
	Weight         int                       `yaml:"weight" json:"weight"`                                   // weight for load balancing among same priority
	TimeoutSeconds int                       `yaml:"timeout_seconds" json:"timeout_seconds"`
	Status         string                    `yaml:"status,omitempty" json:"status,omitempty"`
}

// SupportsProtocol checks whether this channel supports the requested inbound/outbound protocol.
// If Protocols is empty, all protocols are supported by default.
func (c *ChannelConfig) SupportsProtocol(proto string) bool {
	if len(c.Protocols) == 0 {
		return true
	}
	for _, p := range c.Protocols {
		if p == "*" || p == proto {
			return true
		}
		if (p == "chat" || p == "openai_chat") && (proto == "chat" || proto == "openai_chat") {
			return true
		}
		if (p == "response" || p == "responses" || p == "openai_response" || p == "openai_responses") && (proto == "response" || proto == "responses" || proto == "openai_response" || proto == "openai_responses") {
			return true
		}
		// Upstream chat providers can seamlessly adapt to openai_response
		if (p == "chat" || p == "openai_chat") && (proto == "response" || proto == "responses" || proto == "openai_response" || proto == "openai_responses") {
			return true
		}
		if (p == "completion" || p == "openai_text") && (proto == "completion" || proto == "openai_text") {
			return true
		}
		if (p == "messages" || p == "anthropic_messages") && (proto == "messages" || proto == "anthropic_messages") {
			return true
		}
		if (p == "images" || p == "image_generation") && (proto == "images" || proto == "image_generation") {
			return true
		}
		if (p == "audio" || p == "audio_speech" || p == "tts") && (proto == "audio" || proto == "audio_speech" || proto == "tts") {
			return true
		}
		if (p == "audio" || p == "audio_transcription" || p == "stt") && (proto == "audio" || proto == "audio_transcription" || proto == "stt") {
			return true
		}
		if (p == "videos" || p == "video_generation") && (proto == "videos" || proto == "video_generation") {
			return true
		}
		if (p == "embeddings" || p == "embedding" || p == "openai_embeddings") && (proto == "embeddings" || proto == "embedding" || proto == "openai_embeddings") {
			return true
		}
		if (p == "rerank" || p == "reranker" || p == "cohere_rerank") && (proto == "rerank" || proto == "reranker" || proto == "cohere_rerank") {
			return true
		}
		if (p == "moderation" || p == "moderations") && (proto == "moderation" || proto == "moderations") {
			return true
		}
	}
	return false
}

// OnlySupportsProtocol checks if the channel only supports a single protocol (accounting for standard aliases).
func (c *ChannelConfig) OnlySupportsProtocol(proto string) bool {
	if len(c.Protocols) == 0 {
		return false
	}
	for _, p := range c.Protocols {
		match := false
		if p == proto {
			match = true
		} else if (p == "openai_text" || p == "completion") && (proto == "openai_text" || proto == "completion") {
			match = true
		} else if (p == "openai_chat" || p == "chat") && (proto == "openai_chat" || proto == "chat") {
			match = true
		}
		if !match {
			return false
		}
	}
	return true
}

// HasExplicitProtocol checks if the channel explicitly lists the given protocol in its Protocols slice.
func (c *ChannelConfig) HasExplicitProtocol(proto string) bool {
	for _, p := range c.Protocols {
		if p == "*" || p == proto {
			return true
		}
		if (p == "messages" || p == "anthropic_messages") && (proto == "messages" || proto == "anthropic_messages") {
			return true
		}
		if (p == "chat" || p == "openai_chat") && (proto == "chat" || proto == "openai_chat") {
			return true
		}
	}
	return false
}

// SupportsModel checks if this channel can service the requested model (supporting cascading models, wildcards, and prefix stripping).
func (c *ChannelConfig) SupportsModel(requestedModel string) bool {
	if _, allowed, overridden := c.routeProvider(requestedModel); overridden {
		return allowed
	}
	// 1. Exact match or wildcard in Models list
	for _, m := range c.Models {
		if m == "*" || m == requestedModel {
			return true
		}
		if strings.HasSuffix(m, "/*") {
			prefix := strings.TrimSuffix(m, "/*") + "/"
			if strings.HasPrefix(requestedModel, prefix) {
				return true
			}
		}
	}

	// 2. Exact match or wildcard in ModelMapping
	if c.ModelMapping != nil {
		if _, ok := c.ModelMapping[requestedModel]; ok {
			return true
		}
		for pattern := range c.ModelMapping {
			if strings.HasSuffix(pattern, "/*") {
				prefix := strings.TrimSuffix(pattern, "/*") + "/"
				if strings.HasPrefix(requestedModel, prefix) {
					return true
				}
			}
		}
	}

	// 3. Automatic channel name prefix match: e.g. channel "yy" servicing "yy/xxx/xx"
	chPrefix := c.Name + "/"
	if strings.HasPrefix(requestedModel, chPrefix) {
		remainder := strings.TrimPrefix(requestedModel, chPrefix)
		if c.SupportsModel(remainder) {
			return true
		}
	}

	return false
}

// GetUpstreamModel returns the mapped upstream model name if specified, otherwise the requested model.
// Supports cascading models, unlimited slashes, wildcard prefixes (e.g. "yy/*": "*"), and channel prefix stripping.
const (
	// DefaultChannelTimeoutSeconds is the per-request upstream budget applied when
	// an operator does not set one.
	DefaultChannelTimeoutSeconds = 60
	// MinChannelTimeoutSeconds rejects a budget so small that no request can finish.
	MinChannelTimeoutSeconds = 1
	// EdgeProxyTimeoutSeconds is the deployment's own read timeout (nginx
	// proxy_read_timeout). Operators never touch it: it is a safety net, not a
	// budget, and it is deliberately far above anything an operator would configure
	// so that the channel timeout is always the value that decides when a request
	// stops. Setting it to a day means a channel timeout of an hour, or several
	// hours, simply works instead of being rejected by an artificial ceiling.
	//
	// It is the single source for the limit below, and a test asserts that the
	// shipped nginx config matches it exactly, so the two cannot drift apart.
	EdgeProxyTimeoutSeconds = 86400
	// edgeProxyMarginSeconds keeps every accepted channel timeout strictly inside
	// the proxy budget: at the exact boundary a race could let the proxy cut first.
	edgeProxyMarginSeconds = 60
	// MaxChannelTimeoutSeconds is derived, not chosen: the largest channel timeout
	// that still fits inside the proxy budget with margin.
	MaxChannelTimeoutSeconds = EdgeProxyTimeoutSeconds - edgeProxyMarginSeconds
)

func (c *ChannelConfig) GetUpstreamModel(requestedModel string) string {
	if p, allowed, _ := c.routeProvider(requestedModel); allowed {
		if p.MappedModel != "" {
			return p.MappedModel
		}
		return requestedModel
	}
	if c.ModelMapping != nil {
		// 1. Exact match
		if actual, ok := c.ModelMapping[requestedModel]; ok && actual != "" {
			return actual
		}

		// 2. Pattern / Wildcard prefix match: e.g. "yy/*": "*" or "yy/*": "downstream/*"
		for pattern, targetPattern := range c.ModelMapping {
			if strings.HasSuffix(pattern, "/*") {
				prefix := strings.TrimSuffix(pattern, "/*") + "/"
				if strings.HasPrefix(requestedModel, prefix) {
					remainder := strings.TrimPrefix(requestedModel, prefix)
					if targetPattern == "*" || targetPattern == "" {
						return remainder
					}
					if strings.HasSuffix(targetPattern, "/*") {
						targetPrefix := strings.TrimSuffix(targetPattern, "/*") + "/"
						return targetPrefix + remainder
					}
					return targetPattern
				}
			}
		}
	}

	// 3. Automatic channel prefix match: "yy/xxx/xx" -> "xxx/xx"
	chPrefix := c.Name + "/"
	if strings.HasPrefix(requestedModel, chPrefix) {
		remainder := strings.TrimPrefix(requestedModel, chPrefix)
		return c.GetUpstreamModel(remainder)
	}

	return requestedModel
}

// APIKeyConfig defines an API key configured on the gateway.
type APIKeyConfig struct {
	Key              string   `yaml:"key" json:"key"` // e.g. "sk-airoute-8f92a1c4b7e3"
	TenantID         string   `yaml:"tenant_id" json:"tenant_id"`
	AllowedModels    []string `yaml:"allowed_models" json:"allowed_models"` // empty means all allowed
	RPM              int      `yaml:"rpm" json:"rpm"`                       // Requests per minute limit (0 = unlimited)
	TPM              int      `yaml:"tpm" json:"tpm"`                       // Tokens per minute limit (0 = unlimited)
	Budget           float64  `yaml:"budget" json:"budget"`                 // total dollar or token budget
	UsedCost         float64  `yaml:"used_cost" json:"used_cost"`
	BudgetUnits      int64    `yaml:"budget_units,omitempty" json:"budget_units,omitempty"`       // integer 1e-9-unit mirror
	UsedCostUnits    int64    `yaml:"used_cost_units,omitempty" json:"used_cost_units,omitempty"` // integer 1e-9-unit mirror
	Status           string   `yaml:"status" json:"status"`
	GroupName        string   `yaml:"group_name" json:"group_name"`                                   // pricing group e.g. default, vip
	UserID           int64    `yaml:"user_id" json:"user_id"`                                         // owner user id
	OrgID            int64    `yaml:"org_id,omitempty" json:"org_id,omitempty"`                       // attribution organization
	FormatValidation string   `yaml:"format_validation,omitempty" json:"format_validation,omitempty"` // "off" | "lenient" | "strict"
	UserStatus       string   `yaml:"user_status,omitempty" json:"user_status,omitempty"`
	UserRole         string   `yaml:"user_role,omitempty" json:"user_role,omitempty"`
	UserBalance      float64  `yaml:"user_balance,omitempty" json:"user_balance,omitempty"`
}
