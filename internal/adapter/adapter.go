package adapter

import (
	"context"

	"github.com/ifnodoraemon/airoute/internal/model"
)

// ProtocolAdapter defines the contract for bidirectional protocol adaptation under the Adapter Pattern.
type ProtocolAdapter interface {
	Protocol() string
	ToCanonical(ctx context.Context, input any) (*model.ChatCompletionRequest, error)
	FromCanonical(ctx context.Context, resp *model.ChatCompletionResponse) (any, error)
}
