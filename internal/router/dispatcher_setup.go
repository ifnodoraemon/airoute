package router

import (
	"context"
	"strings"
	"time"

	"github.com/ifnodoraemon/airoute/internal/model"
	"github.com/ifnodoraemon/airoute/internal/provider"
)

// NewDispatcher creates a new Dispatcher instance.
func NewDispatcher(channels []model.ChannelConfig) *Dispatcher {
	d := &Dispatcher{
		channels:       channels,
		providers:      make(map[model.ProviderType]provider.Provider),
		wrrWeights:     make(map[string]int),
		circuitBreaker: newConfiguredCircuitBreaker(),
		modelFallbacks: make(map[string]string),
		sessionPins:    newSessionPins(),
		cooldowns:      newCooldowns(),
		lanes:          newCallLanes(channels),
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
	d.providers[model.ProviderVertex] = geminiProv

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
	d.lanes.configure(channels)
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

// IsAnthropicWireChannel reports whether the channel's upstream speaks the
// Anthropic Messages protocol natively. Utility callers outside this package (for
// example the count_tokens passthrough) must use this instead of sniffing the
// inbound capability from channel.protocols.
func IsAnthropicWireChannel(ch *model.ChannelConfig) bool {
	return isAnthropicWireChannel(ch)
}

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
	return d.getProviderForRequest(ch, nil)
}

func (d *Dispatcher) getProviderForRequest(ch *model.ChannelConfig, req *model.ChatCompletionRequest) (provider.Provider, bool) {
	d.mu.RLock()
	defer d.mu.RUnlock()
	prov, exists := d.providers[model.ChatProviderType(ch, req)]
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
