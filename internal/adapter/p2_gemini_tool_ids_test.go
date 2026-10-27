package adapter

import (
	"context"
	"strings"
	"testing"

	"github.com/ifnodoraemon/airoute/internal/provider"
)

// R-11: Gemini functionResponse turns became role=tool messages with an empty
// tool_call_id, so a follow-up tool turn could not be replayed to an OpenAI or
// Anthropic upstream (both require the id of the assistant call).
func TestP2GeminiFunctionResponseKeepsToolCallID(t *testing.T) {
	req := &provider.GeminiRequest{
		Contents: []provider.GeminiContent{
			{Role: "user", Parts: []provider.GeminiPart{{Text: "weather?"}}},
			{Role: "model", Parts: []provider.GeminiPart{{FunctionCall: &provider.GeminiFunctionCall{
				Name: "get_weather", Args: map[string]any{"city": "x"},
			}}}},
			{Role: "user", Parts: []provider.GeminiPart{{FunctionResponse: &provider.GeminiFunctionResponse{
				Name: "get_weather", Response: map[string]any{"temp": 21},
			}}}},
		},
	}
	canonical, err := NewGeminiAdapter().ToCanonical(context.Background(), req)
	if err != nil {
		t.Fatalf("ToCanonical: %v", err)
	}
	if len(canonical.Messages) < 3 {
		t.Fatalf("expected 3 canonical messages, got %d", len(canonical.Messages))
	}
	assistant := canonical.Messages[1]
	if len(assistant.ToolCalls) != 1 || assistant.ToolCalls[0].ID == "" {
		t.Fatalf("assistant tool call has no id: %+v", assistant.ToolCalls)
	}
	callID := assistant.ToolCalls[0].ID
	toolMsg := canonical.Messages[2]
	if toolMsg.Role != "tool" {
		t.Fatalf("response message role = %q, want tool", toolMsg.Role)
	}
	if toolMsg.ToolCallID != callID {
		t.Fatalf("tool message tool_call_id = %q, want the assistant call id %q", toolMsg.ToolCallID, callID)
	}
	if toolMsg.Name != "get_weather" {
		t.Fatalf("tool message name = %q, want get_weather", toolMsg.Name)
	}
	content, _ := toolMsg.Content.(string)
	if !strings.Contains(content, `"temp":21`) {
		t.Fatalf("tool result content = %q, want the JSON response", content)
	}
}

// Repeated calls to the same tool must keep distinct ids and each response must
// answer its own call, in order.
func TestP2GeminiRepeatedToolCallsKeepDistinctIDs(t *testing.T) {
	req := &provider.GeminiRequest{
		Contents: []provider.GeminiContent{
			{Role: "user", Parts: []provider.GeminiPart{{Text: "compare"}}},
			{Role: "model", Parts: []provider.GeminiPart{
				{FunctionCall: &provider.GeminiFunctionCall{Name: "lookup", Args: map[string]any{"q": "a"}}},
				{FunctionCall: &provider.GeminiFunctionCall{Name: "lookup", Args: map[string]any{"q": "b"}}},
			}},
			{Role: "user", Parts: []provider.GeminiPart{
				{FunctionResponse: &provider.GeminiFunctionResponse{Name: "lookup", Response: map[string]any{"r": "A"}}},
				{FunctionResponse: &provider.GeminiFunctionResponse{Name: "lookup", Response: map[string]any{"r": "B"}}},
			}},
		},
	}
	canonical, err := NewGeminiAdapter().ToCanonical(context.Background(), req)
	if err != nil {
		t.Fatalf("ToCanonical: %v", err)
	}
	assistant := canonical.Messages[1]
	if len(assistant.ToolCalls) != 2 {
		t.Fatalf("expected 2 tool calls, got %d", len(assistant.ToolCalls))
	}
	first, second := assistant.ToolCalls[0].ID, assistant.ToolCalls[1].ID
	if first == "" || second == "" || first == second {
		t.Fatalf("tool call ids are not distinct: %q / %q", first, second)
	}
	if got := canonical.Messages[2]; got.ToolCallID != first {
		t.Fatalf("first response answered %q, want %q", got.ToolCallID, first)
	}
	if got := canonical.Messages[3]; got.ToolCallID != second {
		t.Fatalf("second response answered %q, want %q", got.ToolCallID, second)
	}
}
