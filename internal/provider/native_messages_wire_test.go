package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ifnodoraemon/airoute/internal/model"
)

// The inbound capability "anthropic_messages" (and the wildcard) only advertise
// that a channel serves /v1/messages traffic; they must not be mistaken for an
// Anthropic-speaking upstream. An OpenAI/Gemini channel must translate, never
// forward Anthropic JSON to <base>/v1/messages (R-04 L2, mirrors L1).
func TestNativeMessagesRequiresAnthropicWire(t *testing.T) {
	req := &model.ChatCompletionRequest{Native: &model.NativeRequest{Protocol: model.NativeMessages, Body: json.RawMessage(`{}`)}}
	cases := []struct {
		name      string
		kind      model.ProviderType
		protocols []string
		want      bool
	}{
		{"default openai channel", model.ProviderOpenAI, []string{"openai_chat", "openai_response", "anthropic_messages", "embeddings"}, false},
		{"wildcard openai channel", model.ProviderOpenAI, []string{"*"}, false},
		{"empty openai channel", model.ProviderOpenAI, nil, false},
		{"messages token only", model.ProviderOpenAI, []string{"messages"}, false},
		{"explicit native marker", model.ProviderOpenAI, []string{"openai_chat", "anthropic_native"}, true},
		{"anthropic type", model.ProviderAnthropic, []string{"openai_chat"}, true},
		{"gemini channel", model.ProviderGemini, []string{"anthropic_messages"}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ch := &model.ChannelConfig{Type: tc.kind, Protocols: tc.protocols}
			if got := nativeMatches(req, ch, model.NativeMessages); got != tc.want {
				t.Fatalf("nativeMessages=%v want %v (protocols=%v)", got, tc.want, tc.protocols)
			}
		})
	}
}

func TestAnthropicInboundTranslatesOnOpenAIChannel(t *testing.T) {
	var gotPath string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		fmt.Fprint(w, `{"id":"c","model":"m","choices":[{"message":{"role":"assistant","content":"ok"}}],"usage":{"prompt_tokens":1,"completion_tokens":1}}`)
	}))
	defer server.Close()
	ch := &model.ChannelConfig{Name: "a", Type: model.ProviderOpenAI, BaseURL: server.URL, Protocols: []string{"openai_chat", "openai_response", "anthropic_messages", "embeddings"}}
	req := &model.ChatCompletionRequest{
		Model:    "m",
		Messages: []model.ChatMessage{{Role: "user", Content: "hi"}},
		Native:   &model.NativeRequest{Protocol: model.NativeMessages, Body: json.RawMessage(`{"model":"m","max_tokens":64,"messages":[{"role":"user","content":"hi"}]}`)},
	}
	if _, err := NewOpenAIProvider(server.Client()).ChatComplete(context.Background(), req, ch); err != nil {
		t.Fatal(err)
	}
	if gotPath != "/v1/chat/completions" {
		t.Fatalf("Anthropic inbound mis-forwarded to %q; want translated /v1/chat/completions", gotPath)
	}
}

func TestAnthropicNativeMarkerStillPassthrough(t *testing.T) {
	var gotPath string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		fmt.Fprint(w, `{"id":"a","model":"m","content":[{"type":"text","text":"ok"}],"usage":{"input_tokens":1,"output_tokens":1}}`)
	}))
	defer server.Close()
	ch := &model.ChannelConfig{Name: "a", Type: model.ProviderOpenAI, BaseURL: server.URL, Protocols: []string{"openai_chat", "anthropic_native"}}
	req := &model.ChatCompletionRequest{
		Model:  "m",
		Native: &model.NativeRequest{Protocol: model.NativeMessages, Body: json.RawMessage(`{"model":"m","max_tokens":64,"messages":[{"role":"user","content":"hi"}]}`)},
	}
	if _, err := NewOpenAIProvider(server.Client()).ChatComplete(context.Background(), req, ch); err != nil {
		t.Fatal(err)
	}
	if gotPath != "/v1/messages" {
		t.Fatalf("explicit anthropic_native marker should passthrough to /v1/messages, got %q", gotPath)
	}
}