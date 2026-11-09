package model

import (
	"encoding/json"
	"fmt"
)

// ResponseInputToChatMessages converts the Responses API `input` field into
// canonical chat-completion messages.
//
// The Responses wire format is an item list, not a chat message list: an input
// turn can be a message carrying content parts (input_text, input_image), the
// function_call the assistant previously emitted, or the function_call_output
// that answers it. Chat completions represents the same history as user/tool
// messages and assistant tool_calls, so every shape has to be translated. When
// the shapes are not understood the item is skipped: forwarding an unknown item
// as an empty user message actively corrupts the conversation (the upstream sees
// a blank turn) instead of merely losing the item.
func ResponseInputToChatMessages(input any) ([]ChatMessage, error) {
	switch typed := input.(type) {
	case nil:
		return nil, nil
	case string:
		return []ChatMessage{{Role: "user", Content: typed}}, nil
	case ChatMessage:
		return responseChatMessageItems([]ChatMessage{typed})
	case []ChatMessage:
		return responseChatMessageItems(typed)
	case []any:
		return responseItemsToChatMessages(typed)
	case []map[string]any:
		items := make([]any, len(typed))
		for i := range typed {
			items[i] = typed[i]
		}
		return responseItemsToChatMessages(items)
	default:
		return nil, fmt.Errorf("field 'input' must be a string or an array of input items, got %T", input)
	}
}

// responseChatMessageItems normalizes already-structured chat messages. They
// still pass through the content-part conversion because a Responses client can
// send chat-shaped messages whose content uses Responses parts.
func responseChatMessageItems(messages []ChatMessage) ([]ChatMessage, error) {
	out := make([]ChatMessage, 0, len(messages))
	for _, message := range messages {
		converted, err := responseMessageToChatMessage(map[string]any{
			"role":         message.Role,
			"content":      message.Content,
			"name":         message.Name,
			"tool_calls":   message.ToolCalls,
			"tool_call_id": message.ToolCallID,
		})
		if err != nil {
			return nil, err
		}
		if converted == nil {
			continue
		}
		out = appendResponseChatMessage(out, *converted)
	}
	return out, nil
}

// responseItemsToChatMessages converts a Responses input item array.
func responseItemsToChatMessages(items []any) ([]ChatMessage, error) {
	out := make([]ChatMessage, 0, len(items))
	for index, raw := range items {
		item, err := responseItemObject(raw)
		if err != nil {
			return nil, fmt.Errorf("invalid 'input' item %d: %w", index, err)
		}
		converted, err := responseItemToChatMessages(item)
		if err != nil {
			return nil, fmt.Errorf("invalid 'input' item %d: %w", index, err)
		}
		for _, message := range converted {
			out = appendResponseChatMessage(out, message)
		}
	}
	return out, nil
}

// responseItemObject normalizes one input item to a JSON object. Items decoded
// from JSON are already map[string]any; typed callers can pass structs.
func responseItemObject(raw any) (map[string]any, error) {
	switch typed := raw.(type) {
	case nil:
		return map[string]any{}, nil
	case map[string]any:
		return typed, nil
	default:
		encoded, err := json.Marshal(typed)
		if err != nil {
			return nil, err
		}
		var object map[string]any
		if err := json.Unmarshal(encoded, &object); err != nil {
			return nil, fmt.Errorf("expected an object, got %T", raw)
		}
		return object, nil
	}
}

// responseItemToChatMessages converts one Responses input item. It returns nil
// for items with no chat representation (reasoning, item_reference, built-in
// tool calls, ...); those must never surface as an empty user message.
func responseItemToChatMessages(item map[string]any) ([]ChatMessage, error) {
	if len(item) == 0 {
		return nil, nil
	}
	itemType, _ := item["type"].(string)
	switch itemType {
	case "function_call":
		return responseFunctionCallItems(item)
	case "function_call_output":
		message, ok := responseFunctionCallOutputMessage(item)
		if !ok {
			return nil, nil
		}
		return []ChatMessage{message}, nil
	case "input_text", "text", "output_text", "input_image", "image_url":
		// Content parts are normally nested inside a message item, but some
		// clients send a bare part as an item. Treat it as a user message part.
		content, ok, err := responseContentToChatContent([]any{item})
		if err != nil || !ok {
			return nil, err
		}
		return []ChatMessage{{Role: "user", Content: content}}, nil
	}

	role, _ := item["role"].(string)
	if role == "" {
		if itemType != "" && itemType != "message" {
			// Unknown Responses item type: skip instead of fabricating a user turn.
			return nil, nil
		}
		_, hasContent := item["content"]
		_, hasText := item["text"].(string)
		if !hasContent && !hasText {
			// An item with neither a role nor content carries nothing a chat
			// request can express.
			return nil, nil
		}
	}
	message, err := responseMessageToChatMessage(item)
	if err != nil || message == nil {
		return nil, err
	}
	return []ChatMessage{*message}, nil
}

// responseMessageToChatMessage converts a message-shaped item (or a plain chat
// message) into a ChatMessage.
func responseMessageToChatMessage(item map[string]any) (*ChatMessage, error) {
	role, _ := item["role"].(string)
	role = normalizeResponseRole(role)

	content, hasContent := item["content"]
	if !hasContent {
		content, hasContent = item["text"]
	}

	var chatContent any
	if hasContent {
		converted, ok, err := responseContentToChatContent(content)
		if err != nil {
			return nil, err
		}
		if ok {
			chatContent = converted
		}
	}

	message := &ChatMessage{Role: role, Content: chatContent}
	if toolCallID, ok := item["tool_call_id"].(string); ok && toolCallID != "" {
		message.ToolCallID = toolCallID
	}
	if name, ok := item["name"].(string); ok && name != "" {
		message.Name = name
	}
	if toolCalls, ok := responseToolCalls(item["tool_calls"]); ok {
		message.ToolCalls = toolCalls
	}
	if message.Content == nil && len(message.ToolCalls) == 0 {
		// Nothing to say: an empty turn would only confuse the upstream.
		return nil, nil
	}
	return message, nil
}
