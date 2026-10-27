package model

import "fmt"

// responseToolsToChatTools converts Responses-API function tools into chat
// tool definitions.
//
// The Responses wire shape is flat ({"type":"function","name":...,"parameters":
// ...}); chat completions nests the same fields under "function". Built-in
// Responses tools (web_search, file_search, code_interpreter, ...) have no chat
// equivalent and are skipped: the gateway cannot execute them upstream, and
// inventing a function with the same name would misrepresent what happened.
func responseToolsToChatTools(tools []any) []Tool {
	if len(tools) == 0 {
		return nil
	}
	out := make([]Tool, 0, len(tools))
	for _, raw := range tools {
		def, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		if typ, _ := def["type"].(string); typ != "function" {
			continue
		}
		src := def
		// Tolerate the chat-style nested shape some SDKs send by mistake.
		if nested, ok := def["function"].(map[string]any); ok {
			src = nested
		}
		name, _ := src["name"].(string)
		if name == "" {
			continue
		}
		fn := map[string]any{"name": name}
		if description, ok := src["description"]; ok {
			fn["description"] = description
		}
		if parameters, ok := src["parameters"]; ok {
			fn["parameters"] = parameters
		}
		if strict, ok := src["strict"]; ok {
			fn["strict"] = strict
		}
		out = append(out, Tool{Type: "function", Function: fn})
	}
	return out
}

// responseToolChoiceToChatToolChoice translates a Responses tool_choice into the
// chat completions shape. Unknown/unsupported selectors (allowed_tools,
// built-in tool references) become nil, which the OpenAI schema treats as the
// default "auto".
func responseToolChoiceToChatToolChoice(choice any) any {
	switch typed := choice.(type) {
	case string:
		switch typed {
		case "auto", "none", "required":
			return typed
		}
		return nil
	case map[string]any:
		if typ, _ := typed["type"].(string); typ != "function" {
			return nil
		}
		name, _ := typed["name"].(string)
		if name == "" {
			if nested, ok := typed["function"].(map[string]any); ok {
				name, _ = nested["name"].(string)
			}
		}
		if name == "" {
			return nil
		}
		return map[string]any{"type": "function", "function": map[string]any{"name": name}}
	}
	return nil
}

// chatToolCallsToResponseItems renders chat tool calls as Responses
// function_call output items. IDs are deterministic so a retry/replay of the
// same upstream response is stable.
func chatToolCallsToResponseItems(prefix string, calls []ToolCall) []ResponseOutputItem {
	if len(calls) == 0 {
		return nil
	}
	out := make([]ResponseOutputItem, 0, len(calls))
	for i, call := range calls {
		callID := call.ID
		if callID == "" {
			callID = fmt.Sprintf("call_%s_%d", prefix, i)
		}
		out = append(out, ResponseOutputItem{
			ID:        fmt.Sprintf("fc_%s_%d", prefix, i),
			Type:      "function_call",
			Status:    "completed",
			CallID:    callID,
			Name:      call.Function.Name,
			Arguments: call.Function.Arguments,
		})
	}
	return out
}
