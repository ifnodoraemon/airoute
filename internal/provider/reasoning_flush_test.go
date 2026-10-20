package provider

import (
	"testing"

	"github.com/ifnodoraemon/airoute/internal/model"
)

// TestFlushWithUsageOnlyChunk is a regression test for a nil/empty-choices
// panic: a content chunk ending in a partial  thinking tag is buffered, the
// final frame before [DONE] is a usage-only chunk (stream_options.include_usage)
// with an empty choices array, and Flush() used to index Choices[0] on it.
// The panic happened inside the provider goroutine and killed the process.
func TestFlushWithUsageOnlyChunk(t *testing.T) {
	a := NewReasoningStreamAdapter()

	// Content ending with a partial opening tag gets buffered.
	a.ProcessChunk(&model.ChatCompletionChunk{
		ID:    "chunk-1",
		Model: "test-model",
		Choices: []model.ChunkChoice{
			{Index: 0, Delta: model.ChunkDelta{Content: "<thi"}},
		},
	})

	usageOnly := &model.ChatCompletionChunk{
		ID:      "chunk-2",
		Model:   "test-model",
		Choices: []model.ChunkChoice{},
		Usage:   &model.Usage{PromptTokens: 5, CompletionTokens: 2, TotalTokens: 7},
	}
	a.ProcessChunk(usageOnly)

	out := a.Flush(usageOnly)
	if len(out) != 1 {
		t.Fatalf("expected the buffered tail to be flushed as one chunk, got %d", len(out))
	}
	if len(out[0].Choices) != 1 {
		t.Fatalf("expected a synthesized choice, got %d", len(out[0].Choices))
	}
	if got := out[0].Choices[0].Delta.Content; got != "<thi" {
		t.Fatalf("buffered tail lost: got %q", got)
	}
	if out[0].Usage != nil {
		t.Fatal("usage must not be duplicated onto the synthetic flush chunk")
	}
}

// TestFlushWithNoBaseChunk guards the nil path.
func TestFlushWithNoBaseChunk(t *testing.T) {
	a := NewReasoningStreamAdapter()
	a.ProcessChunk(&model.ChatCompletionChunk{
		Choices: []model.ChunkChoice{{Index: 0, Delta: model.ChunkDelta{Content: "<th"}}},
	})
	if out := a.Flush(nil); out != nil {
		t.Fatalf("expected nil flush for nil base chunk, got %d chunks", len(out))
	}
}
