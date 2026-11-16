package api

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/ifnodoraemon/airoute/internal/model"
	"github.com/ifnodoraemon/airoute/internal/router"
	"github.com/ifnodoraemon/airoute/internal/storage"
)

// OpenAI interleaves parallel tool_call argument fragments by index; Anthropic
// content blocks are strictly sequential. The renderer must assemble each call
// by its upstream index and emit distinct blocks with the correct arguments,
// not concatenate both calls' arguments onto the last opened block.
func TestAnthropicStreamParallelToolIndices(t *testing.T) {
	fixBatch2WithTransport(t, func(r *http.Request) (*http.Response, error) {
		body := `data: {"id":"c1","choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"id":"call_a","type":"function","function":{"name":"f_a","arguments":""}}]}}]}

data: {"id":"c1","choices":[{"index":0,"delta":{"tool_calls":[{"index":1,"id":"call_b","type":"function","function":{"name":"f_b","arguments":""}}]}}]}

data: {"id":"c1","choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"function":{"arguments":"{\"x\":1}"}}]}}]}

data: {"id":"c1","choices":[{"index":0,"delta":{"tool_calls":[{"index":1,"function":{"arguments":"{\"y\":2}"}}]}}]}

data: {"id":"c1","choices":[{"index":0,"delta":{"content":"trailing text"}}]}

data: {"id":"c1","choices":[{"index":0,"delta":{},"finish_reason":"tool_calls"}]}

data: [DONE]

`
		return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": []string{"text/event-stream"}}, Body: io.NopCloser(strings.NewReader(body))}, nil
	})
	prevLogger := storage.GlobalAsyncLogger
	storage.GlobalAsyncLogger = nil
	t.Cleanup(func() { storage.GlobalAsyncLogger = prevLogger })

	dispatcher := router.NewDispatcher([]model.ChannelConfig{{
		Name: "up", Type: model.ProviderOpenAI, BaseURL: "https://up.invalid",
		Models: []string{"m"}, Status: "active", Weight: 1,
	}})
	h := NewHandler(dispatcher)
	canonical := &model.ChatCompletionRequest{Model: "m", Stream: true, Messages: []model.ChatMessage{{Role: "user", Content: "go"}}}
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", strings.NewReader(`{}`))
	h.handleAnthropicStream(c, c.Request.Context(), &AnthropicInboundRequest{Model: "m", Stream: true}, canonical, "s-parallel", time.Now())

	events := fixBatch2ParseSSE(w.Body.String())
	blockTool := map[int]string{}      // anthropic block index -> tool id
	blockArgs := map[int]string{}      // anthropic block index -> assembled args
	textByBlock := map[int]string{}    // anthropic block index -> text
	for _, ev := range events {
		if ev.Data == nil {
			continue
		}
		idxVal, ok := ev.Data["index"].(float64)
		if !ok {
			continue
		}
		idx := int(idxVal)
		switch ev.Event {
		case "content_block_start":
			block, _ := ev.Data["content_block"].(map[string]any)
			if block != nil && block["type"] == "tool_use" {
				id, _ := block["id"].(string)
				blockTool[idx] = id
			}
		case "content_block_delta":
			delta, _ := ev.Data["delta"].(map[string]any)
			if delta == nil {
				continue
			}
			if pj, ok := delta["partial_json"].(string); ok {
				blockArgs[idx] += pj
			}
			if txt, ok := delta["text"].(string); ok {
				textByBlock[idx] += txt
			}
		}
	}
	if len(blockTool) != 2 {
		t.Fatalf("expected 2 distinct tool_use blocks, got %d: %v\n%s", len(blockTool), blockTool, w.Body.String())
	}
	argsByID := map[string]string{}
	for idx, id := range blockTool {
		argsByID[id] = blockArgs[idx]
	}
	if argsByID["call_a"] != `{"x":1}` {
		t.Fatalf("call_a args = %q, want {\"x\":1} (interleaved fragments mis-assembled)", argsByID["call_a"])
	}
	if argsByID["call_b"] != `{"y":2}` {
		t.Fatalf("call_b args = %q, want {\"y\":2}", argsByID["call_b"])
	}
	// Trailing text must land on an open text block, never a closed tool block.
	for idx, txt := range textByBlock {
		if _, isTool := blockTool[idx]; isTool {
			t.Fatalf("text delta written onto tool block %d", idx)
		}
		if txt != "trailing text" {
			t.Fatalf("text block %d = %q, want trailing text", idx, txt)
		}
	}
	if !strings.Contains(w.Body.String(), "message_stop") {
		t.Error("stream ended without message_stop")
	}
}