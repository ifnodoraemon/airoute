package router

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/ifnodoraemon/airoute/internal/model"
	"github.com/stretchr/testify/require"
)

// Expose only attempt.Done on the first select, then expose parent.Done so a
// regression terminates and fails the iteration assertion instead of hanging.
type delayedDoneContext struct {
	context.Context
	iterations int
}

func (c *delayedDoneContext) Done() <-chan struct{} {
	c.iterations++
	if c.iterations == 1 {
		return nil
	}
	return c.Context.Done()
}

func TestStreamPreambleAttemptCancellationExitsFirstIteration(t *testing.T) {
	parent, cancel := context.WithCancelCause(context.Background())
	cause := errors.New("client disconnected")
	cancel(cause)
	ctx := &delayedDoneContext{Context: parent}
	attempt, cancelAttempt := context.WithCancel(context.Background())
	cancelAttempt()
	_, err := waitStreamPreamble(ctx, attempt, nil, "up", time.Second)
	require.ErrorIs(t, err, cause)
	require.Equal(t, 1, ctx.iterations, "attempt cancellation must exit in one select iteration")
}

type waitingStreamProvider struct {
	scriptedStreamProvider
	started chan context.Context
}

func (p *waitingStreamProvider) ChatCompleteStream(ctx context.Context, _ *model.ChatCompletionRequest, _ *model.ChannelConfig) (<-chan *model.StreamEvent, error) {
	p.started <- ctx
	return make(chan *model.StreamEvent), nil
}

func TestStreamPreambleParentCancelReleasesCanary(t *testing.T) {
	d := NewDispatcher([]model.ChannelConfig{{Name: "up", Type: model.ProviderCustom, Models: []string{"m"}, Status: "active"}})
	d.circuitBreaker = NewCircuitBreaker(1, time.Second)
	d.circuitBreaker.breakers["up"] = &BreakerStatus{State: StateHalfOpen, Generation: 7, LastStateChange: time.Now(), handler: &halfOpenState{}}
	p := &waitingStreamProvider{started: make(chan context.Context, 1)}
	d.RegisterProvider(p)
	ctx, cancel := context.WithCancelCause(context.Background())
	defer cancel(nil)
	done := make(chan error, 1)
	go func() { _, err := d.DispatchStream(ctx, &model.ChatCompletionRequest{Model: "m"}); done <- err }()
	attempt := <-p.started
	cause := errors.New("caller stopped waiting")
	cancel(cause)
	select {
	case err := <-done:
		require.ErrorIs(t, err, cause)
		require.ErrorIs(t, err, context.Canceled)
		de, ok := AsDispatchError(err)
		require.True(t, ok)
		require.Equal(t, []string{"up"}, de.Attempted)
		require.ErrorIs(t, attempt.Err(), context.Canceled)
		require.Zero(t, d.circuitBreaker.Snapshot("up").CanaryInFlight)
	case <-time.After(time.Second):
		t.Fatal("dispatch did not exit promptly")
	}
}

func TestAbortCanaryIgnoresStaleGeneration(t *testing.T) {
	cb := NewCircuitBreaker(1, time.Second)
	cb.breakers["up"] = &BreakerStatus{State: StateHalfOpen, Generation: 8, CanaryInFlight: 1, handler: &halfOpenState{}}
	cb.AbortCanaryGen("up", 7)
	require.Equal(t, 1, cb.Snapshot("up").CanaryInFlight)
	cb.AbortCanaryGen("up", 8)
	require.Zero(t, cb.Snapshot("up").CanaryInFlight)
}
