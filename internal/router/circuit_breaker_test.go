package router

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/ifnodoraemon/airoute/internal/model"
)

func TestCircuitBreaker_AbortCanary(t *testing.T) {
	cb := NewCircuitBreaker(2, 50*time.Millisecond)

	// Trip to open
	cb.RecordFailure("test-ch")
	cb.RecordFailure("test-ch")
	if cb.GetStatus("test-ch") != StateOpen {
		t.Fatalf("expected StateOpen, got %v", cb.GetStatus("test-ch"))
	}

	// Wait for cooldown to allow half-open probe
	time.Sleep(60 * time.Millisecond)

	// First execution should transition to half-open and consume canary
	if !cb.CanExecute("test-ch") {
		t.Fatalf("expected CanExecute to return true for half-open probe")
	}
	if cb.GetStatus("test-ch") != StateHalfOpen {
		t.Fatalf("expected StateHalfOpen, got %v", cb.GetStatus("test-ch"))
	}

	// While canary is in flight, next CanExecute should return false
	if cb.CanExecute("test-ch") {
		t.Fatalf("expected CanExecute to return false while canary probe is in-flight")
	}

	// Client cancels request: abort canary
	cb.AbortCanary("test-ch")

	// Now another probe should be admitted immediately without waiting 2x cooldown
	if !cb.CanExecute("test-ch") {
		t.Fatalf("expected CanExecute to succeed after AbortCanary was called")
	}
}

type cancelingTestProvider struct{}

func (m *cancelingTestProvider) Name() string             { return "canceling_test" }
func (m *cancelingTestProvider) Type() model.ProviderType { return model.ProviderOpenAI }

func (m *cancelingTestProvider) ChatComplete(ctx context.Context, req *model.ChatCompletionRequest, channel *model.ChannelConfig) (*model.ChatCompletionResponse, error) {
	return nil, context.Canceled
}

func (m *cancelingTestProvider) ChatCompleteStream(ctx context.Context, req *model.ChatCompletionRequest, channel *model.ChannelConfig) (<-chan *model.StreamEvent, error) {
	return nil, context.Canceled
}

func TestDispatcher_ClientCancellationDoesNotTripCircuitBreaker(t *testing.T) {
	channels := []model.ChannelConfig{
		{
			ID:       1,
			Name:     "ch-1",
			Type:     model.ProviderOpenAI,
			Status:   "active",
			Models:   []string{"test-model"},
			Priority: 1,
			Weight:   100,
		},
		{
			ID:       2,
			Name:     "ch-2",
			Type:     model.ProviderOpenAI,
			Status:   "active",
			Models:   []string{"test-model"},
			Priority: 2,
			Weight:   100,
		},
	}

	d := NewDispatcher(channels)
	d.RegisterProvider(&cancelingTestProvider{})

	// Canceled context
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	req := &model.ChatCompletionRequest{
		Model: "test-model",
	}

	resp, err := d.Dispatch(ctx, req)
	if resp != nil {
		t.Fatalf("expected nil response on canceled context, got %v", resp)
	}
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got %v", err)
	}

	// Verify circuit breaker status for ch-1 and ch-2 is still CLOSED (not tripped!)
	if d.circuitBreaker.GetStatus("ch-1") != StateClosed {
		t.Fatalf("expected ch-1 breaker to remain Closed, got %v", d.circuitBreaker.GetStatus("ch-1"))
	}
	if d.circuitBreaker.GetStatus("ch-2") != StateClosed {
		t.Fatalf("expected ch-2 breaker to remain Closed, got %v", d.circuitBreaker.GetStatus("ch-2"))
	}
}
