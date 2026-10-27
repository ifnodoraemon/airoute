package router

import (
	"context"
	"errors"
	"testing"

	"github.com/ifnodoraemon/airoute/internal/model"
)

// wireProvider records which protocol implementation a dispatch attempt used.
type wireProvider struct {
	typ   model.ProviderType
	calls int
}

func (p *wireProvider) Name() string             { return "wire-" + string(p.typ) }
func (p *wireProvider) Type() model.ProviderType { return p.typ }
func (p *wireProvider) ChatComplete(context.Context, *model.ChatCompletionRequest, *model.ChannelConfig) (*model.ChatCompletionResponse, error) {
	p.calls++
	return &model.ChatCompletionResponse{Choices: []model.ChatCompletionChoice{{Index: 0, Message: model.ChatMessage{Role: "assistant", Content: "ok"}}}}, nil
}
func (p *wireProvider) ChatCompleteStream(context.Context, *model.ChatCompletionRequest, *model.ChannelConfig) (<-chan *model.StreamEvent, error) {
	p.calls++
	return nil, errors.New("not used")
}

func anthropicWireDispatcher(t *testing.T, channels []model.ChannelConfig) (*Dispatcher, *wireProvider, *wireProvider) {
	t.Helper()
	d := NewDispatcher(channels)
	oai := &wireProvider{typ: model.ProviderOpenAI}
	ant := &wireProvider{typ: model.ProviderAnthropic}
	d.RegisterProvider(oai)
	d.RegisterProvider(ant)
	return d, oai, ant
}

func anthropicInboundRequest() *model.ChatCompletionRequest {
	return &model.ChatCompletionRequest{
		Model:             "gpt-x",
		Protocol:          "anthropic_messages",
		AnthropicOriginal: &model.AnthropicInboundRequest{Model: "gpt-x"},
	}
}

// R-04: an OpenAI channel that declares the inbound anthropic_messages capability
// must translate, not receive Anthropic JSON at <base>/v1/messages.
func TestAnthropicInboundOnOpenAIChannelTranslates(t *testing.T) {
	d, oai, ant := anthropicWireDispatcher(t, []model.ChannelConfig{{
		Name: "openai-official", Type: model.ProviderOpenAI, BaseURL: "https://api.openai.com/v1",
		Models: []string{"gpt-x"}, Status: "active", Priority: 1, Weight: 1,
		Protocols: []string{"openai_chat", "openai_response", "anthropic_messages"},
	}})
	if _, err := d.Dispatch(context.Background(), anthropicInboundRequest()); err != nil {
		t.Fatalf("dispatch: %v", err)
	}
	if oai.calls != 1 || ant.calls != 0 {
		t.Fatalf("openai provider calls=%d anthropic provider calls=%d: an OpenAI upstream must be translated", oai.calls, ant.calls)
	}
}

// R-04: a channel with the wildcard protocol set (the console default) must not be
// treated as an Anthropic upstream either.
func TestWildcardProtocolsDoNotImplyAnthropicWire(t *testing.T) {
	d, oai, ant := anthropicWireDispatcher(t, []model.ChannelConfig{{
		Name: "openai-wildcard", Type: model.ProviderOpenAI, BaseURL: "https://example.invalid/v1",
		Models: []string{"gpt-x"}, Status: "active", Priority: 1, Weight: 1,
		Protocols: []string{"*"},
	}})
	if _, err := d.Dispatch(context.Background(), anthropicInboundRequest()); err != nil {
		t.Fatalf("dispatch: %v", err)
	}
	if oai.calls != 1 || ant.calls != 0 {
		t.Fatalf("wildcard protocols must not select the native Anthropic wire (openai=%d anthropic=%d)", oai.calls, ant.calls)
	}
}

// R-04: an Anthropic-typed channel is native regardless of the request protocol.
func TestAnthropicTypedChannelUsesNativeProvider(t *testing.T) {
	d, oai, ant := anthropicWireDispatcher(t, []model.ChannelConfig{{
		Name: "anthropic", Type: model.ProviderAnthropic, BaseURL: "https://api.anthropic.com",
		Models: []string{"claude-x"}, Status: "active", Priority: 1, Weight: 1,
		Protocols: []string{"openai_chat", "anthropic_messages"},
	}})
	req := anthropicInboundRequest()
	req.Model = "claude-x"
	if _, err := d.Dispatch(context.Background(), req); err != nil {
		t.Fatalf("dispatch: %v", err)
	}
	if ant.calls != 1 || oai.calls != 0 {
		t.Fatalf("anthropic-typed channel must use the native provider (openai=%d anthropic=%d)", oai.calls, ant.calls)
	}
}

// R-04: an OpenAI-typed channel whose upstream really is Anthropic opts in with the
// explicit marker.
func TestExplicitNativeMarkerUsesNativeProvider(t *testing.T) {
	d, oai, ant := anthropicWireDispatcher(t, []model.ChannelConfig{{
		Name: "claude-proxy", Type: model.ProviderCustom, BaseURL: "https://claude-proxy.invalid",
		Models: []string{"claude-x"}, Status: "active", Priority: 1, Weight: 1,
		Protocols: []string{"openai_chat", AnthropicNativeProtocol},
	}})
	req := anthropicInboundRequest()
	req.Model = "claude-x"
	if _, err := d.Dispatch(context.Background(), req); err != nil {
		t.Fatalf("dispatch: %v", err)
	}
	if ant.calls != 1 || oai.calls != 0 {
		t.Fatalf("explicit %s marker must use the native provider (openai=%d anthropic=%d)", AnthropicNativeProtocol, oai.calls, ant.calls)
	}
}

// R-04: native-only requests (extended thinking, cache_control, top_k) must not be
// routed to an OpenAI channel that merely declares the capability; with no native
// channel available the documented translation fallback still applies.
func TestNativeOnlyCandidatesRequireNativeWire(t *testing.T) {
	d, _, _ := anthropicWireDispatcher(t, []model.ChannelConfig{{
		Name: "openai-official", Type: model.ProviderOpenAI, BaseURL: "https://api.openai.com/v1",
		Models: []string{"gpt-x"}, Status: "active", Priority: 1, Weight: 1,
		Protocols: []string{"openai_chat", "anthropic_messages"},
	}})
	req := &model.ChatCompletionRequest{
		Model: "gpt-x", Protocol: "anthropic_messages", AnthropicNativeOnly: true,
		AnthropicOriginal: &model.AnthropicInboundRequest{Model: "gpt-x", Thinking: map[string]any{"type": "enabled"}},
	}
	channels := d.candidateChannelsForRequest(context.Background(), req, "anthropic_messages")
	// No native channel exists, so the documented fallback returns the general channels.
	if len(channels) != 1 || isAnthropicWireChannel(channels[0]) {
		t.Fatalf("candidate %+v must not be classified as a native Anthropic channel", channels)
	}
}

// R-04 guard: the marker must not be confused with the inbound capability token.
func TestInboundCapabilityIsNotNativeMarker(t *testing.T) {
	ch := &model.ChannelConfig{Type: model.ProviderOpenAI, Protocols: []string{"anthropic_messages"}}
	if isAnthropicWireChannel(ch) {
		t.Fatal("anthropic_messages is the inbound capability, not a native wire marker")
	}
	if isAnthropicWireChannel(&model.ChannelConfig{Type: model.ProviderAnthropic}) != true {
		t.Fatal("an anthropic-typed channel is always native")
	}
	if isAnthropicWireChannel(nil) {
		t.Fatal("nil channel must not be native")
	}
}
