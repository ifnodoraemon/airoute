package model

import (
	"encoding/json"
	"testing"
)

func TestToChatCompletionRequestMapsFunctionTools(t *testing.T) {
	body := `{
		"model":"m",
		"input":"hi",
		"tools":[
			{"type":"function","name":"get_weather","description":"Look up weather","parameters":{"type":"object","properties":{"city":{"type":"string"}}},"strict":true},
			{"type":"web_search"}
		],
		"tool_choice":{"type":"function","name":"get_weather"},
		"parallel_tool_calls":false
	}`
	var req ResponseRequest
	if err := json.Unmarshal([]byte(body), &req); err != nil {
		t.Fatal(err)
	}
	chatReq, err := req.ToChatCompletionRequest()
	if err != nil {
		t.Fatal(err)
	}

	if len(chatReq.Tools) != 1 {
		t.Fatalf("expected only the function tool, got %+v", chatReq.Tools)
	}
	tool := chatReq.Tools[0]
	if tool.Type != "function" {
		t.Fatalf("tool type = %q", tool.Type)
	}
	if tool.Function["name"] != "get_weather" || tool.Function["description"] != "Look up weather" {
		t.Fatalf("tool function not mapped: %+v", tool.Function)
	}
	if _, ok := tool.Function["parameters"].(map[string]any); !ok {
		t.Fatalf("parameters not preserved: %+v", tool.Function["parameters"])
	}
	if strict, ok := tool.Function["strict"].(bool); !ok || !strict {
		t.Fatalf("strict not preserved: %+v", tool.Function["strict"])
	}

	choice, ok := chatReq.ToolChoice.(map[string]any)
	if !ok {
		t.Fatalf("tool_choice not mapped: %+v", chatReq.ToolChoice)
	}
	fn, _ := choice["function"].(map[string]any)
	if choice["type"] != "function" || fn["name"] != "get_weather" {
		t.Fatalf("tool_choice shape = %+v", choice)
	}
	if chatReq.ParallelToolCalls == nil || *chatReq.ParallelToolCalls {
		t.Fatalf("parallel_tool_calls not mapped: %+v", chatReq.ParallelToolCalls)
	}
}

func TestToChatCompletionRequestKeepsStringToolChoice(t *testing.T) {
	for _, choice := range []string{"auto", "none", "required"} {
		req := ResponseRequest{
			Model:      "m",
			Input:      "hi",
			ToolChoice: choice,
			Tools:      []any{map[string]any{"type": "function", "name": "f"}},
		}
		chatReq, err := req.ToChatCompletionRequest()
		if err != nil {
			t.Fatal(err)
		}
		if chatReq.ToolChoice != choice {
			t.Fatalf("tool_choice %q became %+v", choice, chatReq.ToolChoice)
		}
	}
}

func TestConvertChatResponseToResponseResultMapsToolCalls(t *testing.T) {
	stop := "tool_calls"
	chatResp := &ChatCompletionResponse{
		ID: "chatcmpl-tool-1", Object: "chat.completion", Created: 1, Model: "m",
		Choices: []ChatCompletionChoice{{
			Index: 0,
			Message: ChatMessage{
				Role:    "assistant",
				Content: nil,
				ToolCalls: []ToolCall{{
					ID:       "call_abc",
					Type:     "function",
					Function: FunctionCall{Name: "get_weather", Arguments: `{"city":"BJ"}`},
				}},
			},
			FinishReason: &stop,
		}},
	}

	result := ConvertChatResponseToResponseResult(chatResp)
	if len(result.Output) != 1 {
		t.Fatalf("pure tool turn must not add an empty message item: %+v", result.Output)
	}
	item := result.Output[0]
	if item.Type != "function_call" || item.Status != "completed" {
		t.Fatalf("unexpected output item: %+v", item)
	}
	if item.CallID != "call_abc" || item.Name != "get_weather" || item.Arguments != `{"city":"BJ"}` {
		t.Fatalf("function_call fields not mapped: %+v", item)
	}
	if item.ID == "" {
		t.Fatal("function_call item has no id")
	}

	encoded, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`"type":"function_call"`, `"call_id":"call_abc"`, `"name":"get_weather"`, `"arguments":"{\"city\":\"BJ\"}"`} {
		if !json.Valid(encoded) || !containsBytes(encoded, want) {
			t.Fatalf("encoded result lost %s: %s", want, encoded)
		}
	}
}

func TestConvertChatResponseToResponseResultKeepsTextAndTools(t *testing.T) {
	stop := "tool_calls"
	chatResp := &ChatCompletionResponse{
		ID: "chatcmpl-tool-2", Object: "chat.completion", Created: 1, Model: "m",
		Choices: []ChatCompletionChoice{{
			Index: 0,
			Message: ChatMessage{
				Role:    "assistant",
				Content: "let me check",
				ToolCalls: []ToolCall{{
					ID:       "call_1",
					Function: FunctionCall{Name: "lookup", Arguments: "{}"},
				}},
			},
			FinishReason: &stop,
		}},
	}
	result := ConvertChatResponseToResponseResult(chatResp)
	if len(result.Output) != 2 {
		t.Fatalf("expected message + function_call, got %+v", result.Output)
	}
	if result.Output[0].Type != "message" || result.Output[0].Content[0].Text != "let me check" {
		t.Fatalf("message item lost text: %+v", result.Output[0])
	}
	if result.Output[1].Type != "function_call" {
		t.Fatalf("function_call missing: %+v", result.Output[1])
	}
}

func containsBytes(haystack []byte, needle string) bool {
	return len(needle) == 0 || jsonValidContains(string(haystack), needle)
}

func jsonValidContains(haystack, needle string) bool {
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if haystack[i:i+len(needle)] == needle {
			return true
		}
	}
	return false
}

// A request whose tools are all built-ins must not forward a tool_choice that
// references a tool the upstream will never see.
func TestToChatCompletionRequestDropsToolChoiceWithoutFunctionTools(t *testing.T) {
	req := ResponseRequest{
		Model:      "m",
		Input:      "hi",
		Tools:      []any{map[string]any{"type": "web_search"}},
		ToolChoice: "required",
	}
	chatReq, err := req.ToChatCompletionRequest()
	if err != nil {
		t.Fatal(err)
	}
	if len(chatReq.Tools) != 0 {
		t.Fatalf("built-in tools must not become chat tools: %+v", chatReq.Tools)
	}
	if chatReq.ToolChoice != nil {
		t.Fatalf("tool_choice must be dropped when no function tools survive: %+v", chatReq.ToolChoice)
	}
}
