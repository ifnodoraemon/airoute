package router

import (
	"context"
	"errors"
	"testing"

	"github.com/ifnodoraemon/airoute/internal/model"
)

type preambleFailProvider struct{}

func (p *preambleFailProvider) Name() string             { return "preamble-fail" }
func (p *preambleFailProvider) Type() model.ProviderType { return model.ProviderOpenAI }
func (p *preambleFailProvider) ChatComplete(context.Context, *model.ChatCompletionRequest, *model.ChannelConfig) (*model.ChatCompletionResponse, error) {
	return nil, errors.New("not used")
}
func (p *preambleFailProvider) ChatCompleteStream(_ context.Context, _ *model.ChatCompletionRequest, ch *model.ChannelConfig) (<-chan *model.StreamEvent, error) {
	out := make(chan *model.StreamEvent, 2)
	if ch.Name == "primary" {
		out <- &model.StreamEvent{Chunk: &model.ChatCompletionChunk{Choices: []model.ChunkChoice{{Delta: model.ChunkDelta{Role: "assistant"}}}}}
		out <- &model.StreamEvent{Err: errors.New("upstream disconnected")}
	} else {
		out <- &model.StreamEvent{Chunk: &model.ChatCompletionChunk{Choices: []model.ChunkChoice{{Delta: model.ChunkDelta{Content: "backup response"}}}}}
	}
	close(out)
	return out, nil
}

func TestStreamFallsBackAfterRoleOnlyPreamble(t *testing.T) {
	d := NewDispatcher([]model.ChannelConfig{
		{Name: "primary", Type: model.ProviderOpenAI, Status: "active", Models: []string{"example"}, Priority: 1},
		{Name: "backup", Type: model.ProviderOpenAI, Status: "active", Models: []string{"example"}, Priority: 2},
	})
	d.RegisterProvider(&preambleFailProvider{})
	stream, err := d.DispatchStream(context.Background(), &model.ChatCompletionRequest{Model: "example", Stream: true})
	if err != nil {
		t.Fatal(err)
	}
	var events []*model.StreamEvent
	for event := range stream {
		events = append(events, event)
	}
	if len(events) != 1 || events[0].Chunk.Choices[0].Delta.Content != "backup response" || events[0].Channel != "primary→backup" {
		t.Fatalf("expected only backup content after preamble failure, got %+v", events)
	}
}

func TestNativeAnthropicRequestExcludesOtherProviders(t *testing.T) {
	d := NewDispatcher([]model.ChannelConfig{
		{Name: "openai", Type: model.ProviderOpenAI, Status: "active", Models: []string{"example"}},
		{Name: "anthropic", Type: model.ProviderAnthropic, Status: "active", Models: []string{"example"}},
	})
	channels := d.candidateChannelsForRequest(context.Background(), &model.ChatCompletionRequest{Model: "example", AnthropicNativeOnly: true}, "chat")
	if len(channels) != 1 || channels[0].Name != "anthropic" {
		t.Fatalf("unexpected native channels: %+v", channels)
	}
}

func TestNativeAnthropicRequestFallsBackWhenNoNativeProvider(t *testing.T) {
	d := NewDispatcher([]model.ChannelConfig{
		{Name: "gpustack", Type: model.ProviderGPUStack, Status: "active", Models: []string{"deepseek-v4.1-flash"}},
	})
	channels := d.candidateChannelsForRequest(context.Background(), &model.ChatCompletionRequest{Model: "deepseek-v4.1-flash", AnthropicNativeOnly: true}, "chat")
	if len(channels) != 1 || channels[0].Name != "gpustack" {
		t.Fatalf("expected fallback to gpustack when no native anthropic channel exists, got: %+v", channels)
	}
}
