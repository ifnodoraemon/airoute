package provider

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ifnodoraemon/airoute/internal/model"
)

// B-12: the openai_text streaming adapter must apply the same inline
// <think>...</think> split the unary adapter already applies. The same upstream
// used to report thinking tokens as reasoning_content when not streamed and leak
// the raw tags into content when streamed.
func TestTextStreamSplitInlineReasoning(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		// The tag is intentionally split across two data frames to exercise the
		// partial-marker buffer, not just the trivial same-chunk case.
		fmt.Fprint(w, "data: {\"id\":\"t1\",\"choices\":[{\"index\":0,\"text\":\"<thi\"}]}\n\n")
		fmt.Fprint(w, "data: {\"id\":\"t1\",\"choices\":[{\"index\":0,\"text\":\"nk>step 1</think>answer\"}]}\n\n")
		fmt.Fprint(w, "data: [DONE]\n\n")
	}))
	defer server.Close()

	channel := &model.ChannelConfig{
		Name: "text-upstream", Type: model.ProviderCustom, BaseURL: server.URL,
		Protocols: []string{"openai_text"},
	}
	events, err := NewOpenAIProvider(server.Client()).ChatCompleteStream(
		context.Background(), &model.ChatCompletionRequest{Model: "m", Stream: true}, channel)
	if err != nil {
		t.Fatalf("stream: %v", err)
	}

	var content, reasoning strings.Builder
	sawDone := false
	for event := range events {
		if event.Err != nil {
			t.Fatalf("stream error: %v", event.Err)
		}
		if event.IsDone {
			sawDone = true
		}
		if event.Chunk == nil {
			continue
		}
		for _, choice := range event.Chunk.Choices {
			content.WriteString(choice.Delta.Content)
			reasoning.WriteString(choice.Delta.ReasoningContent)
		}
	}

	if !sawDone {
		t.Fatal("text stream ended without a terminal event")
	}
	if strings.Contains(content.String(), "<think>") || strings.Contains(content.String(), "</think>") {
		t.Fatalf("thinking tags leaked into streamed content %q", content.String())
	}
	if !strings.Contains(reasoning.String(), "step 1") {
		t.Fatalf("reasoning_content = %q, want the thinking trace split out", reasoning.String())
	}
	if !strings.Contains(content.String(), "answer") {
		t.Fatalf("content = %q, want the final answer", content.String())
	}
}
