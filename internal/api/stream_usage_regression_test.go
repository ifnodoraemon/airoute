package api

import (
	"fmt"
	"math"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/ifnodoraemon/airoute/internal/model"
	"github.com/ifnodoraemon/airoute/internal/router"
	"github.com/ifnodoraemon/airoute/internal/storage"
)

// F-05 regression (billing): an Anthropic stream reports cache writes in
// cache_creation_input_tokens, which billing prices at the cache-write premium
// (prompt price x 1.25). The provider used to drop that count on the streaming
// path and the API layer never accumulated it, so every streamed request that
// warmed a prompt cache was billed as if those tokens were plain input.
func TestAnthropicStreamBillsCacheWriteAtPremium(t *testing.T) {
	const modelName = "cache-write-stream-model"

	// input=100, cache_creation=900 => prompt_tokens 1000; output=10.
	const sseBody = "event: message_start\n" +
		`data: {"type":"message_start","message":{"id":"msg_1","usage":{"input_tokens":100,"cache_read_input_tokens":0,"cache_creation_input_tokens":900}}}` + "\n\n" +
		"event: content_block_delta\n" +
		`data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"hi"}}` + "\n\n" +
		"event: message_delta\n" +
		`data: {"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"output_tokens":10}}` + "\n\n" +
		"event: message_stop\n" +
		`data: {"type":"message_stop"}` + "\n\n"

	repo := edgeBillingRepo(t, &storage.ModelPriceRecord{
		Model: modelName, GroupName: "default",
		PromptPrice: 2, CompletionPrice: 8, CacheReadPrice: 0.2, Currency: "CNY",
	})
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, sseBody)
	}))
	t.Cleanup(upstream.Close)

	dispatcher := router.NewDispatcher([]model.ChannelConfig{{
		Name: "claude-cache-write", Type: model.ProviderAnthropic, BaseURL: upstream.URL,
		Models: []string{modelName}, Status: "active", Weight: 1,
	}})
	handler := NewHandler(dispatcher)

	req := &model.ChatCompletionRequest{
		Model:    modelName,
		Stream:   true,
		Messages: []model.ChatMessage{{Role: "user", Content: "hello"}},
	}
	c, _ := newTestContext(http.MethodPost, "/v1/chat/completions")
	handler.handleChatStream(c, c.Request.Context(), req, "sess-cache-write", time.Now())

	rows := edgeUsageRows(t, repo, 1)
	row := rows[0]
	if row.PromptTokens != 1000 || row.CompletionTokens != 10 {
		t.Fatalf("audit row recorded prompt=%d completion=%d, want 1000/10", row.PromptTokens, row.CompletionTokens)
	}
	if row.PricingSnapshot == nil || row.PricingSnapshot.CacheCreationTokens != 900 {
		t.Fatalf("cache-creation tokens not audited: %+v", row.PricingSnapshot)
	}
	// (100 uncached * 2 + 900 cache writes * 2*1.25 + 10 completion * 8) / 1e6
	const want = 0.00253
	if math.Abs(row.Cost-want) > 1e-9 {
		t.Fatalf("cost = %v, want %v (treating cache writes as plain input would be 0.00208)", row.Cost, want)
	}
}

// F-02 regression (streaming): the Responses stream must echo the reasoning share
// the upstream reported instead of hardcoding output_tokens_details to 0.
func TestResponsesStreamReportsUpstreamReasoningTokens(t *testing.T) {
	upstream := fixBatch2SSEServer(t,
		`data: {"id":"c1","choices":[{"index":0,"delta":{"role":"assistant","content":"answer"},"finish_reason":null}]}`,
		`data: {"id":"c1","choices":[{"index":0,"delta":{},"finish_reason":"stop"}]}`,
		`data: {"id":"c1","choices":[],"usage":{"prompt_tokens":5,"completion_tokens":120,"total_tokens":125,"completion_tokens_details":{"reasoning_tokens":77}}}`,
		`data: [DONE]`,
	)
	h := fixBatch2ChatHandler(t, upstream.URL)

	c, w := newTestContext(http.MethodPost, "/v1/responses")
	req := model.ResponseRequest{Model: "m", Stream: true, Input: "hi"}
	chatReq, err := req.ToChatCompletionRequest()
	if err != nil {
		t.Fatal(err)
	}
	h.handleResponsesStreaming(c, c.Request.Context(), req, chatReq, "sess-reasoning", time.Now())

	body := w.Body.String()
	if !strings.Contains(body, "data: [DONE]") {
		t.Fatalf("stream is missing [DONE]:\n%s", body)
	}

	var completed map[string]any
	for _, ev := range fixBatch2ParseSSE(body) {
		if ev.Event == "response.completed" {
			completed = ev.Data
		}
	}
	if completed == nil {
		t.Fatalf("no response.completed event:\n%s", body)
	}
	response, _ := completed["response"].(map[string]any)
	usage, _ := response["usage"].(map[string]any)
	if usage == nil {
		t.Fatalf("response.completed carries no usage: %v", completed)
	}
	details, _ := usage["output_tokens_details"].(map[string]any)
	if details == nil {
		t.Fatalf("usage has no output_tokens_details: %v", usage)
	}
	got, _ := details["reasoning_tokens"].(float64)
	if int(got) != 77 {
		t.Fatalf("output_tokens_details.reasoning_tokens = %v, want 77", details["reasoning_tokens"])
	}
	if out, _ := usage["output_tokens"].(float64); int(out) != 120 {
		t.Fatalf("output_tokens = %v, want 120 (reasoning is already inside it)", usage["output_tokens"])
	}
}
