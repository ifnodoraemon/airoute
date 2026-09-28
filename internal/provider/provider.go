package provider

import (
	"context"

	"github.com/ifnodoraemon/airoute/internal/model"
)

// Provider defines the interface for interacting with upstream LLM engines.
type Provider interface {
	Name() string
	Type() model.ProviderType

	// ChatComplete executes a non-streaming chat completion request.
	ChatComplete(ctx context.Context, req *model.ChatCompletionRequest, channel *model.ChannelConfig) (*model.ChatCompletionResponse, error)

	// ChatCompleteStream executes a streaming chat completion request, returning a channel of SSE events.
	ChatCompleteStream(ctx context.Context, req *model.ChatCompletionRequest, channel *model.ChannelConfig) (<-chan *model.StreamEvent, error)
}
