package tests

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/ifnodoraemon/airoute/internal/api"
	"github.com/ifnodoraemon/airoute/internal/model"
	"github.com/ifnodoraemon/airoute/internal/router"
	"github.com/ifnodoraemon/airoute/internal/storage"
)

// These tests exercise the real HTTP path end to end (fake upstream server ->
// provider -> dispatcher -> gin handler -> client) so that the streaming and
// circuit-breaker fixes are validated at runtime, not just at the unit level.

func newChatEngine(t *testing.T, channels []model.ChannelConfig) (*gin.Engine, *router.Dispatcher) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	dispatcher := router.NewDispatcher(channels)
	handler := api.NewHandler(dispatcher)
	engine := gin.New()
	engine.POST("/v1/chat/completions", handler.HandleChatCompletions)
	return engine, dispatcher
}

// TestStreamingContract_UsageTrailerAndDone verifies that a streaming response
// terminates with [DONE] and carries the billing metadata as a trailing SSE
// comment (headers can no longer be set once the stream has started).
func TestStreamingContract_UsageTrailerAndDone(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		flusher := w.(http.Flusher)

		fmt.Fprint(w, "data: {\"id\":\"c1\",\"choices\":[{\"index\":0,\"delta\":{\"role\":\"assistant\"}}]}\n\n")
		flusher.Flush()
		fmt.Fprint(w, "data: {\"id\":\"c1\",\"choices\":[{\"index\":0,\"delta\":{\"content\":\"hello\"}}]}\n\n")
		flusher.Flush()
		fmt.Fprint(w, "data: {\"id\":\"c1\",\"choices\":[],\"usage\":{\"prompt_tokens\":11,\"completion_tokens\":3,\"total_tokens\":14}}\n\n")
		flusher.Flush()
		fmt.Fprint(w, "data: [DONE]\n\n")
		flusher.Flush()
	}))
	defer upstream.Close()

	engine, _ := newChatEngine(t, []model.ChannelConfig{{
		Name:     "stream-up",
		Type:     model.ProviderOpenAI,
		BaseURL:  upstream.URL,
		Models:   []string{"m1"},
		Priority: 1,
		Weight:   10,
	}})

	body := `{"model":"m1","stream":true,"messages":[{"role":"user","content":"hi"}]}`
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	out := w.Body.String()

	if !strings.Contains(out, `"content":"hello"`) {
		t.Fatalf("streamed content missing:\n%s", out)
	}
	if !strings.Contains(out, "data: [DONE]") {
		t.Fatalf("stream must terminate with [DONE]:\n%s", out)
	}
	if !strings.Contains(out, ": airoute-cost=") {
		t.Fatalf("expected the usage trailer comment in the stream:\n%s", out)
	}
	// The trailer must precede [DONE] so a client can read it before terminating.
	if strings.Index(out, ": airoute-cost=") > strings.Index(out, "data: [DONE]") {
		t.Fatalf("usage trailer must come before [DONE]:\n%s", out)
	}
}

// TestStreamingReasoningSeparatedFromContent verifies that literal  thinking tags
// are split into reasoning_content, so clients no longer need a tag fallback.
func TestStreamingReasoningSeparatedFromContent(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		flusher := w.(http.Flusher)
		fmt.Fprint(w, "data: {\"id\":\"c1\",\"choices\":[{\"index\":0,\"delta\":{\"content\":\"<think>step one</think>final answer\"}}]}\n\n")
		flusher.Flush()
		fmt.Fprint(w, "data: [DONE]\n\n")
		flusher.Flush()
	}))
	defer upstream.Close()

	engine, _ := newChatEngine(t, []model.ChannelConfig{{
		Name:     "reason-up",
		Type:     model.ProviderOpenAI,
		BaseURL:  upstream.URL,
		Models:   []string{"m1"},
		Priority: 1,
		Weight:   10,
	}})

	body := `{"model":"m1","stream":true,"messages":[{"role":"user","content":"hi"}]}`
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)
	out := w.Body.String()

	if !strings.Contains(out, `"reasoning_content":"step one"`) {
		t.Fatalf("expected reasoning_content to be emitted:\n%s", out)
	}
	if strings.Contains(out, "</think>") || strings.Contains(out, "<think>") {
		t.Fatalf("think tags must not leak into the wire stream:\n%s", out)
	}
	if !strings.Contains(out, `"content":"final answer"`) {
		t.Fatalf("expected the answer to be emitted as content:\n%s", out)
	}
}

// TestUnaryCarriesCostHeader verifies the non-streaming path still sets the
// billing header (only streaming needed the trailer workaround).
func TestUnaryCarriesCostHeader(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		stop := "stop"
		_ = json.NewEncoder(w).Encode(model.ChatCompletionResponse{
			ID:      "chatcmpl-1",
			Object:  "chat.completion",
			Created: 1,
			Model:   "m1",
			Choices: []model.ChatCompletionChoice{{
				Index:        0,
				Message:      model.ChatMessage{Role: "assistant", Content: "ok"},
				FinishReason: &stop,
			}},
			Usage: &model.Usage{PromptTokens: 10, CompletionTokens: 2, TotalTokens: 12},
		})
	}))
	defer upstream.Close()

	engine, _ := newChatEngine(t, []model.ChannelConfig{{
		Name:     "unary-up",
		Type:     model.ProviderOpenAI,
		BaseURL:  upstream.URL,
		Models:   []string{"m1"},
		Priority: 1,
		Weight:   10,
	}})

	body := `{"model":"m1","messages":[{"role":"user","content":"hi"}]}`
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if w.Header().Get("X-Airoute-Cost") == "" {
		t.Fatalf("expected X-Airoute-Cost header on a non-streaming response")
	}
}

// TestUpstream4xxDoesNotTripBreaker is the runtime check for the classification
// fix: three client-caused 400s used to open the circuit breaker for 30s and
// replay the same bad request across every other channel.
func TestUpstream4xxDoesNotTripBreaker(t *testing.T) {
	var calls int64
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt64(&calls, 1)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprint(w, `{"error":{"message":"invalid parameter: temperature"}}`)
	}))
	defer upstream.Close()

	engine, dispatcher := newChatEngine(t, []model.ChannelConfig{{
		Name:     "bad-request-up",
		Type:     model.ProviderOpenAI,
		BaseURL:  upstream.URL,
		Models:   []string{"m1"},
		Priority: 1,
		Weight:   10,
	}})

	var lastResponse string
	for i := 0; i < 4; i++ {
		body := `{"model":"m1","messages":[{"role":"user","content":"hi"}]}`
		req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewBufferString(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		engine.ServeHTTP(w, req)

		if w.Code != http.StatusBadGateway && w.Code != http.StatusBadRequest {
			t.Fatalf("attempt %d: unexpected status %d: %s", i+1, w.Code, w.Body.String())
		}
		lastResponse = w.Body.String()
	}

	if got := dispatcher.GetBreakerStatus("bad-request-up"); got == "OPEN" {
		t.Fatalf("client-side 400s must not trip the circuit breaker, got %s", got)
	}
	if got := atomic.LoadInt64(&calls); got != 4 {
		t.Fatalf("expected the channel to be attempted once per request (4), got %d", got)
	}
	// The upstream diagnostic must survive to the caller.
	if !strings.Contains(strings.ToLower(lastResponse), "invalid parameter") {
		t.Fatalf("expected the upstream 400 message to be surfaced, got: %s", lastResponse)
	}
}

// TestTextOnlyChannelRejectsToolRequests verifies the adapter refuses to silently
// drop tool definitions (previously they vanished and the model could never call
// a tool, with no error to explain why).
func TestTextOnlyChannelRejectsToolRequests(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"id":"cmpl-1","object":"text_completion","created":1,"model":"m1","choices":[{"text":"x","index":0,"finish_reason":"stop"}]}`)
	}))
	defer upstream.Close()

	engine, _ := newChatEngine(t, []model.ChannelConfig{{
		Name:      "text-only-up",
		Type:      model.ProviderOpenAI,
		BaseURL:   upstream.URL,
		Models:    []string{"m1"},
		Protocols: []string{"openai_text"},
		Priority:  1,
		Weight:    10,
	}})

	body := `{"model":"m1","messages":[{"role":"user","content":"hi"}],"tools":[{"type":"function","function":{"name":"get_weather","description":"d","parameters":{"type":"object"}}}]}`
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	if w.Code == http.StatusOK {
		t.Fatalf("a tool request must not be served by a text-only channel, got 200: %s", w.Body.String())
	}
	if !strings.Contains(strings.ToLower(w.Body.String()), "text-completion") {
		t.Fatalf("expected an explanatory capability error, got: %s", w.Body.String())
	}
}

// TestPostgresStatementTimeoutInjection covers the DSN hardening that bounds
// context-free queries on the server side.
func TestPostgresStatementTimeoutInjection(t *testing.T) {
	got := storage.ApplyPostgresStatementTimeouts("postgres://u:p@localhost:5432/db?sslmode=disable")
	if !strings.Contains(got, "statement_timeout") {
		t.Fatalf("expected statement_timeout in the DSN, got %s", got)
	}
	// An operator-provided options value must win.
	custom := "postgres://u:p@localhost:5432/db?options=-c%20statement_timeout%3D5000"
	if out := storage.ApplyPostgresStatementTimeouts(custom); out != custom {
		t.Fatalf("operator options must be preserved, got %s", out)
	}
	// Keyword/value form.
	kv := storage.ApplyPostgresStatementTimeouts("host=localhost dbname=db")
	if !strings.Contains(kv, "statement_timeout") {
		t.Fatalf("expected statement_timeout in the keyword DSN, got %s", kv)
	}
}
