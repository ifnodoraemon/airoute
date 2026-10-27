package provider

import (
	"bufio"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ifnodoraemon/airoute/internal/model"
)

func p2SSEServer(t *testing.T, body string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		writer := bufio.NewWriter(w)
		_, _ = writer.WriteString(body)
		_ = writer.Flush()
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

func p2AnthropicStreamFixture() string {
	return strings.Join([]string{
		"event: message_start",
		`data: {"type":"message_start","message":{"id":"msg_1","usage":{"input_tokens":5}}}`,
		"",
		"event: content_block_start",
		`data: {"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}`,
		"",
		"event: content_block_delta",
		`data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"Let me check."}}`,
		"",
		"event: content_block_start",
		`data: {"type":"content_block_start","index":1,"content_block":{"type":"thinking","thinking":""}}`,
		"",
		"event: content_block_delta",
		`data: {"type":"content_block_delta","index":1,"delta":{"type":"thinking_delta","thinking":"pondering the request"}}`,
		"",
		"event: content_block_start",
		`data: {"type":"content_block_start","index":2,"content_block":{"type":"tool_use","id":"toolu_1","name":"get_weather"}}`,
		"",
		"event: content_block_delta",
		`data: {"type":"content_block_delta","index":2,"delta":{"type":"input_json_delta","partial_json":"{\"city\":\"x\"}"}}`,
		"",
		"event: message_delta",
		`data: {"type":"message_delta","delta":{"stop_reason":"tool_use"},"usage":{"output_tokens":9}}`,
		"",
		"event: message_stop",
		`data: {"type":"message_stop"}`,
		"",
	}, "\n")
}

func p2AnthropicEvents(t *testing.T) []*model.StreamEvent {
	t.Helper()
	srv := p2SSEServer(t, p2AnthropicStreamFixture())
	p := NewAnthropicProvider(srv.Client())
	ch := &model.ChannelConfig{Name: "a", Type: model.ProviderAnthropic, BaseURL: srv.URL, APIKey: "k", Models: []string{"m"}}
	stream, err := p.ChatCompleteStream(context.Background(), &model.ChatCompletionRequest{Model: "m", Stream: true}, ch)
	if err != nil {
		t.Fatalf("open stream: %v", err)
	}
	var events []*model.StreamEvent
	for ev := range stream {
		events = append(events, ev)
	}
	return events
}

// R-08: extended-thinking deltas were dropped on the OpenAI-adapted path.
func TestP2AdaptedStreamPreservesThinkingDeltas(t *testing.T) {
	var reasoning strings.Builder
	for _, ev := range p2AnthropicEvents(t) {
		if ev.Err != nil {
			t.Fatalf("unexpected stream error: %v", ev.Err)
		}
		if ev.Chunk == nil {
			continue
		}
		for _, choice := range ev.Chunk.Choices {
			reasoning.WriteString(choice.Delta.ReasoningContent)
		}
	}
	if !strings.Contains(reasoning.String(), "pondering the request") {
		t.Fatalf("adapted stream dropped the thinking delta: reasoning_content=%q", reasoning.String())
	}
}

// R-09: Anthropic content-block indexes are not OpenAI tool-call indexes: a tool
// call preceded by text/thinking blocks arrived as tool_calls[2] with no 0/1.
func TestP2AdaptedStreamToolCallIndexesStartAtZero(t *testing.T) {
	var indexes []int
	for _, ev := range p2AnthropicEvents(t) {
		if ev.Chunk == nil {
			continue
		}
		for _, choice := range ev.Chunk.Choices {
			for _, tc := range choice.Delta.ToolCalls {
				if tc.Index != nil {
					indexes = append(indexes, *tc.Index)
				}
			}
		}
	}
	if len(indexes) == 0 {
		t.Fatalf("no tool-call deltas emitted")
	}
	for _, idx := range indexes {
		if idx != 0 {
			t.Fatalf("tool-call indexes = %v, want a zero-based tool_call index (0)", indexes)
		}
	}
}
