package provider

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ifnodoraemon/airoute/internal/model"
)

// F-05 regression: the streaming Anthropic adapter must report the cache-write
// count exactly like the unary converter does. Billing splits PromptTokens into
// uncached / cache-read / cache-write buckets and prices the write bucket at the
// cache-write premium, so dropping the count silently under-bills every streamed
// request that warms a prompt cache.
func TestAnthropicStreamReportsCacheCreationTokens(t *testing.T) {
	const sse = "event: message_start\n" +
		`data: {"type":"message_start","message":{"id":"msg_1","usage":{"input_tokens":10,"cache_read_input_tokens":20,"cache_creation_input_tokens":300}}}` + "\n\n" +
		"event: content_block_delta\n" +
		`data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"hi"}}` + "\n\n" +
		"event: message_delta\n" +
		`data: {"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"output_tokens":5}}` + "\n\n" +
		"event: message_stop\n" +
		`data: {"type":"message_stop"}` + "\n\n"

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte(sse))
	}))
	defer srv.Close()

	p := NewAnthropicProvider(srv.Client())
	channel := &model.ChannelConfig{Name: "claude", Type: model.ProviderAnthropic, BaseURL: srv.URL, APIKey: "k"}
	stream, err := p.ChatCompleteStream(context.Background(), &model.ChatCompletionRequest{
		Model:    "claude-3-5-sonnet",
		Messages: []model.ChatMessage{{Role: "user", Content: "hi"}},
	}, channel)
	if err != nil {
		t.Fatalf("open stream: %v", err)
	}

	var usage *model.Usage
	for event := range stream {
		if event == nil || event.Chunk == nil || event.Chunk.Usage == nil {
			continue
		}
		usage = event.Chunk.Usage
	}
	if usage == nil {
		t.Fatal("no usage frame was emitted")
	}
	if usage.PromptTokens != 330 {
		t.Fatalf("prompt tokens = %d, want 330", usage.PromptTokens)
	}
	if got := usage.GetCachedTokens(); got != 20 {
		t.Fatalf("cached tokens = %d, want 20", got)
	}
	if got := usage.GetCacheCreationTokens(); got != 300 {
		t.Fatalf("cache creation tokens = %d, want 300", got)
	}

	// The unary converter must keep producing the same breakdown.
	unary := anthropicUsageToModel(&AnthropicUsage{
		InputTokens: 10, OutputTokens: 5,
		CacheReadInputTokens: 20, CacheCreationInputTokens: 300,
	})
	if unary.GetCacheCreationTokens() != 300 || unary.GetCachedTokens() != 20 {
		t.Fatalf("unary cache breakdown drifted: %+v", unary.PromptTokensDetails)
	}
}

// A stream that reports no cache usage must not grow a non-nil breakdown.
func TestAnthropicStreamOmitsEmptyCacheBreakdown(t *testing.T) {
	if got := cacheTokenDetails(0, 0); got != nil {
		t.Fatalf("expected nil breakdown, got %#v", got)
	}
}
