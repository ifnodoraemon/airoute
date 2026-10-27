package api

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/ifnodoraemon/airoute/internal/billing"
	"github.com/ifnodoraemon/airoute/internal/middleware"
	"github.com/ifnodoraemon/airoute/internal/model"
	"github.com/ifnodoraemon/airoute/internal/provider"
	"github.com/ifnodoraemon/airoute/internal/router"
	"github.com/ifnodoraemon/airoute/internal/storage"
)

// ---------------------------------------------------------------------------
// shared harness
// ---------------------------------------------------------------------------

type fixBatch2Transport func(*http.Request) (*http.Response, error)

func (f fixBatch2Transport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func fixBatch2Response(status int, body string) *http.Response {
	return &http.Response{
		StatusCode: status,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}

// fixBatch2WithTransport installs a fake shared upstream transport and restores
// it (plus the billing engine) after the test.
func fixBatch2WithTransport(t *testing.T, transport fixBatch2Transport) {
	t.Helper()
	prevClient, prevEngine := provider.SharedDefaultHTTPClient, billing.GlobalEngine
	provider.SharedDefaultHTTPClient = &http.Client{Transport: transport}
	billing.GlobalEngine = nil
	t.Cleanup(func() {
		provider.SharedDefaultHTTPClient = prevClient
		billing.GlobalEngine = prevEngine
	})
}

// fixBatch2Repo returns an in-memory repository whose async logger writes
// synchronously enough for tests to read the audit rows back.
func fixBatch2Repo(t *testing.T) (*storage.Repository, *storage.AsyncLogger) {
	t.Helper()
	db, err := storage.OpenDB(filepath.Join(t.TempDir(), "batch2.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	repo := storage.NewRepository(db)
	prev := storage.GlobalAsyncLogger
	logger := storage.InitAsyncLogger(repo, 64, 8, 5*time.Millisecond)
	t.Cleanup(func() {
		logger.Stop()
		storage.GlobalAsyncLogger = prev
	})
	return repo, logger
}

// fixBatch2SSEServer serves a canned chat-completions SSE stream.
func fixBatch2SSEServer(t *testing.T, frames ...string) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		flusher, _ := w.(http.Flusher)
		for _, frame := range frames {
			fmt.Fprint(w, frame)
			if !strings.HasSuffix(frame, "\n\n") {
				fmt.Fprint(w, "\n\n")
			}
			if flusher != nil {
				flusher.Flush()
			}
		}
	}))
	t.Cleanup(server.Close)
	return server
}

type fixBatch2SSEEvent struct {
	Event string
	Data  map[string]any
	Raw   string
}

// fixBatch2ParseSSE parses an SSE body into events, keeping the raw data line.
func fixBatch2ParseSSE(body string) []fixBatch2SSEEvent {
	var out []fixBatch2SSEEvent
	for _, block := range strings.Split(body, "\n\n") {
		var ev fixBatch2SSEEvent
		for _, line := range strings.Split(block, "\n") {
			switch {
			case strings.HasPrefix(line, "event: "):
				ev.Event = strings.TrimPrefix(line, "event: ")
			case strings.HasPrefix(line, "data: "):
				ev.Raw = strings.TrimPrefix(line, "data: ")
				_ = json.Unmarshal([]byte(ev.Raw), &ev.Data)
			}
		}
		if ev.Event != "" || ev.Raw != "" {
			out = append(out, ev)
		}
	}
	return out
}

func fixBatch2ChatHandler(t *testing.T, upstreamURL string) *Handler {
	t.Helper()
	return NewHandler(router.NewDispatcher([]model.ChannelConfig{{
		Name: "up", Type: model.ProviderOpenAI, BaseURL: upstreamURL,
		Models: []string{"m"}, Status: "active", Weight: 1,
		Protocols: []string{"openai_chat"},
	}}))
}

// ---------------------------------------------------------------------------
// A07: Responses API streaming event schema
// ---------------------------------------------------------------------------

func TestFixResponsesStreamEventSchema(t *testing.T) {
	upstream := fixBatch2SSEServer(t,
		`data: {"id":"c1","choices":[{"index":0,"delta":{"role":"assistant","content":"hel"}}]}`,
		`data: {"id":"c1","choices":[{"index":0,"delta":{"content":"lo"}}]}`,
		`data: {"id":"c1","choices":[],"usage":{"prompt_tokens":3,"completion_tokens":2}}`,
		`data: [DONE]`,
	)
	h := fixBatch2ChatHandler(t, upstream.URL)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses",
		strings.NewReader(`{"model":"m","stream":true,"input":"hi"}`))

	req := model.ResponseRequest{Model: "m", Stream: true, Input: "hi"}
	chatReq, err := req.ToChatCompletionRequest()
	if err != nil {
		t.Fatal(err)
	}
	h.handleResponsesStreaming(c, c.Request.Context(), req, chatReq, "sess-fix", time.Now())

	body := w.Body.String()
	if !strings.Contains(body, "data: [DONE]") {
		t.Fatalf("stream is missing the [DONE] sentinel:\n%s", body)
	}
	events := fixBatch2ParseSSE(body)

	// 1. every named event carries the documented type and a monotonic sequence.
	seq := -1
	byType := map[string][]map[string]any{}
	for _, ev := range events {
		if ev.Event == "" || ev.Event == "error" && ev.Data == nil {
			continue // [DONE] sentinel / trailer comment
		}
		typ, _ := ev.Data["type"].(string)
		if typ != ev.Event {
			t.Errorf("%s event: data type is %q, want %q", ev.Event, typ, ev.Event)
		}
		rawSeq, ok := ev.Data["sequence_number"].(float64)
		if !ok {
			t.Errorf("%s event is missing sequence_number: %v", ev.Event, ev.Data)
			continue
		}
		if int(rawSeq) <= seq {
			t.Errorf("%s event sequence_number %v is not greater than previous %d", ev.Event, rawSeq, seq)
		}
		seq = int(rawSeq)
		byType[ev.Event] = append(byType[ev.Event], ev.Data)
	}

	// 2. the documented lifecycle is present.
	for _, want := range []string{
		"response.created", "response.in_progress",
		"response.output_item.added", "response.content_part.added",
		"response.output_text.delta", "response.output_text.done",
		"response.content_part.done", "response.output_item.done",
		"response.completed",
	} {
		if len(byType[want]) == 0 {
			t.Errorf("missing documented event %q (got %v)", want, fixBatch2EventNames(events))
		}
	}

	// 3. item correlation fields that SDKs use to assemble the output.
	created := byType["response.created"]
	if len(created) > 0 {
		resp, _ := created[0]["response"].(map[string]any)
		for _, field := range []string{"id", "object", "status", "model", "output"} {
			if _, ok := resp[field]; !ok {
				t.Errorf("response.created.response is missing %q", field)
			}
		}
	}
	itemAdded := byType["response.output_item.added"]
	if len(itemAdded) == 0 {
		t.Fatal("no response.output_item.added event")
	}
	item, _ := itemAdded[0]["item"].(map[string]any)
	if item == nil {
		t.Fatalf("response.output_item.added uses %v instead of an 'item' object", itemAdded[0])
	}
	itemID, _ := item["id"].(string)
	if itemID == "" {
		t.Error("output item has no id")
	}
	if idx, _ := itemAdded[0]["output_index"].(float64); idx != 0 {
		t.Errorf("output_item.added output_index = %v, want 0", itemAdded[0]["output_index"])
	}

	partAdded := byType["response.content_part.added"]
	if len(partAdded) == 0 {
		t.Fatal("no response.content_part.added event")
	}
	for _, field := range []string{"item_id", "output_index", "content_index", "part"} {
		if _, ok := partAdded[0][field]; !ok {
			t.Errorf("response.content_part.added is missing %q", field)
		}
	}
	if got, _ := partAdded[0]["item_id"].(string); got != itemID {
		t.Errorf("content_part.added item_id = %q, want %q", got, itemID)
	}

	deltas := byType["response.output_text.delta"]
	if len(deltas) != 2 {
		t.Fatalf("got %d output_text.delta events, want 2", len(deltas))
	}
	var assembled strings.Builder
	for _, d := range deltas {
		for _, field := range []string{"item_id", "output_index", "content_index", "delta", "logprobs"} {
			if _, ok := d[field]; !ok {
				t.Errorf("response.output_text.delta is missing %q", field)
			}
		}
		if got, _ := d["item_id"].(string); got != itemID {
			t.Errorf("delta item_id = %q, want %q", got, itemID)
		}
		assembled.WriteString(fmt.Sprint(d["delta"]))
	}
	if assembled.String() != "hello" {
		t.Errorf("deltas assemble to %q, want %q", assembled.String(), "hello")
	}

	done := byType["response.output_text.done"]
	if len(done) == 0 {
		t.Fatal("no response.output_text.done event")
	}
	if got, _ := done[0]["text"].(string); got != "hello" {
		t.Errorf("output_text.done text = %q", got)
	}

	completed := byType["response.completed"]
	if len(completed) == 0 {
		t.Fatal("no response.completed event")
	}
	finalResp, _ := completed[0]["response"].(map[string]any)
	if status, _ := finalResp["status"].(string); status != "completed" {
		t.Errorf("response.completed status = %q", status)
	}
	output, _ := finalResp["output"].([]any)
	if len(output) != 1 {
		t.Fatalf("response.completed output = %v, want one message item", finalResp["output"])
	}
	usage, _ := finalResp["usage"].(map[string]any)
	for _, field := range []string{"input_tokens", "output_tokens", "total_tokens"} {
		if _, ok := usage[field]; !ok {
			t.Errorf("response.completed usage is missing %q", field)
		}
	}
}

func fixBatch2EventNames(events []fixBatch2SSEEvent) []string {
	names := make([]string, 0, len(events))
	for _, ev := range events {
		if ev.Event != "" {
			names = append(names, ev.Event)
		}
	}
	return names
}

// ---------------------------------------------------------------------------
// A08: Gemini :embedContent must hit the embeddings path
// ---------------------------------------------------------------------------

func TestFixGeminiEmbedContentUsesEmbeddingsPath(t *testing.T) {
	var upstreamPath, upstreamBody string
	fixBatch2WithTransport(t, func(r *http.Request) (*http.Response, error) {
		upstreamPath = r.URL.Path
		b, _ := io.ReadAll(r.Body)
		upstreamBody = string(b)
		return fixBatch2Response(200,
			`{"object":"list","data":[{"object":"embedding","index":0,"embedding":[0.1,0.2,0.3]}],"model":"text-embedding","usage":{"prompt_tokens":2,"total_tokens":2}}`), nil
	})
	h := NewHandler(router.NewDispatcher([]model.ChannelConfig{{
		Name: "emb", Type: model.ProviderOpenAI, BaseURL: "https://emb.invalid",
		Models: []string{"text-embedding"}, Protocols: []string{"embeddings"}, Status: "active", Weight: 1,
	}}))

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1beta/models/text-embedding:embedContent",
		strings.NewReader(`{"model":"models/text-embedding","content":{"parts":[{"text":"hello world"}]}}`))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Params = gin.Params{{Key: "modelAction", Value: "/text-embedding:embedContent"}}
	h.HandleGeminiAction(c)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}
	if !strings.Contains(upstreamPath, "embeddings") {
		t.Errorf("BUG: :embedContent dispatched to %q instead of the embeddings path", upstreamPath)
	}
	if strings.Contains(upstreamPath, "chat/completions") {
		t.Errorf("BUG: :embedContent still executed as a chat completion (%s)", upstreamPath)
	}
	if !strings.Contains(upstreamBody, "hello world") {
		t.Errorf("embedding input text was not forwarded: %s", upstreamBody)
	}
	var resp struct {
		Embedding struct {
			Values []float64 `json:"values"`
		} `json:"embedding"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("response is not a Gemini embedContent payload: %v (%s)", err, w.Body.String())
	}
	if len(resp.Embedding.Values) != 3 {
		t.Errorf("embedding.values = %v, want 3 floats", resp.Embedding.Values)
	}
	if strings.Contains(w.Body.String(), "candidates") {
		t.Errorf("BUG: embedContent response contains chat candidates: %s", w.Body.String())
	}
}

// ---------------------------------------------------------------------------
// A09: /v1beta/models* must honour the API key's model allowlist
// ---------------------------------------------------------------------------

func TestFixGeminiModelsHonourKeyAllowlist(t *testing.T) {
	h := NewHandler(router.NewDispatcher([]model.ChannelConfig{{
		Name: "ch", Type: model.ProviderOpenAI, BaseURL: "https://x.invalid",
		Models: []string{"allowed-model", "secret-model"}, Status: "active", Weight: 1,
	}}))
	newCtx := func(target string) (*gin.Context, *httptest.ResponseRecorder) {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest(http.MethodGet, target, nil)
		c.Set(middleware.ContextKeyAPIKeyConfig, &model.APIKeyConfig{
			Key: "sk-restricted", AllowedModels: []string{"allowed-model"},
		})
		return c, w
	}

	c, w := newCtx("/v1beta/models")
	h.HandleGeminiModels(c)
	if !strings.Contains(w.Body.String(), "allowed-model") {
		t.Errorf("allowed model missing from the listing: %s", w.Body.String())
	}
	if strings.Contains(w.Body.String(), "secret-model") {
		t.Errorf("BUG: /v1beta/models leaks a model outside the key allowlist: %s", w.Body.String())
	}

	c2, w2 := newCtx("/v1beta/models/secret-model")
	c2.Params = gin.Params{{Key: "modelAction", Value: "/secret-model"}}
	h.HandleGeminiModelDetail(c2)
	if w2.Code != http.StatusForbidden {
		t.Errorf("BUG: model detail for a non-allowed model = %d, want 403 (%s)", w2.Code, w2.Body.String())
	}

	c3, w3 := newCtx("/v1beta/models/allowed-model")
	c3.Params = gin.Params{{Key: "modelAction", Value: "/allowed-model"}}
	h.HandleGeminiModelDetail(c3)
	if w3.Code != http.StatusOK {
		t.Errorf("allowed model detail = %d, want 200 (%s)", w3.Code, w3.Body.String())
	}
}

// ---------------------------------------------------------------------------
// A10: JSON-RPC notifications and protocol version negotiation
// ---------------------------------------------------------------------------

func TestFixMCPNotificationsGetNoRPCResponse(t *testing.T) {
	h := NewMCPHandler(nil, nil) // open mode: no repo, no keys
	for _, method := range []string{"notifications/initialized", "notifications/cancelled", "notifications/progress"} {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest(http.MethodPost, "/mcp/messages",
			strings.NewReader(`{"jsonrpc":"2.0","method":"`+method+`"}`))
		c.Request.Header.Set("Content-Type", "application/json")
		h.HandleMCPMessages(c)
		c.Writer.WriteHeaderNow()

		if w.Code != http.StatusAccepted {
			t.Errorf("%s: status = %d, want 202 (notifications have no response body)", method, w.Code)
		}
		if strings.TrimSpace(w.Body.String()) != "" {
			t.Errorf("BUG: %s received a JSON-RPC response body: %s", method, w.Body.String())
		}
		if got := w.Header().Get("MCP-Protocol-Version"); got == "" {
			t.Errorf("%s: MCP-Protocol-Version response header is empty", method)
		}
	}

	// A request (with id) must still get a JSON-RPC response.
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/mcp/messages",
		strings.NewReader(`{"jsonrpc":"2.0","id":9,"method":"ping"}`))
	c.Request.Header.Set("Content-Type", "application/json")
	h.HandleMCPMessages(c)
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), "result") {
		t.Errorf("ping with id: status = %d body = %s, want 200 + result", w.Code, w.Body.String())
	}
}

func TestFixMCPInitializeNegotiatesProtocolVersion(t *testing.T) {
	h := NewMCPHandler(nil, nil)
	initialize := func(version string) (*httptest.ResponseRecorder, string) {
		body := fmt.Sprintf(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":%q,"capabilities":{},"clientInfo":{"name":"legacy","version":"1"}}}`, version)
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest(http.MethodPost, "/mcp/messages", strings.NewReader(body))
		c.Request.Header.Set("Content-Type", "application/json")
		h.HandleMCPMessages(c)
		var resp struct {
			Result struct {
				ProtocolVersion string `json:"protocolVersion"`
			} `json:"result"`
		}
		_ = json.Unmarshal(w.Body.Bytes(), &resp)
		return w, resp.Result.ProtocolVersion
	}

	for _, version := range []string{"2024-11-05", "2025-11-25", "2026-07-28"} {
		w, answered := initialize(version)
		if answered != version {
			t.Errorf("initialize requested %q but the server answered %q", version, answered)
		}
		if got := w.Header().Get("MCP-Protocol-Version"); got != version {
			t.Errorf("initialize %q: MCP-Protocol-Version header = %q", version, got)
		}
	}

	// An unsupported revision falls back to the newest supported one.
	_, answered := initialize("1999-01-01")
	if answered != mcpDefaultProtocolVersion {
		t.Errorf("unsupported version negotiated %q, want %q", answered, mcpDefaultProtocolVersion)
	}
}

// ---------------------------------------------------------------------------
// A12: trace rows carry the session id
// ---------------------------------------------------------------------------

func TestFixTraceSessionIDPopulated(t *testing.T) {
	repo, _ := fixBatch2Repo(t)
	t.Setenv("GATEWAY_TRACE_CAPTURE", "all")
	tc := storage.InitTraceCapture(repo)
	defer func() { storage.GlobalTraceCapture = nil }()

	// No channel serves the requested model, so the handler records a failed
	// trace exactly as it does in production.
	h := NewHandler(router.NewDispatcher(nil))
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions",
		strings.NewReader(`{"model":"ghost-model","messages":[{"role":"user","content":"hi"}]}`))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Request.Header.Set("X-Session-ID", "client-session-42")
	h.HandleChatCompletions(c)
	if w.Code < 400 {
		t.Fatalf("expected a dispatch failure, got %d", w.Code)
	}
	tc.Stop()

	traces, err := repo.ListRequestTraces(storage.TraceFilter{Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(traces) == 0 {
		t.Fatal("no trace row persisted")
	}
	if traces[0].SessionID != "client-session-42" {
		t.Errorf("BUG: trace session_id = %q, want %q", traces[0].SessionID, "client-session-42")
	}
}

// ---------------------------------------------------------------------------
// A14: count_tokens must 404 on an unknown model
// ---------------------------------------------------------------------------

func TestFixCountTokensUnknownModelIs404(t *testing.T) {
	h := NewHandler(router.NewDispatcher([]model.ChannelConfig{{
		Name: "known", Type: model.ProviderOpenAI, BaseURL: "https://x.invalid",
		Models: []string{"known-model"}, Status: "active", Weight: 1,
	}}))
	body := func(modelName string) string {
		return `{"model":"` + modelName + `","messages":[{"role":"user","content":"hello there"}]}`
	}

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages/count_tokens", strings.NewReader(body("ghost-model")))
	c.Request.Header.Set("Content-Type", "application/json")
	h.HandleAnthropicCountTokens(c)
	if w.Code != http.StatusNotFound {
		t.Errorf("BUG: count_tokens for an unknown model = %d, want 404 (%s)", w.Code, w.Body.String())
	}

	// A known model without a native Anthropic tokenizer keeps the documented
	// local-estimate fallback.
	w2 := httptest.NewRecorder()
	c2, _ := gin.CreateTestContext(w2)
	c2.Request = httptest.NewRequest(http.MethodPost, "/v1/messages/count_tokens", strings.NewReader(body("known-model")))
	c2.Request.Header.Set("Content-Type", "application/json")
	h.HandleAnthropicCountTokens(c2)
	if w2.Code != http.StatusOK || w2.Header().Get("X-Airoute-Token-Estimate") != "true" {
		t.Errorf("known model count_tokens = %d header=%q, want 200 + estimate marker", w2.Code, w2.Header().Get("X-Airoute-Token-Estimate"))
	}
}

// ---------------------------------------------------------------------------
// A15: oversized bodies are 413, not 400 invalid_payload
// ---------------------------------------------------------------------------

func TestFixOversizedBodyIs413(t *testing.T) {
	// Exactly what middleware.BodyLimit installs.
	limit := func(w http.ResponseWriter, body io.Reader) io.ReadCloser {
		return http.MaxBytesReader(w, io.NopCloser(body), 64)
	}
	bigJSON := `{"model":"m","messages":[{"role":"user","content":"` + strings.Repeat("x", 500) + `"}]}`

	openaiCase := func(name string, handler func(*gin.Context)) {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest(http.MethodPost, "/x", strings.NewReader(bigJSON))
		c.Request.Header.Set("Content-Type", "application/json")
		c.Request.Body = limit(w, strings.NewReader(bigJSON))
		handler(c)
		if w.Code != http.StatusRequestEntityTooLarge {
			t.Errorf("%s: status = %d, want 413 (%s)", name, w.Code, w.Body.String())
			return
		}
		if !strings.Contains(w.Body.String(), "payload_too_large") {
			t.Errorf("%s: error code is not payload_too_large: %s", name, w.Body.String())
		}
	}

	h := NewHandler(router.NewDispatcher(nil))
	openaiCase("chat/completions", h.HandleChatCompletions)
	openaiCase("completions", h.HandleCompletions)
	openaiCase("responses", h.HandleResponses)
	openaiCase("embeddings", h.HandleEmbeddings)
	openaiCase("rerank", h.HandleRerank)

	// Anthropic ingress uses the Anthropic error envelope.
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", strings.NewReader(bigJSON))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Request.Body = limit(w, strings.NewReader(bigJSON))
	h.HandleAnthropicMessages(c)
	if w.Code != http.StatusRequestEntityTooLarge {
		t.Errorf("messages: status = %d, want 413 (%s)", w.Code, w.Body.String())
	}

	// Gemini ingress.
	w2 := httptest.NewRecorder()
	c2, _ := gin.CreateTestContext(w2)
	c2.Request = httptest.NewRequest(http.MethodPost, "/v1beta/models/m:generateContent", strings.NewReader(bigJSON))
	c2.Request.Header.Set("Content-Type", "application/json")
	c2.Request.Body = limit(w2, strings.NewReader(bigJSON))
	c2.Params = gin.Params{{Key: "modelAction", Value: "/m:generateContent"}}
	h.HandleGeminiAction(c2)
	if w2.Code != http.StatusRequestEntityTooLarge {
		t.Errorf("gemini: status = %d, want 413 (%s)", w2.Code, w2.Body.String())
	}
}

// ---------------------------------------------------------------------------
// A16: adapted streaming tool_use blocks carry the mandatory input object
// ---------------------------------------------------------------------------

func TestFixAnthropicToolUseBlockHasInput(t *testing.T) {
	fixBatch2WithTransport(t, func(r *http.Request) (*http.Response, error) {
		body := `data: {"id":"c1","choices":[{"index":0,"delta":{"role":"assistant","content":"checking"}}]}

data: {"id":"c1","choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"id":"call_a","type":"function","function":{"name":"get_weather","arguments":""}}]}}]}

data: {"id":"c1","choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"function":{"arguments":"{\"city\":\"Paris\"}"}}]}}]}

data: {"id":"c1","choices":[{"index":0,"delta":{},"finish_reason":"tool_calls"}]}

data: [DONE]

`
		return &http.Response{
			StatusCode: 200,
			Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
			Body:       io.NopCloser(strings.NewReader(body)),
		}, nil
	})
	prevLogger := storage.GlobalAsyncLogger
	storage.GlobalAsyncLogger = nil
	t.Cleanup(func() { storage.GlobalAsyncLogger = prevLogger })

	dispatcher := router.NewDispatcher([]model.ChannelConfig{{
		Name: "up", Type: model.ProviderOpenAI, BaseURL: "https://up.invalid",
		Models: []string{"m"}, Status: "active", Weight: 1,
	}})
	h := NewHandler(dispatcher)
	canonical := &model.ChatCompletionRequest{
		Model: "m", Stream: true,
		Messages: []model.ChatMessage{{Role: "user", Content: "weather?"}},
		Tools: []model.Tool{{Type: "function", Function: map[string]any{
			"name": "get_weather", "parameters": map[string]any{"type": "object"},
		}}},
	}
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", strings.NewReader(`{}`))
	h.handleAnthropicStream(c, c.Request.Context(), &AnthropicInboundRequest{Model: "m", Stream: true}, canonical, "s-fix", time.Now())

	events := fixBatch2ParseSSE(w.Body.String())
	sawToolBlock := false
	for _, ev := range events {
		if ev.Event != "content_block_start" || ev.Data == nil {
			continue
		}
		block, _ := ev.Data["content_block"].(map[string]any)
		if block == nil || block["type"] != "tool_use" {
			continue
		}
		sawToolBlock = true
		if _, ok := block["input"]; !ok {
			t.Errorf("BUG: tool_use content_block_start has no \"input\" field: %v", block)
		}
		if id, _ := block["id"].(string); id != "call_a" {
			t.Errorf("tool_use block id = %q, want call_a", id)
		}
	}
	if !sawToolBlock {
		t.Fatalf("no tool_use content_block_start emitted:\n%s", w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "message_stop") {
		t.Error("stream ended without message_stop")
	}
}

// ---------------------------------------------------------------------------
// A18: session affinity keys are namespaced by the authenticated caller
// ---------------------------------------------------------------------------

func TestFixSessionAffinityKeyTenantScoped(t *testing.T) {
	// Same client-supplied session id, different tenants.
	cA, _ := newTestContext(http.MethodPost, "/v1/chat/completions")
	cA.Set(middleware.ContextKeyAPIKeyConfig, &model.APIKeyConfig{Key: "sk-tenant-a", TenantID: "tenant-a"})
	cA.Request.Header.Set("X-Session-ID", "shared-session")

	cB, _ := newTestContext(http.MethodPost, "/v1/chat/completions")
	cB.Set(middleware.ContextKeyAPIKeyConfig, &model.APIKeyConfig{Key: "sk-tenant-b", TenantID: "tenant-b"})
	cB.Request.Header.Set("X-Session-ID", "shared-session")

	sessionA := resolveSessionID(cA, "m", nil, "")
	sessionB := resolveSessionID(cB, "m", nil, "")
	if sessionA != sessionB {
		t.Fatalf("client-visible session ids differ (%q vs %q); the raw id must be preserved for headers/audit", sessionA, sessionB)
	}

	keyA := sessionAffinityKey(cA, sessionA)
	keyB := sessionAffinityKey(cB, sessionB)
	if keyA == keyB {
		t.Errorf("BUG: the affinity key is not tenant-scoped: both tenants map to %q", keyA)
	}
	if !strings.Contains(keyA, "shared-session") {
		t.Errorf("affinity key %q lost the session component", keyA)
	}

	// Same tenant + same id stays sticky.
	cA2, _ := newTestContext(http.MethodPost, "/v1/chat/completions")
	cA2.Set(middleware.ContextKeyAPIKeyConfig, &model.APIKeyConfig{Key: "sk-tenant-a", TenantID: "tenant-a"})
	cA2.Request.Header.Set("X-Session-ID", "shared-session")
	if got := sessionAffinityKey(cA2, "shared-session"); got != keyA {
		t.Errorf("affinity key is not stable within a tenant: %q vs %q", got, keyA)
	}

	// The response header keeps the client-visible id (not the namespaced key).
	if got := cA.Writer.Header().Get("X-Airoute-Session-ID"); got != "shared-session" {
		t.Errorf("X-Airoute-Session-ID = %q, want the raw client session", got)
	}
}

// ---------------------------------------------------------------------------
// A20: legacy /v1/completions prompt handling and stream termination
// ---------------------------------------------------------------------------

func TestFixCompletionsRejectsTokenIDPrompt(t *testing.T) {
	upstreamCalls := 0
	fixBatch2WithTransport(t, func(r *http.Request) (*http.Response, error) {
		upstreamCalls++
		return fixBatch2Response(200,
			`{"id":"x","object":"chat.completion","choices":[{"index":0,"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}],"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}}`), nil
	})
	h := fixBatch2ChatHandler(t, "https://unused.invalid")

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/completions", strings.NewReader(`{"model":"m","prompt":[1,2,3]}`))
	c.Request.Header.Set("Content-Type", "application/json")
	h.HandleCompletions(c)
	if w.Code != http.StatusBadRequest || !strings.Contains(w.Body.String(), "unsupported_prompt_type") {
		t.Errorf("token-id prompt = %d %s, want 400 unsupported_prompt_type", w.Code, w.Body.String())
	}
	if upstreamCalls != 0 {
		t.Errorf("BUG: token-id prompt reached the upstream %d time(s) instead of being rejected", upstreamCalls)
	}

	// A text-array prompt (e.g. ["hello"]) is still accepted and dispatched.
	w2 := httptest.NewRecorder()
	c2, _ := gin.CreateTestContext(w2)
	c2.Request = httptest.NewRequest(http.MethodPost, "/v1/completions", strings.NewReader(`{"model":"m","prompt":["hello"]}`))
	c2.Request.Header.Set("Content-Type", "application/json")
	h.HandleCompletions(c2)
	if strings.Contains(w2.Body.String(), "unsupported_prompt_type") {
		t.Errorf("a text-array prompt must keep working, got %s", w2.Body.String())
	}
	if upstreamCalls != 1 {
		t.Errorf("text-array prompt dispatched %d times, want 1", upstreamCalls)
	}
}

func TestFixCompletionsStreamErrorEmitsDone(t *testing.T) {
	fixBatch2WithTransport(t, func(r *http.Request) (*http.Response, error) {
		body := `data: {"id":"c1","choices":[{"index":0,"delta":{"content":"partial"}}]}

data: {"error":{"message":"upstream overloaded"}}

`
		return &http.Response{
			StatusCode: 200,
			Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
			Body:       io.NopCloser(strings.NewReader(body)),
		}, nil
	})
	prevLogger := storage.GlobalAsyncLogger
	storage.GlobalAsyncLogger = nil
	t.Cleanup(func() { storage.GlobalAsyncLogger = prevLogger })
	h := fixBatch2ChatHandler(t, "https://up.invalid")

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/completions",
		strings.NewReader(`{"model":"m","prompt":"hi","stream":true}`))
	c.Request.Header.Set("Content-Type", "application/json")
	h.HandleCompletions(c)

	body := w.Body.String()
	if !strings.Contains(body, "stream_error") {
		t.Fatalf("stream error was not surfaced:\n%s", body)
	}
	if !strings.Contains(body, "data: [DONE]") {
		t.Errorf("BUG: error-terminated completions stream has no [DONE] sentinel:\n%s", body)
	}
	// The terminator must be the last frame so clients stop immediately.
	trimmed := strings.TrimSpace(body)
	if !strings.HasSuffix(trimmed, "data: [DONE]") {
		t.Errorf("the [DONE] sentinel is not the final frame:\n%s", body)
	}
	_ = context.Background
}
