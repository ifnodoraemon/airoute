package router

import (
	"context"
	"errors"
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
		circuitBreaker: newConfiguredCircuitBreaker(),
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

// swrrKeySeparator separates the model name from the channel name in the SWRR
// weight map. It must not be ":" because both model and channel names may
// contain colons, which made the old strings.Split(k, ":") prune check silently
// skip those entries and leak them forever.
const swrrKeySeparator = "\x00"

// swrrKey builds the weight-map key for a (model, channel) pair.
func swrrKey(modelName, channelName string) string {
	return modelName + swrrKeySeparator + channelName
}

// UpdateChannels updates channel configurations dynamically and prunes obsolete weights.
func (d *Dispatcher) UpdateChannels(channels []model.ChannelConfig) {
	d.mu.Lock()
	d.channels = channels
	d.mu.Unlock()

	activeNames := make(map[string]bool, len(channels))
	for _, ch := range channels {
		activeNames[ch.Name] = true
	}

	d.wrrMu.Lock()
	for k := range d.wrrWeights {
		idx := strings.LastIndex(k, swrrKeySeparator)
		if idx < 0 {
			// Unrecognized legacy key shape: drop it rather than leak.
			delete(d.wrrWeights, k)
			continue
		}
		if !activeNames[k[idx+len(swrrKeySeparator):]] {
			delete(d.wrrWeights, k)
		}
	}
	d.wrrMu.Unlock()
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

// AnthropicNativeProtocol is the explicit channel-protocol marker for an upstream
// that speaks the Anthropic Messages wire format even though the channel's
// ProviderType is not "anthropic" (for example an OpenAI-compatible proxy in
// front of Claude). Operators add it to channel.protocols.
//
// The ordinary "anthropic_messages" token is NOT such a marker: it is the inbound
// capability ("serves /v1/messages traffic"). A channel that declares only that
// token must still talk to its upstream in its own protocol (openai/gemini), with
// the gateway translating the request; otherwise an OpenAI endpoint receives
// Anthropic JSON at <base>/v1/messages.
const AnthropicNativeProtocol = "anthropic_native"

// isAnthropicWireChannel reports whether this channel's upstream speaks the
// Anthropic Messages protocol natively. Only the channel type and the explicit
// native marker decide that, never the inbound protocol capability.
func isAnthropicWireChannel(ch *model.ChannelConfig) bool {
	if ch == nil {
		return false
	}
	if ch.Type == model.ProviderAnthropic {
		return true
	}
	for _, proto := range ch.Protocols {
		if proto == AnthropicNativeProtocol {
			return true
		}
	}
	return false
}

// getProviderForChannel selects the Provider implementation that talks to the
// channel's upstream. The native Anthropic provider is used only for channels
// whose upstream actually speaks Anthropic; OpenAI/Gemini channels translate.
func (d *Dispatcher) getProviderForChannel(ch *model.ChannelConfig) (provider.Provider, bool) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	if isAnthropicWireChannel(ch) {
		if prov, ok := d.providers[model.ProviderAnthropic]; ok {
			return prov, true
		}
	}

	prov, exists := d.providers[ch.Type]
	return prov, exists
}

// channelCallContext bounds an upstream call by the channel's configured
// timeout_seconds. It is the only upstream budget an operator configures, so it
// works for streaming, non-streaming and modality calls alike: the shared HTTP
// client no longer imposes a transport-level header deadline (it used to cap
// every call at 60s, far below the accepted maximum), and the first-token window
// is derived from the same number. When a channel sets no timeout the documented
// default applies, so no provider call is left without a deadline.
func channelCallContext(ctx context.Context, ch *model.ChannelConfig) (context.Context, context.CancelFunc) {
	if ctx == nil {
		ctx = context.Background()
	}
	timeout := time.Duration(model.DefaultChannelTimeoutSeconds) * time.Second
	if ch != nil && ch.TimeoutSeconds > 0 {
		timeout = time.Duration(ch.TimeoutSeconds) * time.Second
	}
	return context.WithTimeout(ctx, timeout)
}

// isFatalClientPayloadError checks if the error is a deterministic client request syntax/payload
// error (400, 422, 413) that would fail identically on every channel, rather than a node-specific 404.
func isFatalClientPayloadError(err error) bool {
	if ue, ok := provider.AsUpstreamError(err); ok {
		return ue.Status == 400 || ue.Status == 422 || ue.Status == 413
	}
	return false
}

// Dispatch executes non-streaming chat with automatic fallback.
func (d *Dispatcher) Dispatch(ctx context.Context, req *model.ChatCompletionRequest) (*model.ChatCompletionResponse, error) {
	proto := "chat"
	if req.Protocol != "" {
		proto = req.Protocol
	}
	channels := d.candidateChannelsForRequest(ctx, req, proto)
	if len(channels) == 0 {
		if fb := d.GetModelFallback(req.Model); fb != "" && !strings.EqualFold(fb, req.Model) {
			nextCtx, fbErr := nextFallbackContext(ctx, req.Model, fb)
			if fbErr != nil {
				routerLog(ctx).Warn("aborting fallback to prevent cycle or depth exhaustion", "error", fbErr.Error())
				return nil, newNoRouteError(ctx, req.Model, fmt.Errorf("no upstream provider available for requested model '%s': %w", req.Model, fbErr))
			}
			routerLog(ctx).Warn("no upstream provider for model, triggering cross-model fallback",
				"requested_model", req.Model,
				"fallback_model", fb,
			)
			reqCopy := *req
			reqCopy.Model = fb
			return d.Dispatch(nextCtx, &reqCopy)
		}
		return nil, newNoRouteError(ctx, req.Model, fmt.Errorf("no upstream provider available for requested model '%s'", req.Model))
	}

	var lastErr error
	var attempted []string // channels actually called and failed for this model
	var skipped []string   // channels bypassed by the circuit breaker, never called
	for i, ch := range channels {
		// High-Availability Circuit Breaker: fail fast if upstream is down
		if !d.circuitBreaker.CanExecute(ch.Name) {
			telemetry.WarnLogger(routerLog(ctx), "breaker-open:"+ch.Name, 30*time.Second,
				"circuit breaker is OPEN, bypassing dead upstream",
				"channel", ch.Name, "attempt", i+1)
			skipped = append(skipped, ch.Name)
			continue
		}

		prov, exists := d.getProviderForChannel(ch)

		if !exists {
			lastErr = fmt.Errorf("unsupported provider type '%s' on channel %s", ch.Type, ch.Name)
			routerLog(ctx).Warn("provider type not found", "channel", ch.Name, "type", ch.Type)
			// CanExecute may have taken a half-open canary slot; hand it back or
			// the channel stays locked out until the 2x-cooldown reset.
			d.circuitBreaker.AbortCanary(ch.Name)
			continue
		}

		routerLog(ctx).Debug("attempting chat completion",
			"channel", ch.Name,
			"model", req.Model,
			"attempt", i+1,
			"total_channels", len(channels),
		)

		start := time.Now()
		callCtx, cancelCall := channelCallContext(ctx, ch)
		resp, err := prov.ChatComplete(callCtx, req, ch)
		cancelCall()
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

		if ctx.Err() == context.Canceled || errors.Is(err, context.Canceled) {
			d.circuitBreaker.AbortCanary(ch.Name)
			return nil, newCanceledError(ctx, attempted, ch.Name, req.EstimatedPromptTokens())
		}

		lastErr = err
		// A deterministic payload error means the caller's request is at fault (400, 422, 413):
		// the same request would fail on every channel, so do not trip the breaker and do not fan out.
		// Note: 404 from a single channel means this node does not host the model; we fall through to next channels.
		if isFatalClientPayloadError(err) {
			routerLog(ctx).Warn("upstream rejected request with a deterministic client error, aborting fallback",
				"channel", ch.Name,
				"error", err.Error(),
			)
			d.circuitBreaker.AbortCanary(ch.Name)
			return nil, err
		}
		if !provider.IsChannelFailure(err) {
			// Unknown but non-fatal classification: try the next channel without
			// charging the breaker.
			routerLog(ctx).Warn("channel returned a non-health error, trying next channel without tripping breaker",
				"channel", ch.Name,
				"error", err.Error(),
			)
			attempted = append(attempted, ch.Name)
			continue
		}
		attempted = append(attempted, d.recordChannelFailure(ctx, ch.Name, err, i+1))
	}

	// A loop that only ever skipped channels (all circuit-broken) leaves lastErr
	// nil. Wrapping it below printed "%!w(<nil>)" into the audit log; replace it
	// with the condition the operator actually needs to see.
	if lastErr == nil {
		lastErr = skippedOnlyError(req.Model, skipped)
	}

	if fb := d.GetModelFallback(req.Model); fb != "" && !strings.EqualFold(fb, req.Model) {
		nextCtx, fbErr := nextFallbackContext(ctx, req.Model, fb)
		if fbErr != nil {
			routerLog(ctx).Warn("aborting fallback to prevent cycle or depth exhaustion", "error", fbErr.Error())
			return nil, newExhaustedError(ctx, req.Model, exhaustedTrail(attempted, skipped), fmt.Errorf("all %d candidate channels failed for model %s (last error: %v); cannot fallback to %s: %w", len(channels), req.Model, lastErr, fb, fbErr))
		}
		routerLog(ctx).Warn("all candidate channels failed, triggering cross-model fallback",
			"requested_model", req.Model,
			"fallback_model", fb,
			"last_error", lastErr,
		)
		reqCopy := *req
		reqCopy.Model = fb
		return d.Dispatch(withChannelTrail(nextCtx, attempted), &reqCopy)
	}

	telemetry.GlobalMetrics.RecordRequest(false, 0, 0, 0)
	return nil, newExhaustedError(ctx, req.Model, exhaustedTrail(attempted, skipped), fmt.Errorf("all %d candidate channels failed for model %s. Last error: %w", len(channels), req.Model, lastErr))
}

func (d *Dispatcher) candidateChannelsForRequest(ctx context.Context, req *model.ChatCompletionRequest, proto string) []*model.ChannelConfig {
	channels := d.ResolveCandidateChannels(ctx, req.Model, proto)
	if !req.AnthropicNativeOnly {
		return channels
	}
	native := make([]*model.ChannelConfig, 0, len(channels))
	for _, channel := range channels {
		if isAnthropicWireChannel(channel) {
			native = append(native, channel)
		}
	}
	if len(native) > 0 {
		return native
	}
	// Fall back to general channels capable of serving the model via protocol adaptation
	return channels
}

// GetBreakerStatus returns the circuit breaker status of a channel.
func (d *Dispatcher) GetBreakerStatus(name string) string {
	if d.circuitBreaker == nil {
		return "CLOSED"
	}
	return d.circuitBreaker.GetStatus(name).String()
}
