package adapter

import (
	"context"
	"fmt"
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

// MustGet retrieves a registered protocol adapter or returns an error if not found.
func MustGet(protocol string) (ProtocolAdapter, error) {
	if a, ok := Get(protocol); ok {
		return a, nil
	}
	return nil, fmt.Errorf("protocol adapter for '%s' is not registered", protocol)
}

func init() {
	Register(NewAnthropicAdapter())
	Register(NewGeminiAdapter())
}
