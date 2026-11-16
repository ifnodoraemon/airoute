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

// A chat upstream that reports finish_reason=content_filter must surface as the
// Anthropic stop_reason "refusal", not the default end_turn: an agent keys its
// safety/retry decision off that distinction. The adapted stream used to drop it.
func TestAnthropicStreamMapsContentFilterToRefusal(t *testing.T) {
	fixBatch2WithTransport(t, func(r *http.Request) (*http.Response, error) {
		body := `data: {"id":"c1","choices":[{"index":0,"delta":{"content":"partial"}}]}

data: {"id":"c1","choices":[{"index":0,"delta":{},"finish_reason":"content_filter"}]}

data: {"id":"c1","choices":[],"usage":{"prompt_tokens":4,"completion_tokens":2}}

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
	h.handleAnthropicStream(c, c.Request.Context(), &AnthropicInboundRequest{Model: "m", Stream: true}, canonical, "s-refusal", time.Now())

	var stopReason string
	for _, ev := range fixBatch2ParseSSE(w.Body.String()) {
		if ev.Event == "message_delta" {
			delta, _ := ev.Data["delta"].(map[string]any)
			if delta != nil {
				stopReason, _ = delta["stop_reason"].(string)
			}
		}
	}
	if stopReason != "refusal" {
		t.Fatalf("message_delta stop_reason = %q, want refusal for content_filter\n%s", stopReason, w.Body.String())
	}
}