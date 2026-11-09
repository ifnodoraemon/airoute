package model

import (
	"encoding/json"
	"strings"
)

// responseFunctionCallItems converts a Responses function_call item into an
// assistant tool_calls turn.
func responseFunctionCallItems(item map[string]any) ([]ChatMessage, error) {
	name, _ := item["name"].(string)
	callID, _ := item["call_id"].(string)
	if callID == "" {
		// SDKs sometimes round-trip the Responses item id instead of call_id.
		callID, _ = item["id"].(string)
	}
	arguments, ok := responseArgumentsString(item["arguments"])
	if name == "" || callID == "" || !ok {
		// Without a name/call_id the call cannot be referenced by its output.
		return nil, nil
	}
	return []ChatMessage{{
		Role: "assistant",
		ToolCalls: []ToolCall{{
			ID:       callID,
			Type:     "function",
			Function: FunctionCall{Name: name, Arguments: arguments},
		}},
	}}, nil
}

// responseFunctionCallOutputMessage converts a function_call_output item into a
// chat tool message. The output can be a string or any JSON value (object/array
// for structured tool results), which becomes the tool message content.
func responseFunctionCallOutputMessage(item map[string]any) (ChatMessage, bool) {
	callID, _ := item["call_id"].(string)
	if callID == "" {
		return ChatMessage{}, false
	}
	content := ""
	switch output := item["output"].(type) {
	case nil:
		// Empty output is still a completed tool call.
	case string:
		content = output
	default:
		encoded, err := json.Marshal(output)
		if err != nil {
			return ChatMessage{}, false
		}
		content = string(encoded)
	}
	return ChatMessage{Role: "tool", ToolCallID: callID, Content: content}, true
}

// responseArgumentsString renders a function_call `arguments` value as the JSON
// string chat completions require.
func responseArgumentsString(raw any) (string, bool) {
	switch typed := raw.(type) {
	case nil:
		return "{}", true
	case string:
		return typed, true
	default:
		encoded, err := json.Marshal(typed)
		if err != nil {
			return "", false
		}
		return string(encoded), true
	}
}

// responseToolCalls decodes chat-style tool_calls echoed inside an input item.
func responseToolCalls(raw any) ([]ToolCall, bool) {
	if raw == nil {
		return nil, false
	}
	encoded, err := json.Marshal(raw)
	if err != nil {
		return nil, false
	}
	var calls []ToolCall
	if err := json.Unmarshal(encoded, &calls); err != nil || len(calls) == 0 {
		return nil, false
	}
	return calls, true
}

// normalizeResponseRole maps Responses roles onto chat roles. "developer" is the
// newer name for the system turn and is not accepted by every chat upstream.
func normalizeResponseRole(role string) string {
	switch strings.ToLower(strings.TrimSpace(role)) {
	case "", "user":
		return "user"
	case "developer", "system":
		return "system"
	default:
		return strings.ToLower(strings.TrimSpace(role))
	}
}

// appendResponseChatMessage appends one converted message, coalescing
// consecutive function_call items into a single assistant tool_calls turn. Chat
// completions expects every call of a turn in one assistant message before the
// tool results that answer them.
func appendResponseChatMessage(out []ChatMessage, message ChatMessage) []ChatMessage {
	if len(out) == 0 || message.Role != "assistant" || len(message.ToolCalls) == 0 || message.Content != nil {
		return append(out, message)
	}
	previous := &out[len(out)-1]
	if previous.Role == "assistant" && previous.Content == nil && len(previous.ToolCalls) > 0 {
		previous.ToolCalls = append(previous.ToolCalls, message.ToolCalls...)
		return out
	}
	return append(out, message)
}
