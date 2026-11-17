package router

import (
	"context"
	"testing"
	"time"

	"github.com/ifnodoraemon/airoute/internal/model"
)

// A bounded preamble commits without dropping any protocol frame.
func TestStreamPreambleCapPreservesEveryEvent(t *testing.T) {
	events := make([]*model.StreamEvent, 0, maxPreambleEvents+6)
	for i := 0; i < maxPreambleEvents+3; i++ {
		events = append(events, &model.StreamEvent{Chunk: &model.ChatCompletionChunk{
			Choices: []model.ChunkChoice{{Index: 0, Delta: model.ChunkDelta{Role: "assistant"}}},
		}})
	}
	events = append(events, &model.StreamEvent{Chunk: &model.ChatCompletionChunk{
		Choices: []model.ChunkChoice{{Delta: model.ChunkDelta{Content: "FIRST-TOKEN"}}},
	}}, &model.StreamEvent{IsDone: true})
	d := NewDispatcher([]model.ChannelConfig{{Name: "preamble", Type: model.ProviderCustom, Models: []string{"m"}, Status: "active"}})
	d.RegisterProvider(&scriptedStreamProvider{events: events})
	stream, err := d.DispatchStream(context.Background(), &model.ChatCompletionRequest{Model: "m", Stream: true})
	if err != nil {
		t.Fatal(err)
	}
	count := 0
	for event := range stream {
		if count >= len(events) || event != events[count] {
			t.Fatalf("event %d was lost or reordered", count)
		}
		count++
	}
	if count != len(events) {
		t.Fatalf("received %d of %d events", count, len(events))
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
