package provider

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ifnodoraemon/airoute/internal/model"
)

// Regression tests for BUG-SSE-HALF-FRAME (fixed).
//
// reader.ReadBytes('\n') returns the bytes read so far together with io.EOF when
// the connection closes without a trailing newline. The gemini and anthropic
// readers used to return on any error and discard that buffered line, so a
// complete final data frame was dropped when the upstream (or a proxy) cut the
// connection right after it:
//   - Gemini: the finishReason/usage trailer disappeared, and the router then
//     classified the stream as truncated and charged a channel failure.
//   - Anthropic: the message_delta carrying output_tokens was lost, so usage fell
//     back to a character estimate.
//
// The openai and openai_text readers already handled this case; these tests pin
// the same behaviour for the other two, including that a processed line is never
// re-processed.

// eofSSEServer serves body and closes the connection immediately after it.
func eofSSEServer(t *testing.T, body string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(body))
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

// collectGeminiStream drains a Gemini provider stream channel.
func collectGeminiStream(t *testing.T, ch <-chan *model.StreamEvent) (chunks []*model.ChatCompletionChunk, done bool, errs []error) {
	t.Helper()
	for event := range ch {
		if event == nil {
			continue
		}
		if event.Chunk != nil {
			chunks = append(chunks, event.Chunk)
		}
		if event.IsDone {
			done = true
		}
		if event.Err != nil {
			errs = append(errs, event.Err)
		}
	}
	return chunks, done, errs
}

// TestGeminiStreamParsesFinalFrameWithoutNewline is the original reproduction:
// the last (complete) data frame arrives without a trailing newline.
func TestGeminiStreamParsesFinalFrameWithoutNewline(t *testing.T) {
	srv := eofSSEServer(t, `data: {"candidates":[{"content":{"parts":[{"text":"hi"}],"role":"model"},"finishReason":"STOP"}],"usageMetadata":{"promptTokenCount":3,"candidatesTokenCount":1,"totalTokenCount":4}}`)

	prov := NewGeminiProvider(srv.Client())
	channel := &model.ChannelConfig{
		Name: "gemini-truncated", Type: model.ProviderGemini, BaseURL: srv.URL,
		Models: []string{"m"}, Status: "active",
	}
	ch, err := prov.ChatCompleteStream(context.Background(), &model.ChatCompletionRequest{Model: "m", Stream: true}, channel)
	if err != nil {
		t.Fatalf("ChatCompleteStream: %v", err)
	}

	chunks, _, errs := collectGeminiStream(t, ch)
	if len(errs) > 0 {
		t.Fatalf("stream surfaced errors: %v", errs)
	}
	if len(chunks) != 1 {
		t.Fatalf("expected the unterminated final frame to be parsed (1 chunk), got %d", len(chunks))
	}
	chunk := chunks[0]
	if len(chunk.Choices) != 1 || chunk.Choices[0].FinishReason == nil || *chunk.Choices[0].FinishReason == "" {
		t.Fatalf("final frame lost its finishReason: %+v", chunk.Choices)
	}
	if chunk.Usage == nil || chunk.Usage.PromptTokens != 3 || chunk.Usage.CompletionTokens != 1 {
		t.Fatalf("final frame lost its usageMetadata: %+v", chunk.Usage)
	}
}

// TestGeminiStreamNewlineFramesUnchanged pins the normal framing: trailing
// newlines keep working and the buffered line is processed exactly once.
func TestGeminiStreamNewlineFramesUnchanged(t *testing.T) {
	srv := eofSSEServer(t, `data: {"candidates":[{"content":{"parts":[{"text":"a"}],"role":"model"}}]}

data: {"candidates":[{"content":{"parts":[{"text":"b"}],"role":"model"},"finishReason":"STOP"}]}
`)

	prov := NewGeminiProvider(srv.Client())
	channel := &model.ChannelConfig{Name: "gemini", Type: model.ProviderGemini, BaseURL: srv.URL, Models: []string{"m"}}
	ch, err := prov.ChatCompleteStream(context.Background(), &model.ChatCompletionRequest{Model: "m", Stream: true}, channel)
	if err != nil {
		t.Fatalf("ChatCompleteStream: %v", err)
	}

	chunks, _, errs := collectGeminiStream(t, ch)
	if len(errs) > 0 {
		t.Fatalf("stream surfaced errors: %v", errs)
	}
	if len(chunks) != 2 {
		t.Fatalf("expected 2 chunks for 2 frames, got %d (a processed line must not be re-processed)", len(chunks))
	}
	var text string
	for _, chunk := range chunks {
		for _, choice := range chunk.Choices {
			text += choice.Delta.Content
		}
	}
	if text != "ab" {
		t.Fatalf("streamed text = %q, want %q", text, "ab")
	}
}

// TestGeminiStreamMixedFraming covers a newline-terminated frame followed by an
// unterminated one, the shape a truncated proxy stream produces.
func TestGeminiStreamMixedFraming(t *testing.T) {
	srv := eofSSEServer(t, "data: {\"candidates\":[{\"content\":{\"parts\":[{\"text\":\"a\"}],\"role\":\"model\"}}]}\n\ndata: {\"candidates\":[{\"content\":{\"parts\":[{\"text\":\"b\"}],\"role\":\"model\"},\"finishReason\":\"STOP\"}]}")

	prov := NewGeminiProvider(srv.Client())
	channel := &model.ChannelConfig{Name: "gemini", Type: model.ProviderGemini, BaseURL: srv.URL, Models: []string{"m"}}
	ch, err := prov.ChatCompleteStream(context.Background(), &model.ChatCompletionRequest{Model: "m", Stream: true}, channel)
	if err != nil {
		t.Fatalf("ChatCompleteStream: %v", err)
	}

	chunks, _, errs := collectGeminiStream(t, ch)
	if len(errs) > 0 {
		t.Fatalf("stream surfaced errors: %v", errs)
	}
	if len(chunks) != 2 {
		t.Fatalf("expected 2 chunks (first terminated, second not), got %d", len(chunks))
	}
	if last := chunks[len(chunks)-1]; len(last.Choices) > 0 && (last.Choices[0].FinishReason == nil || *last.Choices[0].FinishReason == "") {
		t.Fatalf("unterminated frame lost its finishReason: %+v", last.Choices)
	}
}

// TestAnthropicNativeStreamParsesUnterminatedMessageDelta covers the native
// Anthropic path: the message_delta frame (output_tokens) arrives without a
// trailing newline and must still be forwarded, with exactly one IsDone.
func TestAnthropicNativeStreamParsesUnterminatedMessageDelta(t *testing.T) {
	srv := eofSSEServer(t, "event: message_start\n"+`data: {"type":"message_start","message":{"id":"msg_1","usage":{"input_tokens":5}}}`+"\n\n"+
		"event: message_delta\n"+`data: {"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"output_tokens":9}}`)

	inbound := &model.AnthropicInboundRequest{Model: "m", MaxTokens: 16}
	req := &model.ChatCompletionRequest{Model: "m", Stream: true, AnthropicOriginal: inbound}
	channel := &model.ChannelConfig{
		Name: "anthropic", Type: model.ProviderAnthropic, BaseURL: srv.URL,
		Models: []string{"m"}, Protocols: []string{"anthropic_messages"},
	}
	prov := NewAnthropicProvider(srv.Client())
	ch, err := prov.ChatCompleteStream(context.Background(), req, channel)
	if err != nil {
		t.Fatalf("ChatCompleteStream: %v", err)
	}

	var nativeTypes []string
	doneCount := 0
	for event := range ch {
		if event == nil {
			continue
		}
		if event.Err != nil {
			t.Fatalf("stream error: %v", event.Err)
		}
		if event.IsDone {
			doneCount++
		}
		if len(event.NativeAnthropicEvent) > 0 {
			nativeTypes = append(nativeTypes, event.NativeAnthropicType)
		}
	}

	if len(nativeTypes) != 2 {
		t.Fatalf("expected message_start + message_delta events, got %v", nativeTypes)
	}
	if nativeTypes[1] != "message_delta" {
		t.Fatalf("second event = %q, want message_delta (the unterminated frame was dropped?)", nativeTypes[1])
	}
	if doneCount != 1 {
		t.Fatalf("IsDone emitted %d times, want exactly 1 (no double terminal frame)", doneCount)
	}
}
