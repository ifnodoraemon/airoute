package router

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/ifnodoraemon/airoute/internal/model"
)

// R-07: the silent-preamble buffer is a memory bound. When it is full the oldest
// silent frame must be dropped — never the newest one, because that event may be
// the one that commits the stream and carries the first content token.
func TestStreamPreambleCapKeepsCommittingEvent(t *testing.T) {
	events := make([]*model.StreamEvent, 0, maxPreambleEvents+1)
	for i := 0; i < maxPreambleEvents; i++ {
		events = append(events, &model.StreamEvent{Chunk: &model.ChatCompletionChunk{
			Choices: []model.ChunkChoice{{Index: 0, Delta: model.ChunkDelta{}}},
		}})
	}
	events = append(events, &model.StreamEvent{Chunk: &model.ChatCompletionChunk{
		Choices: []model.ChunkChoice{{Index: 0, Delta: model.ChunkDelta{Content: "FIRST-TOKEN"}}},
	}})

	fake := &scriptedStreamProvider{events: events}
	d := NewDispatcher([]model.ChannelConfig{{
		Name: "slow-preamble", Type: model.ProviderCustom, Models: []string{"m"},
		Status: "active", Priority: 1, Weight: 1, TimeoutSeconds: 5,
	}})
	d.RegisterProvider(fake)

	stream, err := d.DispatchStream(context.Background(), &model.ChatCompletionRequest{Model: "m", Stream: true})
	if err != nil {
		t.Fatalf("dispatch stream: %v", err)
	}

	var content strings.Builder
	timeout := time.After(5 * time.Second)
	for {
		select {
		case ev, ok := <-stream:
			if !ok {
				if got := content.String(); got != "FIRST-TOKEN" {
					t.Fatalf("client received %q: the event that committed the stream was dropped by the preamble cap", got)
				}
				return
			}
			if ev != nil && ev.Chunk != nil {
				for _, choice := range ev.Chunk.Choices {
					content.WriteString(choice.Delta.Content)
				}
			}
		case <-timeout:
			t.Fatalf("stream did not finish")
		}
	}
}

// R-16: GATEWAY_FIRST_TOKEN_TIMEOUT is documented as an upper bound. It must be able
// to shorten the window but never extend it past the channel budget.
func TestFirstTokenOverrideIsUpperBoundOnly(t *testing.T) {
	cases := []struct {
		name       string
		ch         *model.ChannelConfig
		modelName  string
		override   time.Duration
		wantWindow time.Duration
	}{
		{"override shortens a large channel budget", &model.ChannelConfig{TimeoutSeconds: 300}, "m", 15 * time.Second, 15 * time.Second},
		{"override never extends the channel budget", &model.ChannelConfig{TimeoutSeconds: 10}, "m", 60 * time.Second, 10 * time.Second},
		{"channel budget without override", &model.ChannelConfig{TimeoutSeconds: 300}, "m", 0, 300 * time.Second},
		{"override without a channel budget", &model.ChannelConfig{}, "m", 5 * time.Second, 5 * time.Second},
		{"reasoning default when nothing is configured", &model.ChannelConfig{}, "deepseek-r1", 0, 60 * time.Second},
		{"plain default when nothing is configured", &model.ChannelConfig{}, "deepseek-v4.1-flash", 0, 30 * time.Second},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := resolveFirstTokenTimeout(tc.ch, tc.modelName, tc.override); got != tc.wantWindow {
				t.Fatalf("first-token window = %v, want %v", got, tc.wantWindow)
			}
		})
	}
}
