package model

// ChatProviderType selects the upstream wire protocol for this request. A relay
// advertising both OpenAI and native Messages must keep OpenAI extensions on
// its OpenAI endpoint, while native Messages requests retain their original wire.
// A nil request preserves the channel's legacy native-Messages preference.
func ChatProviderType(channel *ChannelConfig, request *ChatCompletionRequest) ProviderType {
	if channel.Type == ProviderAnthropic {
		return ProviderAnthropic
	}
	nativeMessages, openAI := false, false
	for _, protocol := range channel.Protocols {
		switch protocol {
		case "anthropic_native":
			nativeMessages = true
		case "*", "chat", "openai_chat", "response", "responses", "openai_response", "openai_responses":
			openAI = true
		}
	}
	if nativeMessages {
		if request == nil || !openAI || request.AnthropicNativeOnly || request.Protocol == "anthropic_messages" || request.AnthropicOriginal != nil || (request.Native != nil && request.Native.Protocol == NativeMessages) {
			return ProviderAnthropic
		}
	}
	return channel.Type
}
