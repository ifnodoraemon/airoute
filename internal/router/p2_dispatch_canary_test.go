package router

import (
	"context"
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/ifnodoraemon/airoute/internal/model"
	"github.com/ifnodoraemon/airoute/internal/provider"
)

type p2FakeProvider struct {
	mu   sync.Mutex
	errs []error
}

func (p *p2FakeProvider) Name() string             { return "p2-fake" }
func (p *p2FakeProvider) Type() model.ProviderType { return model.ProviderCustom }
func (p *p2FakeProvider) ChatComplete(context.Context, *model.ChatCompletionRequest, *model.ChannelConfig) (*model.ChatCompletionResponse, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if len(p.errs) > 0 {
		err := p.errs[0]
		p.errs = p.errs[1:]
		return nil, err
	}
	return &model.ChatCompletionResponse{Choices: []model.ChatCompletionChoice{{Index: 0, Message: model.ChatMessage{Role: "assistant", Content: "ok"}}}}, nil
}
func (p *p2FakeProvider) ChatCompleteStream(context.Context, *model.ChatCompletionRequest, *model.ChannelConfig) (<-chan *model.StreamEvent, error) {
	return nil, provider.NewUpstreamError("p2", http.StatusBadGateway, nil)
}

// R-05: a non-health error (429) returned by the half-open canary must hand the
// canary slot back. It used to leak, and CanExecute then refused the channel until
// the 2x-cooldown stale-slot reset even though the channel had answered.
func TestP2DispatchDoesNotLeakHalfOpenCanaryOnNonHealthError(t *testing.T) {
	fake := &p2FakeProvider{errs: []error{
		provider.NewUpstreamError("ch", http.StatusInternalServerError, []byte("boom")),
		provider.NewUpstreamError("ch", http.StatusTooManyRequests, []byte("slow down")),
	}}
	d := NewDispatcher([]model.ChannelConfig{{
		Name: "ch", Type: model.ProviderCustom, Models: []string{"m"}, Status: "active", Priority: 1, Weight: 1,
	}})
	d.RegisterProvider(fake)
	d.circuitBreaker = NewCircuitBreaker(1, 5*time.Millisecond)

	// Trip the breaker, then let the cooldown expire so the next attempt is the
	// half-open canary.
	if _, err := d.Dispatch(context.Background(), &model.ChatCompletionRequest{Model: "m"}); err == nil {
		t.Fatalf("first dispatch unexpectedly succeeded")
	}
	if got := d.GetBreakerStatus("ch"); got != "OPEN" {
		t.Fatalf("setup: breaker state = %s, want OPEN", got)
	}
	time.Sleep(10 * time.Millisecond)

	// Canary attempt returns 429: not a channel failure, but the slot must return.
	if _, err := d.Dispatch(context.Background(), &model.ChatCompletionRequest{Model: "m"}); err == nil {
		t.Fatalf("canary dispatch unexpectedly succeeded")
	}

	snapshot := d.circuitBreaker.Snapshot("ch")
	if snapshot.CanaryInFlight != 0 {
		t.Fatalf("half-open canary slot leaked: %+v (the channel is locked out of rotation)", snapshot)
	}
	if !d.circuitBreaker.CanExecute("ch") {
		t.Fatalf("channel is refused after a rate-limited canary: %+v", snapshot)
	}
}
