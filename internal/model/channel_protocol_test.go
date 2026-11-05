package model

import "testing"

func TestChatProviderTypeForMultiProtocolRelay(t *testing.T) {
	relay := ChannelConfig{Type: ProviderOpenAI, Protocols: []string{"openai_chat", "openai_response", "anthropic_messages", "anthropic_native"}}
	for _, test := range []struct {
		name string
		req  *ChatCompletionRequest
		want ProviderType
	}{
		{"legacy channel preference", nil, ProviderAnthropic},
		{"canonical chat", &ChatCompletionRequest{}, ProviderOpenAI},
		{"native chat", &ChatCompletionRequest{Native: &NativeRequest{Protocol: NativeChat}}, ProviderOpenAI},
		{"native responses", &ChatCompletionRequest{Native: &NativeRequest{Protocol: NativeResponses}}, ProviderOpenAI},
		{"native messages", &ChatCompletionRequest{Native: &NativeRequest{Protocol: NativeMessages}}, ProviderAnthropic},
		{"native-only features", &ChatCompletionRequest{AnthropicNativeOnly: true}, ProviderAnthropic},
		{"translated messages provenance", &ChatCompletionRequest{Protocol: "anthropic_messages"}, ProviderAnthropic},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := ChatProviderType(&relay, test.req); got != test.want {
				t.Fatalf("provider = %s, want %s", got, test.want)
			}
		})
	}
	relay.Protocols = []string{"anthropic_messages", "anthropic_native"}
	if ChatProviderType(&relay, &ChatCompletionRequest{}) != ProviderAnthropic {
		t.Fatal("a relay without a declared OpenAI endpoint must retain Messages transport")
	}
	relay.Type = ProviderAnthropic
	relay.Protocols = []string{"openai_chat"}
	if ChatProviderType(&relay, &ChatCompletionRequest{}) != ProviderAnthropic {
		t.Fatal("an Anthropic provider must retain its own transport")
	}
}
