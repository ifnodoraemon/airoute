package model

import (
	"encoding/json"
	"reflect"
	"testing"
)

// decodeResponseRequest unmarshals a Responses payload the way the handler does.
func decodeResponseRequest(t *testing.T, body string) ResponseRequest {
	t.Helper()
	var req ResponseRequest
	if err := json.Unmarshal([]byte(body), &req); err != nil {
		t.Fatalf("unmarshal request: %v", err)
	}
	return req
}

// TestResponseInputConvertsResponsesItems is the conversion matrix for the
// Responses input shapes that used to be dropped or turned into empty user
// messages (findings #5/#8).
func TestResponseInputConvertsResponsesItems(t *testing.T) {
	imagePart := map[string]any{
		"type":      "image_url",
		"image_url": map[string]any{"url": "https://example.invalid/cat.png", "detail": "high"},
	}
	cases := []struct {
		name  string
		input string
		want  []ChatMessage
	}{
		{
			name:  "plain string input",
			input: `"hello"`,
			want:  []ChatMessage{{Role: "user", Content: "hello"}},
		},
		{
			name:  "message with input_text parts collapses to a string",
			input: `[{"type":"message","role":"user","content":[{"type":"input_text","text":"a"},{"type":"input_text","text":"b"}]}]`,
			want:  []ChatMessage{{Role: "user", Content: "a\nb"}},
		},
		{
			name:  "message with mixed text and image keeps chat parts",
			input: `[{"type":"message","role":"user","content":[{"type":"input_text","text":"what is this"},{"type":"input_image","image_url":"https://example.invalid/cat.png","detail":"high"}]}]`,
			want: []ChatMessage{{Role: "user", Content: []any{
				map[string]any{"type": "text", "text": "what is this"},
				imagePart,
			}}},
		},
		{
			name:  "nested image_url object is accepted",
			input: `[{"type":"message","role":"user","content":[{"type":"input_image","image_url":{"url":"https://example.invalid/cat.png","detail":"low"}}]}]`,
			want: []ChatMessage{{Role: "user", Content: []any{
				map[string]any{"type": "image_url", "image_url": map[string]any{"url": "https://example.invalid/cat.png", "detail": "low"}},
			}}},
		},
		{
			name:  "function_call_output becomes a tool message",
			input: `[{"type":"function_call_output","call_id":"call_1","output":"sunny"}]`,
			want:  []ChatMessage{{Role: "tool", ToolCallID: "call_1", Content: "sunny"}},
		},
		{
			name:  "structured function_call_output becomes JSON text",
			input: `[{"type":"function_call_output","call_id":"call_1","output":{"temp":21,"unit":"C"}}]`,
			want:  []ChatMessage{{Role: "tool", ToolCallID: "call_1", Content: `{"temp":21,"unit":"C"}`}},
		},
		{
			name:  "function_call becomes an assistant tool_calls turn",
			input: `[{"type":"function_call","call_id":"call_1","name":"get_weather","arguments":"{\"city\":\"BJ\"}"}]`,
			want: []ChatMessage{{Role: "assistant", ToolCalls: []ToolCall{{
				ID: "call_1", Type: "function",
				Function: FunctionCall{Name: "get_weather", Arguments: `{"city":"BJ"}`},
			}}}},
		},
		{
			name:  "consecutive function_calls coalesce into one assistant turn",
			input: `[{"type":"function_call","call_id":"call_1","name":"a","arguments":"{}"},{"type":"function_call","call_id":"call_2","name":"b","arguments":"{}"}]`,
			want: []ChatMessage{{Role: "assistant", ToolCalls: []ToolCall{
				{ID: "call_1", Type: "function", Function: FunctionCall{Name: "a", Arguments: "{}"}},
				{ID: "call_2", Type: "function", Function: FunctionCall{Name: "b", Arguments: "{}"}},
			}}},
		},
		{
			name: "call + output round trip preserves the tool linkage",
			input: `[{"type":"function_call","call_id":"call_1","name":"lookup","arguments":"{}"},` +
				`{"type":"function_call_output","call_id":"call_1","output":"42"}]`,
			want: []ChatMessage{
				{Role: "assistant", ToolCalls: []ToolCall{{ID: "call_1", Type: "function", Function: FunctionCall{Name: "lookup", Arguments: "{}"}}}},
				{Role: "tool", ToolCallID: "call_1", Content: "42"},
			},
		},
		{
			name:  "chat-style message item is preserved",
			input: `[{"role":"user","content":"chat style"}]`,
			want:  []ChatMessage{{Role: "user", Content: "chat style"}},
		},
		{
			name:  "developer role maps to system",
			input: `[{"type":"message","role":"developer","content":"be nice"}]`,
			want:  []ChatMessage{{Role: "system", Content: "be nice"}},
		},
		{
			name: "unknown items are skipped, not turned into empty user messages",
			input: `[{"type":"reasoning","summary":[]},{"type":"item_reference","id":"msg_1"},` +
				`{"type":"web_search_call","status":"completed"},{"type":"message","role":"user","content":[{"type":"input_text","text":"still here"}]}]`,
			want: []ChatMessage{{Role: "user", Content: "still here"}},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := decodeResponseRequest(t, `{"model":"m","input":`+tc.input+`}`)
			got, err := req.ToChatCompletionRequest()
			if err != nil {
				t.Fatalf("ToChatCompletionRequest: %v", err)
			}
			if !reflect.DeepEqual(got.Messages, tc.want) {
				t.Fatalf("messages = %#v, want %#v", got.Messages, tc.want)
			}
		})
	}
}

// An input composed only of items with no chat representation must be rejected
// with a clear error rather than forwarded as an empty turn.
func TestResponseInputRejectsUnknownOnlyInput(t *testing.T) {
	req := decodeResponseRequest(t, `{"model":"m","input":[{"type":"reasoning","summary":[]},{"type":"item_reference","id":"msg_1"}]}`)
	if _, err := req.ToChatCompletionRequest(); err == nil {
		t.Fatal("expected an error for input made only of non-chat items")
	}
}

// TestResponseInputKeepsInstructionsFirst pins the ordering contract: the new
// system instructions stay first, before any converted input.
func TestResponseInputKeepsInstructionsFirst(t *testing.T) {
	req := decodeResponseRequest(t, `{"model":"m","instructions":"be brief","input":[{"type":"message","role":"user","content":[{"type":"input_text","text":"hi"}]}]}`)
	chatReq, err := req.ToChatCompletionRequest()
	if err != nil {
		t.Fatal(err)
	}
	want := []ChatMessage{
		{Role: "system", Content: "be brief"},
		{Role: "user", Content: "hi"},
	}
	if !reflect.DeepEqual(chatReq.Messages, want) {
		t.Fatalf("messages = %#v, want %#v", chatReq.Messages, want)
	}
}

// TestResponseInputRejectsStructurallyInvalidInput keeps a malformed item a
// client error instead of silently dropping it.
func TestResponseInputRejectsStructurallyInvalidInput(t *testing.T) {
	req := ResponseRequest{Model: "m", Input: 42}
	if _, err := req.ToChatCompletionRequest(); err == nil {
		t.Fatal("expected an error for a non-string, non-array input")
	}
}

// TestMergeResponseConversationOrdering covers the previous_response_id merge
// contract: new instructions first, then the stored non-system conversation,
// then the new input.
func TestMergeResponseConversationOrdering(t *testing.T) {
	newMessages := []ChatMessage{
		{Role: "system", Content: "new instructions"},
		{Role: "user", Content: "new question"},
	}
	previous := []ChatMessage{
		{Role: "system", Content: "stale instructions"},
		{Role: "user", Content: "old question"},
		{Role: "assistant", Content: "old answer"},
	}
	got := MergeResponseConversation(newMessages, previous)
	want := []ChatMessage{
		{Role: "system", Content: "new instructions"},
		{Role: "user", Content: "old question"},
		{Role: "assistant", Content: "old answer"},
		{Role: "user", Content: "new question"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("merged = %#v, want %#v", got, want)
	}

	if got := MergeResponseConversation(newMessages, nil); !reflect.DeepEqual(got, newMessages) {
		t.Fatalf("merge with no previous context changed messages: %#v", got)
	}
}

// TestResponseConversationMessagesForStorage pins what is persisted: the
// non-system conversation plus the assistant turn, never system instructions.
func TestResponseConversationMessagesForStorage(t *testing.T) {
	conversation := []ChatMessage{
		{Role: "system", Content: "instructions"},
		{Role: "user", Content: "hi"},
		{Role: "assistant", ToolCalls: []ToolCall{{ID: "call_1", Type: "function", Function: FunctionCall{Name: "f", Arguments: "{}"}}}},
		{Role: "tool", ToolCallID: "call_1", Content: "out"},
	}
	assistant := &ChatMessage{Role: "assistant", Content: "done"}
	got := ResponseConversationMessages(conversation, assistant)
	want := []ChatMessage{
		{Role: "user", Content: "hi"},
		{Role: "assistant", ToolCalls: []ToolCall{{ID: "call_1", Type: "function", Function: FunctionCall{Name: "f", Arguments: "{}"}}}},
		{Role: "tool", ToolCallID: "call_1", Content: "out"},
		{Role: "assistant", Content: "done"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("stored conversation = %#v, want %#v", got, want)
	}
}

func TestAssistantMessageFromChatResponse(t *testing.T) {
	if got := AssistantMessageFromChatResponse(nil); got != nil {
		t.Fatalf("nil response must not produce a message: %#v", got)
	}
	empty := &ChatCompletionResponse{Choices: []ChatCompletionChoice{{}}}
	if got := AssistantMessageFromChatResponse(empty); got != nil {
		t.Fatalf("empty choice must not produce a message: %#v", got)
	}

	text := &ChatCompletionResponse{Choices: []ChatCompletionChoice{{
		Message: ChatMessage{Role: "assistant", Content: "hello"},
	}}}
	if got := AssistantMessageFromChatResponse(text); got == nil || got.Content != "hello" {
		t.Fatalf("assistant text not extracted: %#v", got)
	}

	tool := &ChatCompletionResponse{Choices: []ChatCompletionChoice{{
		Message: ChatMessage{Role: "assistant", ToolCalls: []ToolCall{{
			ID: "call_1", Type: "function", Function: FunctionCall{Name: "f", Arguments: "{}"},
		}}},
	}}}
	got := AssistantMessageFromChatResponse(tool)
	if got == nil || len(got.ToolCalls) != 1 || got.ToolCalls[0].ID != "call_1" {
		t.Fatalf("assistant tool calls not extracted: %#v", got)
	}
}

// The persisted conversation must be JSON-encodable in the chat shape, so the
// next request can replay it verbatim.
func TestResponseConversationMessagesMarshalAsChat(t *testing.T) {
	messages := ResponseConversationMessages(
		[]ChatMessage{{Role: "user", Content: "hi"}},
		&ChatMessage{Role: "assistant", ToolCalls: []ToolCall{{ID: "call_1", Type: "function", Function: FunctionCall{Name: "f", Arguments: "{}"}}}},
	)
	encoded, err := json.Marshal(messages)
	if err != nil {
		t.Fatalf("marshal conversation: %v", err)
	}
	var decoded []ChatMessage
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		t.Fatalf("unmarshal conversation: %v", err)
	}
	if !reflect.DeepEqual(decoded, messages) {
		t.Fatalf("conversation round trip changed messages: %#v vs %#v", decoded, messages)
	}
}
