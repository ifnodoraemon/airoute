package api

import (
	"strings"
	"testing"
)

// TestResponsesStreamEmitsReasoningBeforeMessage drives the adapted Responses
// stream with an upstream that thinks before answering. The reasoning output item
// must precede the message item (output_index 0 vs 1), stream the reasoning text
// as reasoning_summary_text deltas, and appear first in response.completed.
func TestResponsesStreamEmitsReasoningBeforeMessage(t *testing.T) {
	events := runResponsesStreamProbe(t, `data: {"id":"c1","choices":[{"index":0,"delta":{"reasoning_content":"Let me think."},"finish_reason":null}]}

data: {"id":"c1","choices":[{"index":0,"delta":{"content":"The answer."},"finish_reason":null}]}

data: {"id":"c1","choices":[{"index":0,"delta":{},"finish_reason":"stop"}]}

data: {"id":"c1","choices":[],"usage":{"prompt_tokens":5,"completion_tokens":8,"total_tokens":13,"completion_tokens_details":{"reasoning_tokens":4}}}

data: [DONE]

`)

	var reasoningAddedIndex, messageAddedIndex = -1, -1
	var reasoningDeltas []string
	for _, ev := range events {
		switch ev.Event {
		case "response.output_item.added":
			item, _ := ev.Data["item"].(map[string]any)
			idx := int(ev.Data["output_index"].(float64))
			switch item["type"] {
			case "reasoning":
				reasoningAddedIndex = idx
			case "message":
				messageAddedIndex = idx
			}
		case "response.reasoning_summary_text.delta":
			delta, _ := ev.Data["delta"].(string)
			reasoningDeltas = append(reasoningDeltas, delta)
		}
	}

	if reasoningAddedIndex != 0 {
		t.Fatalf("reasoning output_index = %d, want 0 (reasoning precedes message)", reasoningAddedIndex)
	}
	if messageAddedIndex != 1 {
		t.Fatalf("message output_index = %d, want 1 (after reasoning)", messageAddedIndex)
	}
	if got := strings.Join(reasoningDeltas, ""); got != "Let me think." {
		t.Fatalf("reasoning deltas = %q, want %q", got, "Let me think.")
	}

	response := terminalResponse(events)
	if response == nil {
		t.Fatal("no response.completed event")
	}
	output, _ := response["output"].([]any)
	if len(output) != 2 {
		t.Fatalf("completed output = %d items, want reasoning + message: %+v", len(output), output)
	}
	first, _ := output[0].(map[string]any)
	second, _ := output[1].(map[string]any)
	if first["type"] != "reasoning" || second["type"] != "message" {
		t.Fatalf("output order = [%v, %v], want [reasoning, message]", first["type"], second["type"])
	}
	summary, _ := first["summary"].([]any)
	if len(summary) != 1 {
		t.Fatalf("reasoning summary = %+v, want one summary_text part", summary)
	}
	part, _ := summary[0].(map[string]any)
	if part["text"] != "Let me think." {
		t.Fatalf("reasoning summary text = %v, want assembled reasoning", part["text"])
	}
	usage, _ := response["usage"].(map[string]any)
	details, _ := usage["output_tokens_details"].(map[string]any)
	if details["reasoning_tokens"] != float64(4) {
		t.Fatalf("reasoning_tokens = %v, want 4", details["reasoning_tokens"])
	}
}

// TestResponsesStreamWithoutReasoningKeepsMessageAtZero pins the no-regression
// case: a plain answer still holds output_index 0 and produces a single message
// output item (the lazy allocation must not shift the index when nothing thinks).
func TestResponsesStreamWithoutReasoningKeepsMessageAtZero(t *testing.T) {
	events := runResponsesStreamProbe(t, `data: {"id":"c1","choices":[{"index":0,"delta":{"content":"hi"},"finish_reason":"stop"}]}

data: {"id":"c1","choices":[],"usage":{"prompt_tokens":3,"completion_tokens":1,"total_tokens":4}}

data: [DONE]

`)
	for _, ev := range events {
		if ev.Event == "response.output_item.added" {
			item, _ := ev.Data["item"].(map[string]any)
			if item["type"] == "message" {
				if idx := int(ev.Data["output_index"].(float64)); idx != 0 {
					t.Fatalf("message output_index = %d, want 0 when no reasoning", idx)
				}
			}
			if item["type"] == "reasoning" {
				t.Fatalf("unexpected reasoning item for a non-thinking stream")
			}
		}
	}
	response := terminalResponse(events)
	output, _ := response["output"].([]any)
	if len(output) != 1 {
		t.Fatalf("completed output = %d items, want a single message", len(output))
	}
}