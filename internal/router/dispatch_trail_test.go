package router

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/ifnodoraemon/airoute/internal/model"
	"github.com/ifnodoraemon/airoute/internal/provider"
	"github.com/stretchr/testify/require"
)

type trailRejectProvider struct {
	scriptedStreamProvider
	rejection  error
	eventError bool
}

func (p *trailRejectProvider) ChatComplete(_ context.Context, _ *model.ChatCompletionRequest, ch *model.ChannelConfig) (*model.ChatCompletionResponse, error) {
	if ch.Name == "fatal" {
		return nil, p.rejection
	}
	return nil, provider.NewUpstreamError(ch.Name, 502, nil)
}

func (p *trailRejectProvider) ChatCompleteStream(ctx context.Context, req *model.ChatCompletionRequest, ch *model.ChannelConfig) (<-chan *model.StreamEvent, error) {
	_, err := p.ChatComplete(ctx, req, ch)
	if !p.eventError {
		return nil, err
	}
	out := make(chan *model.StreamEvent, 1)
	out <- &model.StreamEvent{Err: err}
	close(out)
	return out, nil
}

func trailDispatcher() *Dispatcher {
	channels := []model.ChannelConfig{}
	for i, name := range []string{"skip", "first", "fatal"} {
		channels = append(channels, model.ChannelConfig{Name: name, BaseURL: "https://" + name + ".invalid", Type: model.ProviderCustom, Models: []string{"m"}, Status: "active", Priority: i + 1, MaxConcurrency: 1, QueueLimit: 1})
	}
	d := NewDispatcher(channels)
	d.circuitBreaker = NewCircuitBreaker(1, time.Hour)
	d.circuitBreaker.RecordFailure("skip")
	return d
}

func trailDispatch(d *Dispatcher, ctx context.Context, stream bool) error {
	if stream {
		_, err := d.DispatchStream(ctx, &model.ChatCompletionRequest{Model: "m"})
		return err
	}
	_, err := d.Dispatch(ctx, &model.ChatCompletionRequest{Model: "m"})
	return err
}

func TestFatalPayloadPreservesRoutingTrail(t *testing.T) {
	for _, status := range []int{400, 413, 422} {
		for _, mode := range []string{"unary", "stream-connect", "stream-event"} {
			t.Run(fmt.Sprintf("%d/%s", status, mode), func(t *testing.T) {
				d := trailDispatcher()
				rejection := provider.NewUpstreamError("fatal", status, []byte("invalid payload"))
				d.RegisterProvider(&trailRejectProvider{rejection: rejection, eventError: mode == "stream-event"})
				err := trailDispatch(d, context.Background(), mode != "unary")
				require.ErrorIs(t, err, rejection)
				require.True(t, provider.IsFatalPayloadError(err))
				de, ok := AsDispatchError(err)
				require.True(t, ok)
				require.Equal(t, []string{"first", "fatal"}, de.Attempted)
				require.Equal(t, []string{"skip"}, de.Skipped)
				upstream, ok := provider.AsUpstreamError(err)
				require.True(t, ok)
				require.Equal(t, status, upstream.Status)
			})
		}
	}
}

func TestFallbackPreservesAttemptedAndSkipped(t *testing.T) {
	for _, stream := range []bool{false, true} {
		t.Run(fmt.Sprint(stream), func(t *testing.T) {
			d := trailDispatcher()
			d.RegisterProvider(&trailRejectProvider{rejection: provider.NewUpstreamError("fatal", 502, nil)})
			d.SetModelFallbacks(map[string]string{"m": "unroutable"})
			err := trailDispatch(d, context.Background(), stream)
			de, ok := AsDispatchError(err)
			require.True(t, ok)
			require.True(t, de.NoRoute)
			require.Equal(t, []string{"first", "fatal"}, de.Attempted)
			require.Equal(t, []string{"skip"}, de.Skipped)
		})
	}
}

func TestAdmissionCancellationPreservesRejectedChannel(t *testing.T) {
	for _, stream := range []bool{false, true} {
		t.Run(fmt.Sprint(stream), func(t *testing.T) {
			d := trailDispatcher()
			d.RegisterProvider(&trailRejectProvider{})
			release, err := d.lanes.acquire(context.Background(), &d.channels[2])
			require.NoError(t, err)
			defer release()
			ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
			defer cancel()
			err = trailDispatch(d, ctx, stream)
			require.ErrorIs(t, err, context.DeadlineExceeded)
			de, ok := AsDispatchError(err)
			require.True(t, ok)
			require.Equal(t, []string{"first"}, de.Attempted)
			require.Equal(t, []string{"skip", "fatal"}, de.Skipped)
		})
	}
}
