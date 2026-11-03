package adapter

import (
	"context"
	"sync"

	"github.com/ifnodoraemon/airoute/internal/model"
)

// ProtocolAdapter defines the contract for bidirectional protocol adaptation under the Adapter Pattern.
type ProtocolAdapter interface {
	Protocol() string
	ToCanonical(ctx context.Context, input any) (*model.ChatCompletionRequest, error)
	FromCanonical(ctx context.Context, resp *model.ChatCompletionResponse) (any, error)
}

var (
	registryMu sync.RWMutex
	registry   = make(map[string]ProtocolAdapter)
)

// Register registers a protocol adapter into the global registry.
func Register(a ProtocolAdapter) {
	if a == nil {
		return
	}
	registryMu.Lock()
	defer registryMu.Unlock()
	registry[a.Protocol()] = a
}

// Get retrieves a registered protocol adapter by its protocol name.
func Get(protocol string) (ProtocolAdapter, bool) {
	registryMu.RLock()
	defer registryMu.RUnlock()
	a, ok := registry[protocol]
	return a, ok
}

func init() {
	// Only Gemini implements both directions through this package. Anthropic is
	// inbound-only here: its Messages response rendering lives in
	// internal/api/inbound_anthropic.go, because the Messages API requires a
	// provider signature on every thinking block and a canonical->Messages
	// conversion would have to synthesise an unsigned one (the follow-up request
	// is then rejected with "thinking.signature: Field required"). Cache
	// accounting and the msg_ id prefix are owned there as well. Registering an
	// outbound anthropic implementation here would be a regression trap, so only
	// Gemini is registered; the inbound anthropic conversion is constructed by its
	// caller (see AnthropicAdapter.FromCanonical, B-05).
	Register(NewGeminiAdapter())
}
