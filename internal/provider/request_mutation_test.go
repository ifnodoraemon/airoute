package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/ifnodoraemon/airoute/internal/model"
)

// Regression tests for BUG-REQ-MUTATION (fixed).
//
// The OpenAI-compatible provider strips reasoning_content from the messages it
// sends upstream (strict endpoints such as Azure reject the unknown field). It did
// so by clearing the field in place through a shallow `clonedReq := *req`, which
// rewrote the CALLER's request: the request object is shared by every later
// channel attempt, the cross-model fallback, the prompt-size estimators and the
// audit/trace hooks, so one channel's outbound adaptation leaked into all of them.
//
// The strip now works on a copy of the slice.

// reasoningFixtureRequest builds a multi-turn request carrying reasoning_content
// history, plus a captured-copy of the original values.
func reasoningFixtureRequest() (req *model.ChatCompletionRequest, originalReasoning string) {
	originalReasoning = "The user asks a physics question; recall Rayleigh scattering ∝ 1/λ⁴."
	req = &model.ChatCompletionRequest{
		Model: "reasoning-model",
		Messages: []model.ChatMessage{
			{Role: "user", Content: "why is the sky blue?"},
			{Role: "assistant", Content: "Rayleigh scattering.", ReasoningContent: originalReasoning},
			{Role: "user", Content: "and at sunset?"},
		},
	}
	return req, originalReasoning
}

// captureUpstreamServer records the request bodies it receives and answers with a
// minimal valid completion.
func captureUpstreamServer(t *testing.T) (*httptest.Server, func() []map[string]any) {
	t.Helper()
	var mu sync.Mutex
	var bodies []map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		mu.Lock()
		bodies = append(bodies, body)
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"id":"chatcmpl-1","object":"chat.completion","created":1,"model":"m",`+
			`"choices":[{"index":0,"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}],`+
			`"usage":{"prompt_tokens":5,"completion_tokens":1,"total_tokens":6}}`)
	}))
	t.Cleanup(srv.Close)
	return srv, func() []map[string]any {
		mu.Lock()
		defer mu.Unlock()
		out := make([]map[string]any, len(bodies))
		copy(out, bodies)
		return out
	}
}

// messageReasoning extracts messages[i].reasoning_content from a captured body.
func messageReasoning(body map[string]any, index int) string {
	messages, _ := body["messages"].([]any)
	if index >= len(messages) {
		return ""
	}
	message, _ := messages[index].(map[string]any)
	value, _ := message["reasoning_content"].(string)
	return value
}

// TestChatCompleteDoesNotMutateCallerRequest is the original reproduction: after a
// unary call the caller's reasoning history must be untouched, while the outbound
// body must not carry reasoning_content.
func TestChatCompleteDoesNotMutateCallerRequest(t *testing.T) {
	srv, captured := captureUpstreamServer(t)
	req, originalReasoning := reasoningFixtureRequest()
	channel := &model.ChannelConfig{
		Name: "openai-compat", Type: model.ProviderOpenAI, BaseURL: srv.URL,
		Models: []string{"reasoning-model"}, Status: "active",
	}

	if _, err := NewOpenAIProvider(srv.Client()).ChatComplete(context.Background(), req, channel); err != nil {
		t.Fatalf("ChatComplete: %v", err)
	}

	// The caller object is unchanged…
	if got := req.Messages[1].ReasoningContent; got != originalReasoning {
		t.Fatalf("caller's req.Messages[1].ReasoningContent = %q, want it preserved as %q", got, originalReasoning)
	}
	// …while the wire body was sanitised (that is the point of the strip).
	bodies := captured()
	if len(bodies) != 1 {
		t.Fatalf("expected 1 upstream request, got %d", len(bodies))
	}
	if got := messageReasoning(bodies[0], 1); got != "" {
		t.Fatalf("outbound body carried reasoning_content = %q, want it stripped before sending", got)
	}
}

// TestChatCompleteStreamDoesNotMutateCallerRequest covers the streaming path,
// which had the same shallow-copy strip.
func TestChatCompleteStreamDoesNotMutateCallerRequest(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, "data: {\"id\":\"c1\",\"choices\":[{\"index\":0,\"delta\":{\"content\":\"ok\"},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n")
	}))
	defer upstream.Close()

	req, originalReasoning := reasoningFixtureRequest()
	req.Stream = true
	channel := &model.ChannelConfig{
		Name: "openai-compat", Type: model.ProviderOpenAI, BaseURL: upstream.URL,
		Models: []string{"reasoning-model"}, Status: "active",
	}
	ch, err := NewOpenAIProvider(upstream.Client()).ChatCompleteStream(context.Background(), req, channel)
	if err != nil {
		t.Fatalf("ChatCompleteStream: %v", err)
	}
	for range ch {
	}

	if got := req.Messages[1].ReasoningContent; got != originalReasoning {
		t.Fatalf("streaming path mutated the caller's reasoning history: %q, want %q", got, originalReasoning)
	}
}

// TestSecondChannelStillSeesReasoningHistory is the cross-channel consequence: a
// failover to a channel that does support reasoning_content must still receive the
// history, and the prompt estimator must not shrink after an OpenAI attempt.
func TestSecondChannelStillSeesReasoningHistory(t *testing.T) {
	first, capturedFirst := captureUpstreamServer(t)
	second, capturedSecond := captureUpstreamServer(t)

	req, originalReasoning := reasoningFixtureRequest()
	beforeEstimate := req.EstimatedPromptTokens()

	firstChannel := &model.ChannelConfig{
		Name: "strict-openai", Type: model.ProviderOpenAI, BaseURL: first.URL,
		Models: []string{"reasoning-model"}, Status: "active",
	}
	secondChannel := &model.ChannelConfig{
		Name: "reasoning-openai", Type: model.ProviderOpenAI, BaseURL: second.URL,
		Models: []string{"reasoning-model"}, Status: "active",
	}

	provider := NewOpenAIProvider(first.Client())
	if _, err := provider.ChatComplete(context.Background(), req, firstChannel); err != nil {
		t.Fatalf("first channel: %v", err)
	}
	if _, err := provider.ChatComplete(context.Background(), req, secondChannel); err != nil {
		t.Fatalf("second channel: %v", err)
	}

	// Both outbound bodies are sanitised (the strip is a channel rule)…
	for name, bodies := range map[string][]map[string]any{"first": capturedFirst(), "second": capturedSecond()} {
		if len(bodies) != 1 {
			t.Fatalf("%s channel: expected 1 request, got %d", name, len(bodies))
		}
		if got := messageReasoning(bodies[0], 1); got != "" {
			t.Fatalf("%s channel outbound body carried reasoning_content = %q, want stripped", name, got)
		}
	}

	// …but the request object still holds the history for later consumers.
	if got := req.Messages[1].ReasoningContent; got != originalReasoning {
		t.Fatalf("reasoning history lost between channel attempts: %q, want %q", got, originalReasoning)
	}
	if got := req.EstimatedPromptTokens(); got != beforeEstimate {
		t.Fatalf("prompt estimate changed from %d to %d after a channel attempt (the request was mutated)", beforeEstimate, got)
	}
}

// TestStripReasoningContentLeavesInputSliceAlone pins the helper contract directly.
func TestStripReasoningContentLeavesInputSliceAlone(t *testing.T) {
	messages := []model.ChatMessage{
		{Role: "assistant", Content: "a", ReasoningContent: "keep me"},
		{Role: "user", Content: "b"},
	}
	stripped := stripReasoningContent(messages)

	if messages[0].ReasoningContent != "keep me" {
		t.Fatalf("input slice mutated: %q", messages[0].ReasoningContent)
	}
	if stripped[0].ReasoningContent != "" {
		t.Fatalf("returned slice still carries reasoning content: %q", stripped[0].ReasoningContent)
	}
	// Same length and other fields preserved.
	if len(stripped) != len(messages) || stripped[0].Content != "a" || stripped[1].Content != "b" {
		t.Fatalf("returned slice is not a faithful copy: %+v", stripped)
	}
}
