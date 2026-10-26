package router

import (
	"context"
	"errors"
	"testing"

	"github.com/ifnodoraemon/airoute/internal/model"
)

// A cancellation must keep errors.Is(err, context.Canceled) working (handlers map
// it to 499) while also naming the channel that was in flight and the prompt the
// upstream had already accepted, so the audit row is neither empty nor free.
func TestCanceledDispatchErrorCarriesChannelAndPrompt(t *testing.T) {
	err := newCanceledError(context.Background(), nil, "罗总", 1234)

	if !errors.Is(err, context.Canceled) {
		t.Fatal("cancellation no longer matches context.Canceled")
	}
	de, ok := AsDispatchError(err)
	if !ok {
		t.Fatal("cancellation is not a DispatchError")
	}
	if de.Trail != "罗总" {
		t.Fatalf("trail = %q, want the in-flight channel", de.Trail)
	}
	if de.PromptTokens != 1234 {
		t.Fatalf("prompt estimate = %d, want 1234", de.PromptTokens)
	}
}

// When the breaker bypasses every candidate, nothing was "attempted"; the skipped
// channels are the only attribution available.
func TestExhaustedTrailFallsBackToSkippedChannels(t *testing.T) {
	if got := exhaustedTrail(nil, []string{"a", "b"}); len(got) != 2 || got[0] != "a" {
		t.Fatalf("exhaustedTrail = %v, want the skipped channels", got)
	}
	if got := exhaustedTrail([]string{"called"}, []string{"skipped"}); len(got) != 1 || got[0] != "called" {
		t.Fatalf("exhaustedTrail = %v, want the attempted channels to win", got)
	}
}

func TestChatCompletionRequestEstimatesPrompt(t *testing.T) {
	req := &model.ChatCompletionRequest{
		Messages: []model.ChatMessage{{Role: "user", Content: "0123456789"}},
	}
	if got := req.EstimatedPromptTokens(); got <= 0 {
		t.Fatalf("estimate = %d, want a positive value", got)
	}
	var nilReq *model.ChatCompletionRequest
	if got := nilReq.EstimatedPromptTokens(); got != 0 {
		t.Fatalf("nil request estimate = %d, want 0", got)
	}
}
