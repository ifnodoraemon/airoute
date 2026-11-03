package model

import (
	"encoding/json"
	"testing"
)

// F-02 regression: a reasoning upstream reports the reasoning share under
// completion_tokens_details.reasoning_tokens. The canonical Usage keeps it in
// Extra, and the Responses conversion must surface it instead of hardcoding 0.
func TestResponsesReportsUpstreamReasoningTokens(t *testing.T) {
	body := `{"id":"chatcmpl-r1","object":"chat.completion","created":1,"model":"deepseek-r1",` +
		`"choices":[{"index":0,"message":{"role":"assistant","content":"answer"},"finish_reason":"stop"}],` +
		`"usage":{"prompt_tokens":10,"completion_tokens":120,"total_tokens":130,` +
		`"completion_tokens_details":{"reasoning_tokens":99}}}`

	var resp ChatCompletionResponse
	if err := json.Unmarshal([]byte(body), &resp); err != nil {
		t.Fatalf("unmarshal upstream response: %v", err)
	}
	result := ConvertChatResponseToResponseResult(&resp)
	if result == nil || result.Usage.OutputTokensDetails == nil {
		t.Fatal("Responses usage has no output_tokens_details")
	}
	if got := result.Usage.OutputTokensDetails.ReasoningTokens; got != 99 {
		t.Fatalf("reasoning_tokens = %d, want 99", got)
	}

	// CompletionTokens already includes the reasoning share: surfacing the
	// breakdown must not double count.
	if result.Usage.OutputTokens != 120 {
		t.Fatalf("output_tokens = %d, want 120", result.Usage.OutputTokens)
	}
}

// An upstream that reports no reasoning detail must still produce 0, not a
// bogus number.
func TestResponsesReasoningTokensAbsentIsZero(t *testing.T) {
	resp := &ChatCompletionResponse{
		ID:    "chatcmpl-plain",
		Model: "gpt-x",
		Choices: []ChatCompletionChoice{{
			Index:   0,
			Message: ChatMessage{Role: "assistant", Content: "hi"},
		}},
		Usage: &Usage{PromptTokens: 1, CompletionTokens: 2, TotalTokens: 3},
	}
	result := ConvertChatResponseToResponseResult(resp)
	if result.Usage.OutputTokensDetails == nil {
		t.Fatal("missing output_tokens_details")
	}
	if got := result.Usage.OutputTokensDetails.ReasoningTokens; got != 0 {
		t.Fatalf("reasoning_tokens = %d, want 0", got)
	}
	if got := resp.Usage.GetReasoningTokens(); got != 0 {
		t.Fatalf("GetReasoningTokens on a usage without details = %d, want 0", got)
	}
}

// GetReasoningTokens must survive a malformed detail object without panicking.
func TestUsageGetReasoningTokensHandlesMalformedExtra(t *testing.T) {
	usage := &Usage{Extra: ExtraFields{"completion_tokens_details": json.RawMessage(`"not an object"`)}}
	if got := usage.GetReasoningTokens(); got != 0 {
		t.Fatalf("got %d, want 0", got)
	}
	if got := (*Usage)(nil).GetReasoningTokens(); got != 0 {
		t.Fatalf("nil usage returned %d", got)
	}
}
