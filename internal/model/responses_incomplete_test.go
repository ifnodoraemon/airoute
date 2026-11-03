package model

import (
	"encoding/json"
	"strings"
	"testing"
)

// Regression tests for BUG-RESPONSES-INCOMPLETE (fixed).
//
// The Responses API distinguishes a truncated answer (status "incomplete" plus
// incomplete_details.reason) from a normal end. The unary conversion used to
// hard-code status "completed" and drop the chat finish_reason entirely, so an
// agent could not tell a max_output_tokens cut from a complete answer.

// truncatedChatResponse builds a chat response with the given finish_reason.
func truncatedChatResponse(finishReason string) *ChatCompletionResponse {
	finish := finishReason
	return &ChatCompletionResponse{
		ID:      "chatcmpl-abc123",
		Object:  "chat.completion",
		Created: 1,
		Model:   "gpt-x",
		Choices: []ChatCompletionChoice{{
			Index:        0,
			Message:      ChatMessage{Role: "assistant", Content: "The answer is 42 and the next step is"},
			FinishReason: &finish,
		}},
		Usage: &Usage{
			PromptTokens:        10,
			CompletionTokens:    7,
			TotalTokens:         17,
			PromptTokensDetails: &PromptTokensDetails{CachedTokens: 4},
		},
	}
}

// TestResponsesMaxOutputTokensIsIncomplete is the original reproduction: a
// max_tokens cut must be visible as status "incomplete".
func TestResponsesMaxOutputTokensIsIncomplete(t *testing.T) {
	result := ConvertChatResponseToResponseResult(truncatedChatResponse("length"))
	if result == nil {
		t.Fatal("ConvertChatResponseToResponseResult returned nil")
	}
	if result.Status != ResponseStatusIncomplete {
		t.Fatalf("status = %q, want %q for finish_reason=length", result.Status, ResponseStatusIncomplete)
	}
	if result.IncompleteDetails == nil || result.IncompleteDetails.Reason != "max_output_tokens" {
		t.Fatalf("incomplete_details = %+v, want reason=max_output_tokens", result.IncompleteDetails)
	}

	// The truncation signal must survive JSON encoding into the client payload.
	payload, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("marshal response result: %v", err)
	}
	if !strings.Contains(string(payload), `"status":"incomplete"`) {
		t.Fatalf("payload does not report incomplete: %s", payload)
	}
	if !strings.Contains(string(payload), `"incomplete_details":{"reason":"max_output_tokens"}`) {
		t.Fatalf("payload does not carry incomplete_details: %s", payload)
	}
}

// TestResponsesContentFilterIsIncomplete covers the other incomplete reason.
func TestResponsesContentFilterIsIncomplete(t *testing.T) {
	finish := "content_filter"
	chatResp := &ChatCompletionResponse{
		ID:      "chatcmpl-filter",
		Created: 1,
		Model:   "gpt-x",
		Choices: []ChatCompletionChoice{{
			Index:        0,
			Message:      ChatMessage{Role: "assistant", Content: "partially"},
			FinishReason: &finish,
		}},
	}
	result := ConvertChatResponseToResponseResult(chatResp)
	if result.Status != ResponseStatusIncomplete {
		t.Fatalf("status = %q, want incomplete for content_filter", result.Status)
	}
	if result.IncompleteDetails == nil || result.IncompleteDetails.Reason != "content_filter" {
		t.Fatalf("incomplete_details = %+v, want reason=content_filter", result.IncompleteDetails)
	}
}

// TestResponsesNormalCompletionStaysCompleted pins the no-regression case: a
// "stop" answer (and an absent finish_reason) must remain a completed response
// with no incomplete_details at all.
func TestResponsesNormalCompletionStaysCompleted(t *testing.T) {
	result := ConvertChatResponseToResponseResult(truncatedChatResponse("stop"))
	if result.Status != ResponseStatusCompleted {
		t.Fatalf("status = %q, want completed for finish_reason=stop", result.Status)
	}
	if result.IncompleteDetails != nil {
		t.Fatalf("incomplete_details = %+v, want nil for a completed response", result.IncompleteDetails)
	}
	payload, _ := json.Marshal(result)
	if strings.Contains(string(payload), "incomplete") {
		t.Fatalf("completed payload must not mention incomplete details: %s", payload)
	}

	noFinish := truncatedChatResponse("stop")
	noFinish.Choices[0].FinishReason = nil
	if got := ConvertChatResponseToResponseResult(noFinish); got.Status != ResponseStatusCompleted {
		t.Fatalf("status without finish_reason = %q, want completed", got.Status)
	}
}

// TestResponsesUnaryUsageCarriesCacheDetails pins the unary/streaming shape
// parity: the streaming path always sent input_tokens_details, the unary path
// dropped the cached-token breakdown.
func TestResponsesUnaryUsageCarriesCacheDetails(t *testing.T) {
	result := ConvertChatResponseToResponseResult(truncatedChatResponse("stop"))
	if result.Usage.InputTokensDetails == nil {
		t.Fatal("unary response usage is missing input_tokens_details (streaming path sends it)")
	}
	if got := result.Usage.InputTokensDetails.CachedTokens; got != 4 {
		t.Fatalf("input_tokens_details.cached_tokens = %d, want 4", got)
	}
	if result.Usage.OutputTokensDetails == nil {
		t.Fatal("unary response usage is missing output_tokens_details (streaming path sends it)")
	}
	payload, _ := json.Marshal(result.Usage)
	if !strings.Contains(string(payload), `"input_tokens_details":{"cached_tokens":4}`) {
		t.Fatalf("usage payload missing cached breakdown: %s", payload)
	}
}
