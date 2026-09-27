package router

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"hash/fnv"
	"io"
	"math"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/ifnodoraemon/nano-gateway/internal/distributed"
	"github.com/ifnodoraemon/nano-gateway/internal/middleware"
	"github.com/ifnodoraemon/nano-gateway/internal/model"
	"github.com/ifnodoraemon/nano-gateway/internal/provider"
	"github.com/ifnodoraemon/nano-gateway/internal/telemetry"
)

// Dispatcher routes requests to appropriate providers with intelligent fallback.
type Dispatcher struct {
	mu             sync.RWMutex
	channels       []model.ChannelConfig
	providers      map[model.ProviderType]provider.Provider
	wrrMu          sync.Mutex
	wrrWeights     map[string]int // model:channel_name -> current_weight
	circuitBreaker *CircuitBreaker
	modelFallbacks map[string]string
	fallbacksMu    sync.RWMutex
}

// NewDispatcher creates a new Dispatcher instance.
func NewDispatcher(channels []model.ChannelConfig) *Dispatcher {
	d := &Dispatcher{
		channels:       channels,
		providers:      make(map[model.ProviderType]provider.Provider),
		wrrWeights:     make(map[string]int),
		circuitBreaker: NewCircuitBreaker(3, 30*time.Second),
		modelFallbacks: make(map[string]string),
	}

	// Register default providers
	openAIProv := provider.NewOpenAIProvider(nil)
	d.providers[model.ProviderOpenAI] = openAIProv
	d.providers[model.ProviderDeepSeek] = openAIProv
	d.providers[model.ProviderVLLM] = openAIProv
	d.providers[model.ProviderSGLang] = openAIProv
	d.providers[model.ProviderOllama] = openAIProv
	d.providers[model.ProviderSub2API] = openAIProv
	d.providers[model.ProviderGPUStack] = openAIProv
	d.providers[model.ProviderCustom] = openAIProv

	anthropicProv := provider.NewAnthropicProvider(nil)
	d.providers[model.ProviderAnthropic] = anthropicProv

	geminiProv := provider.NewGeminiProvider(nil)
	d.providers[model.ProviderGemini] = geminiProv

	return d
}

// RegisterProvider registers a custom provider.
func (d *Dispatcher) RegisterProvider(p provider.Provider) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.providers[p.Type()] = p
}

// UpdateChannels updates channel configurations dynamically.
func (d *Dispatcher) UpdateChannels(channels []model.ChannelConfig) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.channels = channels
}

// SetModelFallbacks updates the cross-model fallback mapping.
func (d *Dispatcher) SetModelFallbacks(fallbacks map[string]string) {
	d.fallbacksMu.Lock()
	defer d.fallbacksMu.Unlock()
	d.modelFallbacks = fallbacks
}

// GetModelFallback returns the fallback model for a requested model if configured.
func (d *Dispatcher) GetModelFallback(modelName string) string {
	d.fallbacksMu.RLock()
	defer d.fallbacksMu.RUnlock()
	if d.modelFallbacks == nil {
		return ""
	}
	return d.modelFallbacks[modelName]
}

// GetAllModelFallbacks returns a copy of the model fallbacks map.
func (d *Dispatcher) GetAllModelFallbacks() map[string]string {
	d.fallbacksMu.RLock()
	defer d.fallbacksMu.RUnlock()
	res := make(map[string]string)
	for k, v := range d.modelFallbacks {
		res[k] = v
	}
	return res
}

// ContextKeySession is the context key type for sticky session IDs.
type ContextKeySession string

const ContextKeySessionID ContextKeySession = "nano_session_id"

// GetChannelsForModel returns matching channels for a given model, sorted by priority.
func (d *Dispatcher) GetChannelsForModel(modelName string) []*model.ChannelConfig {
	return d.GetChannelsForModelAndProtocolWithContext(context.Background(), modelName, "")
}

// GetChannelsForModelAndProtocol delegates to GetChannelsForModelAndProtocolWithContext.
func (d *Dispatcher) GetChannelsForModelAndProtocol(modelName, proto string) []*model.ChannelConfig {
	return d.GetChannelsForModelAndProtocolWithContext(context.Background(), modelName, proto)
}

// GetChannelsForModelAndProtocolWithContext returns matching channels for a given model and protocol.
// If a session ID is present in ctx, it enables LLM Session Affinity (consistent hashing and Redis pin)
// to reuse upstream KV Cache (Prefix Caching) across multi-turn conversations.
func (d *Dispatcher) GetChannelsForModelAndProtocolWithContext(ctx context.Context, modelName, proto string) []*model.ChannelConfig {
	d.mu.RLock()
	var matched []*model.ChannelConfig
	for i := range d.channels {
		ch := &d.channels[i]
		if proto != "" && !ch.SupportsProtocol(proto) {
			continue
		}
		if ch.SupportsModel(modelName) {
			matched = append(matched, ch)
		}
	}
	d.mu.RUnlock()

	if len(matched) <= 1 {
		return matched
	}

	// Extract Session ID for Session Affinity
	var sessionID string
	if ctx != nil {
		if v, ok := ctx.Value(ContextKeySessionID).(string); ok && v != "" {
			sessionID = v
		} else if v, ok := ctx.Value("session_id").(string); ok && v != "" {
			sessionID = v
		}
	}

	// Group channels by Priority
	priorityGroups := make(map[int][]*model.ChannelConfig)
	var priorities []int
	for _, ch := range matched {
		p := ch.Priority
		if len(priorityGroups[p]) == 0 {
			priorities = append(priorities, p)
		}
		priorityGroups[p] = append(priorityGroups[p], ch)
	}
	sort.Ints(priorities)

	var result []*model.ChannelConfig
	d.wrrMu.Lock()
	defer d.wrrMu.Unlock()

	for groupIdx, p := range priorities {
		group := priorityGroups[p]
		if len(group) == 1 {
			result = append(result, group[0])
			continue
		}

		// 1. Session Affinity for Primary Pool (highest priority tier)
		if groupIdx == 0 && sessionID != "" {
			var chosen *model.ChannelConfig
			chosenIdx := -1

			// Check Distributed Redis cache pin first
			if client := distributed.GetClient(); client != nil && client.IsActive() {
				if pinnedName, err := client.GetSessionChannel(ctx, modelName, sessionID); err == nil && pinnedName != "" {
					for i, ch := range group {
						if ch.Name == pinnedName {
							chosen = ch
							chosenIdx = i
							break
						}
					}
				}
			}

			// If not in Redis, compute Consistent Hash via FNV-1a
			if chosen == nil {
				h := fnv.New32a()
				_, _ = h.Write([]byte(sessionID))
				_, _ = h.Write([]byte(":"))
				_, _ = h.Write([]byte(modelName))
				chosenIdx = int(h.Sum32() % uint32(len(group)))
				chosen = group[chosenIdx]

				// Persist pin in Redis (30-minute TTL)
				if client := distributed.GetClient(); client != nil && client.IsActive() {
					go func(m, s, chName string) {
						_ = client.SetSessionChannel(context.Background(), m, s, chName, 30*time.Minute)
					}(modelName, sessionID, chosen.Name)
				}
			}

			telemetry.Logger.Debug("routed via LLM session affinity",
				"session_id", sessionID,
				"model", modelName,
				"channel", chosen.Name,
			)

			result = append(result, chosen)
			for i, ch := range group {
				if i != chosenIdx {
					result = append(result, ch)
				}
			}
			continue
		}

		// 2. Smooth Weighted Round-Robin (SWRR)
		totalWeight := 0
		maxWeight := -1 << 31
		winnerIdx := 0

		for i, ch := range group {
			w := ch.Weight
			if w <= 0 {
				w = 1
			}
			totalWeight += w

			key := modelName + ":" + ch.Name
			cur := d.wrrWeights[key] + w
			d.wrrWeights[key] = cur
			if cur > maxWeight {
				maxWeight = cur
				winnerIdx = i
			}
		}

		winnerKey := modelName + ":" + group[winnerIdx].Name
		d.wrrWeights[winnerKey] -= totalWeight

		// Winner is tried first
		result = append(result, group[winnerIdx])

		// Remaining channels in this tier serve as immediate fallbacks
		for i, ch := range group {
			if i != winnerIdx {
				result = append(result, ch)
			}
		}
	}

	return result
}

// GetAllSupportedModels returns a deduplicated list of all configured models.
func (d *Dispatcher) GetAllSupportedModels() []string {
	d.mu.RLock()
	defer d.mu.RUnlock()

	modelSet := make(map[string]struct{})
	for _, ch := range d.channels {
		for _, m := range ch.Models {
			modelSet[m] = struct{}{}
		}
		for alias := range ch.ModelMapping {
			modelSet[alias] = struct{}{}
		}
	}

	var list []string
	for m := range modelSet {
		list = append(list, m)
	}
	sort.Strings(list)
	return list
}

// Dispatch executes non-streaming chat with automatic fallback.
func (d *Dispatcher) Dispatch(ctx context.Context, req *model.ChatCompletionRequest) (*model.ChatCompletionResponse, error) {
	channels := d.GetChannelsForModelAndProtocolWithContext(ctx, req.Model, "chat")
	if len(channels) == 0 {
		channels = d.GetChannelsForModelAndProtocolWithContext(ctx, req.Model, "")
	}
	if len(channels) == 0 {
		if fb := d.GetModelFallback(req.Model); fb != "" && fb != req.Model {
			telemetry.Logger.Warn("no upstream provider for model, triggering cross-model fallback",
				"requested_model", req.Model,
				"fallback_model", fb,
			)
			reqCopy := *req
			reqCopy.Model = fb
			return d.Dispatch(ctx, &reqCopy)
		}
		return nil, fmt.Errorf("no upstream provider available for requested model '%s'", req.Model)
	}

	var lastErr error
	for i, ch := range channels {
		// High-Availability Circuit Breaker: fail fast if upstream is down
		if !d.circuitBreaker.CanExecute(ch.Name) {
			telemetry.Logger.Warn("circuit breaker is OPEN, bypassing dead upstream", "channel", ch.Name)
			continue
		}

		d.mu.RLock()
		prov, exists := d.providers[ch.Type]
		d.mu.RUnlock()

		if !exists {
			lastErr = fmt.Errorf("unsupported provider type '%s' on channel %s", ch.Type, ch.Name)
			telemetry.Logger.Warn("provider type not found", "channel", ch.Name, "type", ch.Type)
			continue
		}

		telemetry.Logger.Info("attempting chat completion",
			"channel", ch.Name,
			"model", req.Model,
			"attempt", i+1,
			"total_channels", len(channels),
		)

		start := time.Now()
		resp, err := prov.ChatComplete(ctx, req, ch)
		if err == nil {
			d.circuitBreaker.RecordSuccess(ch.Name)
			dur := time.Since(start)
			promptTokens := 0
			compTokens := 0
			if resp.Usage != nil {
				promptTokens = resp.Usage.PromptTokens
				compTokens = resp.Usage.CompletionTokens
			}
			telemetry.GlobalMetrics.RecordRequest(true, dur, promptTokens, compTokens)
			telemetry.Logger.Info("chat completion succeeded",
				"channel", ch.Name,
				"duration_ms", dur.Milliseconds(),
				"total_tokens", promptTokens+compTokens,
			)
			return resp, nil
		}

		d.circuitBreaker.RecordFailure(ch.Name)
		lastErr = err
		telemetry.GlobalMetrics.RecordFallback()
		telemetry.Logger.Warn("channel execution failed, triggering fallback",
			"channel", ch.Name,
			"error", err.Error(),
			"fallback_index", i+1,
		)
	}

	if fb := d.GetModelFallback(req.Model); fb != "" && fb != req.Model {
		telemetry.Logger.Warn("all candidate channels failed, triggering cross-model fallback",
			"requested_model", req.Model,
			"fallback_model", fb,
			"last_error", lastErr,
		)
		reqCopy := *req
		reqCopy.Model = fb
		return d.Dispatch(ctx, &reqCopy)
	}

	telemetry.GlobalMetrics.RecordRequest(false, 0, 0, 0)
	return nil, fmt.Errorf("all %d candidate channels failed for model %s. Last error: %w", len(channels), req.Model, lastErr)
}

// DispatchStream executes streaming chat with Safe Fallback Window before the first token.
func (d *Dispatcher) DispatchStream(ctx context.Context, req *model.ChatCompletionRequest) (<-chan *model.StreamEvent, error) {
	channels := d.GetChannelsForModelAndProtocolWithContext(ctx, req.Model, "chat")
	if len(channels) == 0 {
		channels = d.GetChannelsForModelAndProtocolWithContext(ctx, req.Model, "")
	}
	if len(channels) == 0 {
		if fb := d.GetModelFallback(req.Model); fb != "" && fb != req.Model {
			telemetry.Logger.Warn("no upstream provider for stream model, triggering cross-model fallback",
				"requested_model", req.Model,
				"fallback_model", fb,
			)
			reqCopy := *req
			reqCopy.Model = fb
			return d.DispatchStream(ctx, &reqCopy)
		}
		return nil, fmt.Errorf("no upstream provider available for requested model '%s'", req.Model)
	}

	var lastErr error
	for i, ch := range channels {
		// High-Availability Circuit Breaker: fail fast if upstream is down
		if !d.circuitBreaker.CanExecute(ch.Name) {
			telemetry.Logger.Warn("circuit breaker is OPEN, bypassing dead upstream stream", "channel", ch.Name)
			continue
		}

		d.mu.RLock()
		prov, exists := d.providers[ch.Type]
		d.mu.RUnlock()

		if !exists {
			lastErr = fmt.Errorf("unsupported provider type '%s' on channel %s", ch.Type, ch.Name)
			continue
		}

		telemetry.Logger.Info("attempting streaming connection",
			"channel", ch.Name,
			"model", req.Model,
			"attempt", i+1,
		)

		streamChan, err := prov.ChatCompleteStream(ctx, req, ch)
		if err != nil {
			d.circuitBreaker.RecordFailure(ch.Name)
			lastErr = err
			telemetry.GlobalMetrics.RecordFallback()
			telemetry.Logger.Warn("stream connection failed, trying next channel",
				"channel", ch.Name,
				"error", err.Error(),
			)
			continue
		}

		// Safe Fallback Window: wait for the first event to confirm healthy stream
		select {
		case firstEvent, ok := <-streamChan:
			if !ok {
				d.circuitBreaker.RecordFailure(ch.Name)
				lastErr = fmt.Errorf("channel %s closed stream without events", ch.Name)
				telemetry.GlobalMetrics.RecordFallback()
				continue
			}

			if firstEvent.Err != nil {
				d.circuitBreaker.RecordFailure(ch.Name)
				lastErr = firstEvent.Err
				telemetry.GlobalMetrics.RecordFallback()
				telemetry.Logger.Warn("channel failed before first valid token, falling back",
					"channel", ch.Name,
					"error", firstEvent.Err.Error(),
				)
				continue
			}

			// First token healthy! Mark healthy in circuit breaker
			d.circuitBreaker.RecordSuccess(ch.Name)

			// Wrap and return combined stream
			outChan := make(chan *model.StreamEvent, 64)
			go func(first *model.StreamEvent, in <-chan *model.StreamEvent) {
				defer close(outChan)
				outChan <- first
				for event := range in {
					outChan <- event
				}
			}(firstEvent, streamChan)

			return outChan, nil

		case <-time.After(15 * time.Second):
			d.circuitBreaker.RecordFailure(ch.Name)
			lastErr = fmt.Errorf("channel %s timed out waiting for first token", ch.Name)
			telemetry.GlobalMetrics.RecordFallback()
			telemetry.Logger.Warn("first token timeout, falling back", "channel", ch.Name)
			continue

		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}

	if fb := d.GetModelFallback(req.Model); fb != "" && fb != req.Model {
		telemetry.Logger.Warn("all stream channels failed, triggering cross-model fallback",
			"requested_model", req.Model,
			"fallback_model", fb,
			"last_error", lastErr,
		)
		reqCopy := *req
		reqCopy.Model = fb
		return d.DispatchStream(ctx, &reqCopy)
	}

	return nil, fmt.Errorf("all channels failed for stream request on model %s. Last error: %w", req.Model, lastErr)
}

// GetBreakerStatus returns the circuit breaker status of a channel.
func (d *Dispatcher) GetBreakerStatus(name string) string {
	if d.circuitBreaker == nil {
		return "CLOSED"
	}
	return d.circuitBreaker.GetStatus(name).String()
}

// ModelRouteDetail provides a comprehensive model-centric view of all downstream providers,
// proportional traffic distribution, fallback policy, and protocol translation matrix.
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

// =============================================================================
// Unified Multimodal Forwarding Pipeline (Images, Audio, Video, Generic HTTP)
// =============================================================================

// UpstreamRequest represents a generic HTTP request across any modality.
type UpstreamRequest struct {
	Path        string            // e.g. "/v1/images/generations", "/v1/audio/speech"
	Method      string            // "POST", "GET"
	Headers     map[string]string // custom headers
	Body        []byte            // serialized payload
	ContentType string            // "application/json", "multipart/form-data"
	Model       string            // requested model for routing
	Protocol    string            // e.g. "images", "audio_speech", "videos"
}

// UpstreamResponse encapsulates a generic HTTP response across any modality.
type UpstreamResponse struct {
	StatusCode int
	Headers    http.Header
	Body       []byte
	Stream     io.ReadCloser
}

// RewriteJSONModel cleanly updates the "model" field in a JSON payload.
func RewriteJSONModel(body []byte, newModel string) []byte {
	if len(body) == 0 {
		return body
	}
	var rawMap map[string]any
	if err := json.Unmarshal(body, &rawMap); err != nil {
		return body
	}
	rawMap["model"] = newModel
	rewritten, err := json.Marshal(rawMap)
	if err != nil {
		return body
	}
	return rewritten
}

// DispatchHTTP routes an HTTP request across matching candidate channels with Circuit Breaking and Safe Fallback.
func (d *Dispatcher) DispatchHTTP(ctx context.Context, req *UpstreamRequest) (*UpstreamResponse, error) {
	channels := d.GetChannelsForModelAndProtocolWithContext(ctx, req.Model, req.Protocol)
	if len(channels) == 0 {
		channels = d.GetChannelsForModelAndProtocolWithContext(ctx, req.Model, "")
	}
	if len(channels) == 0 {
		return nil, fmt.Errorf("no upstream provider available for model '%s' and protocol '%s'", req.Model, req.Protocol)
	}

	var lastErr error
	for i, ch := range channels {
		// Circuit Breaker: fail fast if upstream is down
		if !d.circuitBreaker.CanExecute(ch.Name) {
			telemetry.Logger.Warn("circuit breaker is OPEN, bypassing dead upstream", "channel", ch.Name)
			continue
		}

		targetModel := ch.GetUpstreamModel(req.Model)
		targetBody := req.Body
		if strings.Contains(req.ContentType, "application/json") && len(targetBody) > 0 {
			targetBody = RewriteJSONModel(targetBody, targetModel)
		}

		targetURL := strings.TrimRight(ch.BaseURL, "/") + req.Path
		httpReq, err := http.NewRequestWithContext(ctx, req.Method, targetURL, bytes.NewReader(targetBody))
		if err != nil {
			lastErr = err
			continue
		}

		if req.ContentType != "" {
			httpReq.Header.Set("Content-Type", req.ContentType)
		}
		if ch.APIKey != "" && ch.APIKey != "none" {
			httpReq.Header.Set("Authorization", "Bearer "+ch.APIKey)
			httpReq.Header.Set("x-api-key", ch.APIKey)
		}
		for k, v := range req.Headers {
			httpReq.Header.Set(k, v)
		}
		if tid := middleware.GetTraceIDFromContext(ctx); tid != "" {
			httpReq.Header.Set(middleware.HeaderNanoTraceID, tid)
			httpReq.Header.Set(middleware.HeaderRequestID, tid)
			httpReq.Header.Set(middleware.HeaderTraceID, tid)
		}

		start := time.Now()
		httpResp, err := provider.SharedDefaultHTTPClient.Do(httpReq)
		if err != nil {
			d.circuitBreaker.RecordFailure(ch.Name)
			lastErr = err
			telemetry.GlobalMetrics.RecordFallback()
			telemetry.Logger.Warn("upstream HTTP request failed, falling back",
				"channel", ch.Name,
				"error", err.Error(),
				"attempt", i+1,
			)
			continue
		}

		// Check if response indicates server error or rate limit
		if httpResp.StatusCode == http.StatusTooManyRequests || httpResp.StatusCode >= http.StatusInternalServerError {
			d.circuitBreaker.RecordFailure(ch.Name)
			bodyBytes, _ := io.ReadAll(httpResp.Body)
			httpResp.Body.Close()
			lastErr = fmt.Errorf("upstream %s returned status %d: %s", ch.Name, httpResp.StatusCode, string(bodyBytes))
			telemetry.GlobalMetrics.RecordFallback()
			telemetry.Logger.Warn("upstream returned server error, falling back",
				"channel", ch.Name,
				"status", httpResp.StatusCode,
				"attempt", i+1,
			)
			continue
		}

		// Success! Mark circuit breaker healthy
		d.circuitBreaker.RecordSuccess(ch.Name)
		dur := time.Since(start)
		telemetry.GlobalMetrics.RecordRequest(true, dur, 0, 0)

		return &UpstreamResponse{
			StatusCode: httpResp.StatusCode,
			Headers:    httpResp.Header,
			Stream:     httpResp.Body,
		}, nil
	}

	telemetry.GlobalMetrics.RecordRequest(false, 0, 0, 0)
	return nil, fmt.Errorf("all %d providers failed for model '%s'. Last error: %w", len(channels), req.Model, lastErr)
}

// DispatchEmbedding routes an embedding request across matching candidate channels with Circuit Breaking and Safe Fallback.
func (d *Dispatcher) DispatchEmbedding(ctx context.Context, req *model.EmbeddingRequest) (*model.EmbeddingResponse, error) {
	channels := d.GetChannelsForModelAndProtocolWithContext(ctx, req.Model, "embeddings")
	if len(channels) == 0 {
		channels = d.GetChannelsForModelAndProtocolWithContext(ctx, req.Model, "")
	}
	if len(channels) == 0 {
		return nil, fmt.Errorf("no upstream provider available for embedding model '%s'", req.Model)
	}

	var lastErr error
	for i, ch := range channels {
		if !d.circuitBreaker.CanExecute(ch.Name) {
			telemetry.Logger.Warn("circuit breaker is OPEN, bypassing dead upstream", "channel", ch.Name)
			continue
		}

		start := time.Now()
		var resp *model.EmbeddingResponse
		var err error

		if ch.Type == model.ProviderGemini {
			geminiProv, ok := d.providers[model.ProviderGemini].(*provider.GeminiProvider)
			if !ok {
				geminiProv = provider.NewGeminiProvider(nil)
			}
			resp, err = geminiProv.Embed(ctx, req, ch)
		} else {
			openAIProv, ok := d.providers[model.ProviderOpenAI].(*provider.OpenAIProvider)
			if !ok {
				openAIProv = provider.NewOpenAIProvider(nil)
			}
			resp, err = openAIProv.Embed(ctx, req, ch)
		}

		if err != nil {
			d.circuitBreaker.RecordFailure(ch.Name)
			lastErr = err
			telemetry.GlobalMetrics.RecordFallback()
			telemetry.Logger.Warn("upstream embedding request failed, falling back",
				"channel", ch.Name,
				"error", err.Error(),
				"attempt", i+1,
			)
			continue
		}

		d.circuitBreaker.RecordSuccess(ch.Name)
		dur := time.Since(start)
		telemetry.GlobalMetrics.RecordRequest(true, dur, resp.Usage.PromptTokens, 0)
		return resp, nil
	}

	telemetry.GlobalMetrics.RecordRequest(false, 0, 0, 0)
	return nil, fmt.Errorf("all %d providers failed for embedding model '%s'. Last error: %w", len(channels), req.Model, lastErr)
}

// DispatchRerank routes a cross-encoder rerank request across candidate channels with Circuit Breaking and Safe Fallback.
func (d *Dispatcher) DispatchRerank(ctx context.Context, req *model.RerankRequest) (*model.RerankResponse, error) {
	channels := d.GetChannelsForModelAndProtocolWithContext(ctx, req.Model, "rerank")
	if len(channels) == 0 {
		channels = d.GetChannelsForModelAndProtocolWithContext(ctx, req.Model, "")
	}
	if len(channels) == 0 {
		return nil, fmt.Errorf("no upstream provider available for rerank model '%s'", req.Model)
	}

	var lastErr error
	for i, ch := range channels {
		if !d.circuitBreaker.CanExecute(ch.Name) {
			telemetry.Logger.Warn("circuit breaker is OPEN, bypassing dead upstream", "channel", ch.Name)
			continue
		}

		start := time.Now()
		openAIProv, ok := d.providers[model.ProviderOpenAI].(*provider.OpenAIProvider)
		if !ok {
			openAIProv = provider.NewOpenAIProvider(nil)
		}

		resp, err := openAIProv.Rerank(ctx, req, ch)
		if err != nil {
			d.circuitBreaker.RecordFailure(ch.Name)
			lastErr = err
			telemetry.GlobalMetrics.RecordFallback()
			telemetry.Logger.Warn("upstream rerank request failed, falling back",
				"channel", ch.Name,
				"error", err.Error(),
				"attempt", i+1,
			)
			continue
		}

		d.circuitBreaker.RecordSuccess(ch.Name)
		dur := time.Since(start)
		telemetry.GlobalMetrics.RecordRequest(true, dur, resp.Usage.TotalTokens, 0)
		return resp, nil
	}

	telemetry.GlobalMetrics.RecordRequest(false, 0, 0, 0)
	return nil, fmt.Errorf("all %d providers failed for rerank model '%s'. Last error: %w", len(channels), req.Model, lastErr)
}



