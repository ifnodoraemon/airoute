package model

import (
	"strings"
)

// MergeResponseConversation prepends a stored previous-response conversation
// after the new request's leading system instructions and before its new input.
//
// System messages are dropped from the stored side: the caller's new
// `instructions` are authoritative for this turn, and replaying an old system
// prompt after them would silently override it.
func MergeResponseConversation(newMessages, previous []ChatMessage) []ChatMessage {
	if len(previous) == 0 {
		return newMessages
	}
	merged := make([]ChatMessage, 0, len(newMessages)+len(previous))
	leading := 0
	for leading < len(newMessages) && newMessages[leading].Role == "system" {
		merged = append(merged, newMessages[leading])
		leading++
	}
	for _, message := range previous {
		if message.Role == "system" {
			continue
		}
		merged = append(merged, message)
	}
	merged = append(merged, newMessages[leading:]...)
	return merged
}

// ResponseConversationMessages builds the message list persisted for a
// completed response: the non-system messages that produced it plus the
// assistant turn itself. Storing system instructions is deliberately avoided so
// a later request's `instructions` cannot be silently overridden by replaying
// the old prompt.
func ResponseConversationMessages(conversation []ChatMessage, assistant *ChatMessage) []ChatMessage {
	out := make([]ChatMessage, 0, len(conversation)+1)
	for _, message := range conversation {
		if message.Role == "system" {
			continue
		}
		out = append(out, message)
	}
	if assistant != nil {
		out = append(out, *assistant)
	}
	return out
}

// AssistantMessageFromChatResponse extracts the assistant turn of the first
// choice so it can be stored as conversation context.
func AssistantMessageFromChatResponse(resp *ChatCompletionResponse) *ChatMessage {
	if resp == nil || len(resp.Choices) == 0 {
		return nil
	}
	source := resp.Choices[0].Message
	message := &ChatMessage{Role: "assistant"}
	if text, ok := source.Content.(string); ok {
		if strings.TrimSpace(text) != "" {
			message.Content = text
		}
	} else if source.Content != nil {
		message.Content = source.Content
	}
	if len(source.ToolCalls) > 0 {
		message.ToolCalls = source.ToolCalls
	}
	if message.Content == nil && len(message.ToolCalls) == 0 {
		return nil
	}
	return message
}
