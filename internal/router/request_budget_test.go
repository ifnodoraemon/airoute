package router

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/ifnodoraemon/airoute/internal/model"
)

// blockingProvider models an upstream that hangs until the context is
// cancelled. It makes the budget observable: without one, Dispatch would wait
// for the full channel timeout.
type blockingProvider struct{}

func (blockingProvider) Name() string             { return "blocking" }
func (blockingProvider) Type() model.ProviderType { return model.ProviderCustom }
func (blockingProvider) ChatComplete(ctx context.Context, _ *model.ChatCompletionRequest, _ *model.ChannelConfig) (*model.ChatCompletionResponse, error) {
	<-ctx.Done()
	return nil, ctx.Err()
}
func (blockingProvider) ChatCompleteStream(ctx context.Context, _ *model.ChatCompletionRequest, _ *model.ChannelConfig) (<-chan *model.StreamEvent, error) {
	<-ctx.Done()
	return nil, ctx.Err()
}

func TestDispatchStopsAtRequestBudget(t *testing.T) {
	t.Setenv(RequestBudgetEnv, "1")

	dispatcher := NewDispatcher([]model.ChannelConfig{{
		ID: 1, Name: "slow", Type: model.ProviderCustom,
		BaseURL: "http://127.0.0.1:1", Models: []string{"slow-model"}, TimeoutSeconds: 30,
	}})
	dispatcher.RegisterProvider(blockingProvider{})

	start := time.Now()
	resp, err := dispatcher.Dispatch(context.Background(), &model.ChatCompletionRequest{Model: "slow-model"})
	elapsed := time.Since(start)

	if resp != nil {
		t.Fatalf("expected no response, got %+v", resp)
	}
	if !errors.Is(err, ErrRequestBudgetExceeded) {
		t.Fatalf("expected ErrRequestBudgetExceeded, got %v", err)
	}
	if elapsed < 900*time.Millisecond {
		t.Fatalf("dispatch returned before the budget elapsed: %v", elapsed)
	}
	if elapsed > 5*time.Second {
		t.Fatalf("dispatch ignored the 1s budget: %v", elapsed)
	}
}

func TestDispatchRespectsCallerShorterDeadline(t *testing.T) {
	t.Setenv(RequestBudgetEnv, "30")

	dispatcher := NewDispatcher([]model.ChannelConfig{{
		ID: 1, Name: "slow", Type: model.ProviderCustom,
		BaseURL: "http://127.0.0.1:1", Models: []string{"slow-model"}, TimeoutSeconds: 30,
	}})
	dispatcher.RegisterProvider(blockingProvider{})

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	start := time.Now()
	_, err := dispatcher.Dispatch(ctx, &model.ChatCompletionRequest{Model: "slow-model"})
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("expected the caller deadline to fail the dispatch")
	}
	if errors.Is(err, ErrRequestBudgetExceeded) {
		t.Fatal("a caller-supplied deadline must not be reported as gateway budget exhaustion")
	}
	if elapsed > 2*time.Second {
		t.Fatalf("caller deadline ignored: %v", elapsed)
	}
}

func TestRequestBudgetFromEnv(t *testing.T) {
	cases := map[string]time.Duration{
		"":     DefaultRequestBudget,
		"0":    0,
		"-5":   0,
		"45":   45 * time.Second,
		"oops": 0,
	}
	for raw, want := range cases {
		t.Setenv(RequestBudgetEnv, raw)
		if got := requestBudgetFromEnv(); got != want {
			t.Errorf("requestBudgetFromEnv(%q) = %v, want %v", raw, got, want)
		}
	}
}
