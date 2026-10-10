package router

import (
	"encoding/json"
	"math"
	"sort"
	"strings"

	"github.com/ifnodoraemon/airoute/internal/model"
)

// ModelRouteDetail encapsulates complete routing and multi-provider topology for a single model.
type ModelRouteDetail struct {
	Model                  string               `json:"model"`
	Modality               string               `json:"modality"`
	Providers              []*ModelProviderInfo `json:"providers"`
	PrimaryProvidersCount  int                  `json:"primary_providers_count"`
	FallbackProvidersCount int                  `json:"fallback_providers_count"`
	TotalPrimaryWeight     int                  `json:"total_primary_weight"`
	FallbackModel          string               `json:"fallback_model,omitempty"`
	HasCrossProtocol       bool                 `json:"has_cross_protocol"`
	HasFallbackTier        bool                 `json:"has_fallback_tier"`
}

// ModelProviderInfo details an individual downstream provider serving a given model.
type ModelProviderInfo struct {
	ChannelID        int64    `json:"channel_id"`
	ChannelName      string   `json:"channel_name"`
	ChannelType      string   `json:"channel_type"`
	BaseURL          string   `json:"base_url"`
	Priority         int      `json:"priority"`
	Weight           int      `json:"weight"`
	WeightPercent    float64  `json:"weight_percent"`
	MappedModel      string   `json:"mapped_model"`
	Protocols        []string `json:"protocols"`
	BreakerStatus    string   `json:"breaker_status"`
	Status           string   `json:"status"`
	ConversionNeeded bool     `json:"conversion_needed"`
	ConversionType   string   `json:"conversion_type"`
}

// GetModelRoutes aggregates channels and model fallback configurations into a model-centric view.
func (d *Dispatcher) GetModelRoutes() []*ModelRouteDetail {
	d.mu.RLock()
	channels := make([]model.ChannelConfig, len(d.channels))
	copy(channels, d.channels)
	d.mu.RUnlock()

	d.fallbacksMu.RLock()
	fallbacks := make(map[string]string)
	for k, v := range d.modelFallbacks {
		fallbacks[k] = v
	}
	d.fallbacksMu.RUnlock()

	allModels := d.GetAllSupportedModels()

	var routes []*ModelRouteDetail
	for _, m := range allModels {
		detail := &ModelRouteDetail{
			Model:         m,
			Modality:      InferModality(m),
			FallbackModel: fallbacks[m],
			Providers:     make([]*ModelProviderInfo, 0),
		}

		var matched []*model.ChannelConfig
		for i := range channels {
			if channels[i].SupportsModel(m) {
				matched = append(matched, &channels[i])
			}
		}

		totalP1Weight := 0
		hasCrossProto := false
		hasFallbackTier := false

		for _, ch := range matched {
			mapped := ch.GetUpstreamModel(m)
			breaker := d.GetBreakerStatus(ch.Name)

			convNeeded := false
			convType := "原生直连透传 (Native Passthrough)"

			if ch.Type == model.ProviderAnthropic {
				convNeeded = true
				convType = "OpenAI ⇄ Claude Messages 全双工实时转译"
				hasCrossProto = true
			} else if ch.Type == model.ProviderGemini {
				convNeeded = true
				convType = "OpenAI ⇄ Gemini 原生协议转译"
				hasCrossProto = true
			} else if strings.Contains(strings.Join(ch.Protocols, " "), "openai_response") {
				convType = "OpenAI Chat ⇄ Responses 双向转译支持"
			}

			if ch.Priority > 1 {
				hasFallbackTier = true
			} else if ch.Status == "active" {
				totalP1Weight += ch.Weight
			}

			info := &ModelProviderInfo{
				ChannelID:        ch.ID,
				ChannelName:      ch.Name,
				ChannelType:      string(ch.Type),
				BaseURL:          ch.BaseURL,
				Priority:         ch.Priority,
				Weight:           ch.Weight,
				MappedModel:      mapped,
				Protocols:        ch.Protocols,
				BreakerStatus:    breaker,
				Status:           ch.Status,
				ConversionNeeded: convNeeded,
				ConversionType:   convType,
			}
			detail.Providers = append(detail.Providers, info)
		}

		// Calculate weight percentage for primary tier
		for _, info := range detail.Providers {
			if info.Priority == 1 && totalP1Weight > 0 {
				pct := (float64(info.Weight) / float64(totalP1Weight)) * 100
				info.WeightPercent = math.Round(pct*10) / 10
				detail.PrimaryProvidersCount++
			} else {
				detail.FallbackProvidersCount++
			}
		}

		// Sort providers: Priority ASC, then Weight DESC
		sort.Slice(detail.Providers, func(i, j int) bool {
			if detail.Providers[i].Priority != detail.Providers[j].Priority {
				return detail.Providers[i].Priority < detail.Providers[j].Priority
			}
			return detail.Providers[i].Weight > detail.Providers[j].Weight
		})

		detail.TotalPrimaryWeight = totalP1Weight
		detail.HasCrossProtocol = hasCrossProto
		detail.HasFallbackTier = hasFallbackTier

		routes = append(routes, detail)
	}

	return routes
}

// InferModality deduces the primary modality of a model from its name.
func InferModality(modelName string) string {
	lower := strings.ToLower(modelName)
	if strings.Contains(lower, "dall-e") || strings.Contains(lower, "flux") || strings.Contains(lower, "stable-diffusion") || strings.Contains(lower, "image") || strings.Contains(lower, "seedream") || strings.Contains(lower, "midjourney") {
		return "images"
	}
	if strings.Contains(lower, "tts") || strings.Contains(lower, "speech") || strings.Contains(lower, "cosyvoice") || strings.Contains(lower, "chattts") {
		return "audio_speech"
	}
	if strings.Contains(lower, "whisper") || strings.Contains(lower, "transcription") || strings.Contains(lower, "sensevoice") || strings.Contains(lower, "funasr") {
		return "audio_transcription"
	}
	if strings.Contains(lower, "sora") || strings.Contains(lower, "cogvideo") || strings.Contains(lower, "kling") || strings.Contains(lower, "video") || strings.Contains(lower, "seedance") || strings.Contains(lower, "luma") || strings.Contains(lower, "runway") || strings.Contains(lower, "pika") || strings.Contains(lower, "wan") || strings.Contains(lower, "hunyuan") || strings.Contains(lower, "vidu") || strings.Contains(lower, "minimax-video") {
		return "videos"
	}
	if strings.Contains(lower, "embedding") || strings.Contains(lower, "bge-m3") || strings.Contains(lower, "text-embedding") {
		return "embeddings"
	}
	if strings.Contains(lower, "rerank") {
		return "rerank"
	}
	return "chat"
}

// RewriteJSONModel substitutes the "model" field in a JSON payload with newModel, preserving all other keys.
func RewriteJSONModel(body []byte, newModel string) []byte {
	var m map[string]any
	if err := json.Unmarshal(body, &m); err != nil {
		return body
	}
	if _, ok := m["model"]; !ok {
		return body
	}
	m["model"] = newModel
	out, err := json.Marshal(m)
	if err != nil {
		return body
	}
	return out
}
