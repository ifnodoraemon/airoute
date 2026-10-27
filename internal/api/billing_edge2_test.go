package api

import (
	"context"
	"encoding/json"
	"math"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/ifnodoraemon/airoute/internal/model"
	"github.com/ifnodoraemon/airoute/internal/router"
	"github.com/ifnodoraemon/airoute/internal/storage"
)

// An aborted request is billed on an estimated prompt, and that estimate used to
// bypass the calibrator: the dispatcher returns a raw chars/4 number, so the
// charge ignored everything the gateway had learned about the upstream tokenizer.
func TestAbortPromptEstimateUsesTheCalibration(t *testing.T) {
	repo := edgeBillingRepo(t, &storage.ModelPriceRecord{
		Model: "edge-abort-cal", GroupName: "default",
		PromptPrice: 1, CompletionPrice: 4, CacheReadPrice: 0.1, Currency: "CNY",
	})

	// The upstream counts twice as many tokens as the local heuristic for this
	// model (the calibrator needs a few samples before it trusts the ratio).
	for i := 0; i < 20; i++ {
		observePromptEstimate("edge-abort-cal", 100, 200)
	}

	c, _ := newTestContext(http.MethodPost, "/v1/chat/completions")
	recordFailedRequestWithError(c, "sess-abort-cal", "edge-abort-cal", 10*time.Millisecond, statusClientClosed,
		&router.DispatchError{Model: "edge-abort-cal", PromptTokens: 100, Err: context.Canceled})

	logs := edgeUsageRows(t, repo, 1)
	if got := logs[0].PromptTokens; got != 200 {
		t.Fatalf("aborted request billed %d prompt tokens, want 200 (the calibrated 2x ratio, not the raw 100)", got)
	}
	if got := logs[0].UsageSource; got != usageSourceEstimated {
		t.Fatalf("usage_source = %q, want %q", got, usageSourceEstimated)
	}
}

// The native Anthropic stream has three accounting quirks: cache writes were
// billed at the plain prompt rate, and a stream that ends without the closing
// message_delta recorded zero output tokens even though text was generated.
func TestNativeAnthropicStreamBillsCacheWritesAndOutput(t *testing.T) {
	repo := edgeBillingRepo(t, &storage.ModelPriceRecord{
		Model: "native-cache-model", GroupName: "default",
		PromptPrice: 2, CompletionPrice: 4, CacheReadPrice: 0.2, Currency: "CNY",
	})

	startEvent, err := json.Marshal(map[string]any{"message": map[string]any{
		"id":    "msg_native_1",
		"usage": map[string]any{"input_tokens": 1000, "cache_read_input_tokens": 500, "cache_creation_input_tokens": 400},
	}})
	if err != nil {
		t.Fatal(err)
	}
	first := &model.StreamEvent{Channel: "ch-native", NativeAnthropicType: "message_start", NativeAnthropicEvent: startEvent}

	stream := make(chan *model.StreamEvent, 8)
	// 4 deltas x 100 chars = 400 generated chars => 100 estimated completion tokens
	// (the shared chars/4 estimator) when no message_delta arrives.
	for i := 0; i < 4; i++ {
		delta, err := json.Marshal(map[string]any{
			"type": "content_block_delta", "index": 0,
			"delta": map[string]any{"type": "text_delta", "text": strings.Repeat("z", 100)},
		})
		if err != nil {
			t.Fatal(err)
		}
		stream <- &model.StreamEvent{Channel: "ch-native", NativeAnthropicType: "content_block_delta", NativeAnthropicEvent: delta}
	}
	close(stream)

	handler := NewHandler(nil)
	c, _ := newTestContext(http.MethodPost, "/v1/messages")
	handler.handleNativeAnthropicStream(c, stream, first, "native-cache-model", "sess-native", time.Now())

	logs := edgeUsageRows(t, repo, 1)
	row := logs[0]
	if row.PromptTokens != 1900 {
		t.Fatalf("prompt tokens = %d, want 1900 (input + cache read + cache write)", row.PromptTokens)
	}
	if row.CachedTokens != 500 {
		t.Fatalf("cached tokens = %d, want 500", row.CachedTokens)
	}
	if row.CompletionTokens != 100 {
		t.Fatalf("completion tokens = %d, want 100 (estimated from the 400 generated chars)", row.CompletionTokens)
	}
	// 1000 uncached * 2.0 + 500 cached * 0.2 + 400 cache-write * 2.5 + 100 * 4.0
	// = 2000 + 100 + 1000 + 400 nano-CNY-scaled units => CNY 0.0035.
	const wantCost = 0.0035
	if math.Abs(row.Cost-wantCost) > 1e-9 {
		t.Fatalf("cost = %v, want %v (cache writes at 1.25x, output billed)", row.Cost, wantCost)
	}
	if row.UsageSource != usageSourceEstimated {
		t.Fatalf("usage_source = %q, want %q for an estimated completion", row.UsageSource, usageSourceEstimated)
	}
	snapshot := ""
	if row.PricingSnapshot != nil {
		encoded, _ := json.Marshal(row.PricingSnapshot)
		snapshot = string(encoded)
	}
	if !strings.Contains(snapshot, `"cache_creation_tokens":400`) || !strings.Contains(snapshot, `"cache_write_price":2.5`) {
		t.Fatalf("pricing snapshot does not record the cache write: %s", snapshot)
	}
}
