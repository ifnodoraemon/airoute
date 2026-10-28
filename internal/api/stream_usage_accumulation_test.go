package api

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/ifnodoraemon/airoute/internal/model"
	"github.com/ifnodoraemon/airoute/internal/router"
	"github.com/ifnodoraemon/airoute/internal/storage"
)

// Regression test for the streaming usage-accumulation rule.
//
// A streaming upstream may report usage incrementally: the prompt count arrives
// on an early frame, and a later frame carries only the completion count (zeroing
// the prompt field). Overwriting the accumulators with each frame's values erased
// the earlier counters, so the request was billed on a partial (or estimated)
// prompt despite the upstream having reported it. The chat and messages handlers
// already accumulated with max(); the Gemini, Responses and legacy-completions
// handlers did not, so the same upstream produced different bills per entrypoint.

// incrementalUsageSSE reports prompt tokens in the first frame and only completion
// tokens in the terminal frame.
const incrementalUsageSSE = `data: {"id":"u1","choices":[{"index":0,"delta":{"content":"hi"},"finish_reason":null}],"usage":{"prompt_tokens":100,"completion_tokens":0,"total_tokens":100}}

data: {"id":"u1","choices":[{"index":0,"delta":{},"finish_reason":"stop"}],"usage":{"prompt_tokens":0,"completion_tokens":50,"total_tokens":50}}

data: [DONE]

`

const streamUsageModel = "usage-accumulation-model"

// newStreamUsageFixture builds a handler whose upstream emits the incremental
// usage body, plus the audit repository it bills into.
func newStreamUsageFixture(t *testing.T) (*Handler, *storage.Repository) {
	t.Helper()
	repo := edgeBillingRepo(t, &storage.ModelPriceRecord{
		Model: streamUsageModel, GroupName: "default",
		PromptPrice: 2, CompletionPrice: 8, Currency: "CNY",
	})
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, incrementalUsageSSE)
	}))
	t.Cleanup(upstream.Close)
	dispatcher := router.NewDispatcher([]model.ChannelConfig{{
		Name: "usage-acc", Type: model.ProviderOpenAI, BaseURL: upstream.URL,
		Models: []string{streamUsageModel}, Status: "active", Weight: 1,
	}})
	return NewHandler(dispatcher), repo
}

// TestGeminiStreamUsageAccumulatesAcrossFrames pins the Gemini entrypoint: the
// terminal usageMetadata must keep the prompt count reported earlier.
func TestGeminiStreamUsageAccumulatesAcrossFrames(t *testing.T) {
	body := runGeminiStreamProbe(t, incrementalUsageSSE)
	frames := geminiSSEDataFrames(t, body)

	var usage map[string]any
	for _, frame := range frames {
		if raw, ok := frame["usageMetadata"].(map[string]any); ok {
			usage = raw
		}
	}
	if usage == nil {
		t.Fatalf("no usageMetadata frame reached the client:\n%s", body)
	}
	if got := usage["promptTokenCount"]; got != float64(100) {
		t.Fatalf("promptTokenCount = %v, want 100 (an output-only usage frame erased the earlier prompt count)", got)
	}
	if got := usage["candidatesTokenCount"]; got != float64(50) {
		t.Fatalf("candidatesTokenCount = %v, want 50", got)
	}
	if got := usage["totalTokenCount"]; got != float64(150) {
		t.Fatalf("totalTokenCount = %v, want 150 (prompt+completion, not a single frame's total)", got)
	}
}

// TestCompletionsStreamUsageAccumulatesAcrossFrames pins the legacy-completions
// entrypoint through the audit row it bills from.
func TestCompletionsStreamUsageAccumulatesAcrossFrames(t *testing.T) {
	handler, repo := newStreamUsageFixture(t)

	chatReq := model.ChatCompletionRequest{
		Model:    streamUsageModel,
		Stream:   true,
		Messages: []model.ChatMessage{{Role: "user", Content: "hi"}},
	}
	req := LegacyCompletionRequest{Model: streamUsageModel, Prompt: "hi", Stream: true}
	c, _ := newTestContext(http.MethodPost, "/v1/completions")
	handler.handleCompletionsStream(c, c.Request.Context(), req, chatReq, "sess-usage-acc", time.Now())

	rows := edgeUsageRows(t, repo, 1)
	if got := rows[0].PromptTokens; got != 100 {
		t.Fatalf("audit prompt tokens = %d, want 100 (the output-only usage frame erased the prompt count)", got)
	}
	if got := rows[0].CompletionTokens; got != 50 {
		t.Fatalf("audit completion tokens = %d, want 50", got)
	}
}
