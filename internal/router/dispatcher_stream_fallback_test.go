package router

import (
	"context"
	"errors"
	"testing"

	"github.com/ifnodoraemon/airoute/internal/model"
	"github.com/ifnodoraemon/airoute/internal/provider"
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

// Declaring the inbound anthropic_messages capability does not make an upstream
// speak the Anthropic wire protocol: the channel keeps using its own provider and
// the gateway translates. Native passthrough needs the explicit marker.
func TestAnthropicMessagesCapabilityDoesNotSelectNativeWire(t *testing.T) {
	ch := model.ChannelConfig{
		Name:      "gpustack-cluster",
		Type:      model.ProviderGPUStack,
		Status:    "active",
		Models:    []string{"deepseek-v4.1-flash"},
		Protocols: []string{"openai_chat", "anthropic_messages"},
	}
	d := NewDispatcher([]model.ChannelConfig{ch})

	own := &wireProvider{typ: model.ProviderGPUStack}
	d.RegisterProvider(own)

	prov, ok := d.getProviderForChannel(&ch)
	if !ok {
		t.Fatalf("channel has no provider")
	}
	if prov.Type() == model.ProviderAnthropic {
		t.Fatalf("an OpenAI-speaking channel that merely accepts /v1/messages must be translated, not called with the Anthropic wire protocol")
	}
	if prov.Type() != model.ProviderGPUStack {
		t.Fatalf("provider = %v, want the channel's own provider", prov.Type())
	}

	ch.Protocols = append(ch.Protocols, AnthropicNativeProtocol)
	if prov, ok := d.getProviderForChannel(&ch); !ok || prov.Type() != model.ProviderAnthropic {
		t.Fatalf("explicit %q marker must select the native Anthropic provider, got %v", AnthropicNativeProtocol, prov)
	}
}

// midStreamFailProvider commits the stream (emits a content token) and then drops
// the connection: the "connects, first token works, long output always breaks"
// degradation that used to be invisible to the breaker.
type midStreamFailProvider struct{}

func (p *midStreamFailProvider) Name() string             { return "mid-stream-fail" }
func (p *midStreamFailProvider) Type() model.ProviderType { return model.ProviderCustom }
func (p *midStreamFailProvider) ChatComplete(context.Context, *model.ChatCompletionRequest, *model.ChannelConfig) (*model.ChatCompletionResponse, error) {
	return nil, errors.New("not used")
}
func (p *midStreamFailProvider) ChatCompleteStream(context.Context, *model.ChatCompletionRequest, *model.ChannelConfig) (<-chan *model.StreamEvent, error) {
	out := make(chan *model.StreamEvent, 2)
	out <- &model.StreamEvent{Chunk: &model.ChatCompletionChunk{Choices: []model.ChunkChoice{{Delta: model.ChunkDelta{Content: "first token"}}}}}
	out <- &model.StreamEvent{Err: errors.New("upstream reset the connection mid-stream")}
	close(out)
	return out, nil
}

// blockAfterFirstTokenProvider commits the stream and then waits for its context,
// modelling an upstream whose client simply hangs up mid-generation.
type blockAfterFirstTokenProvider struct{}

func (p *blockAfterFirstTokenProvider) Name() string             { return "block-after-first-token" }
func (p *blockAfterFirstTokenProvider) Type() model.ProviderType { return model.ProviderCustom }
func (p *blockAfterFirstTokenProvider) ChatComplete(context.Context, *model.ChatCompletionRequest, *model.ChannelConfig) (*model.ChatCompletionResponse, error) {
	return nil, errors.New("not used")
}
func (p *blockAfterFirstTokenProvider) ChatCompleteStream(ctx context.Context, _ *model.ChatCompletionRequest, _ *model.ChannelConfig) (<-chan *model.StreamEvent, error) {
	out := make(chan *model.StreamEvent, 1)
	out <- &model.StreamEvent{Chunk: &model.ChatCompletionChunk{Choices: []model.ChunkChoice{{Delta: model.ChunkDelta{Content: "first token"}}}}}
	go func() {
		<-ctx.Done()
		close(out)
	}()
	return out, nil
}

// B-02: a stream that fails AFTER the first token must still be recorded as a
// channel failure. The old pipe only forwarded event.Err to the audit row, so a
// channel that always truncated long output stayed CLOSED forever.
func TestStreamMidFailureRecordsBreakerFailure(t *testing.T) {
	d := NewDispatcher([]model.ChannelConfig{{
		Name: "flaky", Type: model.ProviderCustom, Status: "active", Models: []string{"example"},
	}})
	d.RegisterProvider(&midStreamFailProvider{})

	stream, err := d.DispatchStream(context.Background(), &model.ChatCompletionRequest{Model: "example", Stream: true})
	if err != nil {
		t.Fatalf("dispatch: %v", err)
	}
	var sawContent, sawErr bool
	for event := range stream {
		if event.Err != nil {
			sawErr = true
		}
		if streamEventHasContent(event) {
			sawContent = true
		}
	}
	if !sawContent || !sawErr {
		t.Fatalf("expected the commit token and the error event to reach the client (content=%v err=%v)", sawContent, sawErr)
	}
	snapshot := d.circuitBreaker.Snapshot("flaky")
	if snapshot.Failures != 1 {
		t.Fatalf("post-first-token stream failure must be recorded exactly once, got %+v", snapshot)
	}
}

// midStreamRateLimitProvider aborts the stream with a typed rate-limit error: that
// is backpressure from a healthy upstream, not a breaker failure (B-10 semantics
// must hold mid-stream too).
type midStreamRateLimitProvider struct{}

func (p *midStreamRateLimitProvider) Name() string             { return "mid-stream-429" }
func (p *midStreamRateLimitProvider) Type() model.ProviderType { return model.ProviderCustom }
func (p *midStreamRateLimitProvider) ChatComplete(context.Context, *model.ChatCompletionRequest, *model.ChannelConfig) (*model.ChatCompletionResponse, error) {
	return nil, errors.New("not used")
}
func (p *midStreamRateLimitProvider) ChatCompleteStream(context.Context, *model.ChatCompletionRequest, *model.ChannelConfig) (<-chan *model.StreamEvent, error) {
	out := make(chan *model.StreamEvent, 2)
	out <- &model.StreamEvent{Chunk: &model.ChatCompletionChunk{Choices: []model.ChunkChoice{{Delta: model.ChunkDelta{Content: "first token"}}}}}
	out <- &model.StreamEvent{Err: provider.NewUpstreamError("mid-stream-429", 429, []byte("slow down"))}
	close(out)
	return out, nil
}

// B-02/B-10: a mid-stream 429 swaps the channel but must not be charged to the
// breaker, mirroring the unary rule.
func TestStreamMidRateLimitDoesNotChargeBreaker(t *testing.T) {
	d := NewDispatcher([]model.ChannelConfig{{
		Name: "limited", Type: model.ProviderCustom, Status: "active", Models: []string{"example"},
	}})
	d.RegisterProvider(&midStreamRateLimitProvider{})

	stream, err := d.DispatchStream(context.Background(), &model.ChatCompletionRequest{Model: "example", Stream: true})
	if err != nil {
		t.Fatalf("dispatch: %v", err)
	}
	for range stream {
	}
	if snapshot := d.circuitBreaker.Snapshot("limited"); snapshot.Failures != 0 {
		t.Fatalf("mid-stream 429 charged the breaker: %+v", snapshot)
	}
}

// B-02 (negative case): the client hanging up is not an upstream failure. The
// breaker must stay clean when ctx is cancelled mid-stream.
func TestStreamClientCancelDoesNotChargeBreaker(t *testing.T) {
	d := NewDispatcher([]model.ChannelConfig{{
		Name: "long-stream", Type: model.ProviderCustom, Status: "active", Models: []string{"example"},
	}})
	d.RegisterProvider(&blockAfterFirstTokenProvider{})

	ctx, cancel := context.WithCancel(context.Background())
	stream, err := d.DispatchStream(ctx, &model.ChatCompletionRequest{Model: "example", Stream: true})
	if err != nil {
		t.Fatalf("dispatch: %v", err)
	}
	if event := <-stream; !streamEventHasContent(event) {
		t.Fatalf("expected a content token before cancelling, got %+v", event)
	}
	cancel()
	for range stream {
		// drain until the cancellation closes the stream
	}
	if snapshot := d.circuitBreaker.Snapshot("long-stream"); snapshot.Failures != 0 {
		t.Fatalf("client cancellation charged the breaker: %+v", snapshot)
	}
}
