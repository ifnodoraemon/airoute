package router

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/ifnodoraemon/airoute/internal/model"
	"github.com/ifnodoraemon/airoute/internal/provider"
	"github.com/ifnodoraemon/airoute/internal/telemetry"
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

// Dispatch executes non-streaming chat with automatic fallback.
func (d *Dispatcher) Dispatch(ctx context.Context, req *model.ChatCompletionRequest) (*model.ChatCompletionResponse, error) {
	proto := "chat"
	if req.Protocol != "" {
		proto = req.Protocol
	}
	channels := d.ResolveCandidateChannels(ctx, req.Model, proto)
	if len(channels) == 0 {
		if fb := d.GetModelFallback(req.Model); fb != "" && !strings.EqualFold(fb, req.Model) {
			nextCtx, fbErr := nextFallbackContext(ctx, req.Model, fb)
			if fbErr != nil {
				telemetry.Logger.Warn("aborting fallback to prevent cycle or depth exhaustion", "error", fbErr.Error())
				return nil, fmt.Errorf("no upstream provider available for requested model '%s': %w", req.Model, fbErr)
			}
			telemetry.Logger.Warn("no upstream provider for model, triggering cross-model fallback",
				"requested_model", req.Model,
				"fallback_model", fb,
			)
			reqCopy := *req
			reqCopy.Model = fb
			return d.Dispatch(nextCtx, &reqCopy)
		}
		return nil, fmt.Errorf("no upstream provider available for requested model '%s'", req.Model)
	}

	var lastErr error
	var attempted []string // channels actually called and failed for this model
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
			promptTokens := 0
			compTokens := 0
			if resp.Usage != nil {
				promptTokens = resp.Usage.PromptTokens
				compTokens = resp.Usage.CompletionTokens
			}
			resp.Channel = d.recordChannelSuccess(ctx, attempted, ch.Name, time.Since(start), promptTokens, compTokens)
			return resp, nil
		}

		lastErr = err
		attempted = append(attempted, d.recordChannelFailure(ch.Name, err, i+1))
	}

	if fb := d.GetModelFallback(req.Model); fb != "" && !strings.EqualFold(fb, req.Model) {
		nextCtx, fbErr := nextFallbackContext(ctx, req.Model, fb)
		if fbErr != nil {
			telemetry.Logger.Warn("aborting fallback to prevent cycle or depth exhaustion", "error", fbErr.Error())
			return nil, fmt.Errorf("all %d candidate channels failed for model %s (last error: %v); cannot fallback to %s: %w", len(channels), req.Model, lastErr, fb, fbErr)
		}
		telemetry.Logger.Warn("all candidate channels failed, triggering cross-model fallback",
			"requested_model", req.Model,
			"fallback_model", fb,
			"last_error", lastErr,
		)
		reqCopy := *req
		reqCopy.Model = fb
		return d.Dispatch(withChannelTrail(nextCtx, attempted), &reqCopy)
	}

	telemetry.GlobalMetrics.RecordRequest(false, 0, 0, 0)
	return nil, fmt.Errorf("all %d candidate channels failed for model %s. Last error: %w", len(channels), req.Model, lastErr)
}

// GetBreakerStatus returns the circuit breaker status of a channel.
func (d *Dispatcher) GetBreakerStatus(name string) string {
	if d.circuitBreaker == nil {
		return "CLOSED"
	}
	return d.circuitBreaker.GetStatus(name).String()
}
