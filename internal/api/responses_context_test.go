package api

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/ifnodoraemon/airoute/internal/middleware"
	"github.com/ifnodoraemon/airoute/internal/model"
	"github.com/ifnodoraemon/airoute/internal/router"
	"github.com/ifnodoraemon/airoute/internal/storage"
)

// newResponsesContextRepo installs a fresh repository as the process-wide store
// and restores the previous one when the test ends.
func newResponsesContextRepo(t *testing.T) *storage.Repository {
	t.Helper()
	db, err := storage.OpenDB(filepath.Join(t.TempDir(), "responses-context.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	previous := storage.GetGlobalRepository()
	repo := storage.NewRepository(db)
	t.Cleanup(func() {
		storage.SetGlobalRepository(previous)
		_ = db.Close()
	})
	return repo
}

// responsesContextCaller binds the caller identity the context owner is derived
// from (user id when present, else the key digest).
func responsesContextCaller(c *gin.Context, userID int64, key string) {
	c.Set(middleware.ContextKeyAPIKeyConfig, &model.APIKeyConfig{Key: key, UserID: userID})
}

// newResponsesContextHandler builds a handler whose only channel answers chat
// completions with a fixed payload.
func newResponsesContextHandler(t *testing.T, upstreamURL string) *Handler {
	t.Helper()
	return NewHandler(router.NewDispatcher([]model.ChannelConfig{{
		Name: "up", Type: model.ProviderOpenAI, BaseURL: upstreamURL,
		Models: []string{"m"}, Status: "active", Weight: 1,
		Protocols: []string{"openai_chat"},
	}}))
}

// responsesChatUpstream records every upstream request body and answers with an
// assistant turn.
func responsesChatUpstream(t *testing.T) (*httptest.Server, func() []string) {
	t.Helper()
	var mu sync.Mutex
	var bodies []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		mu.Lock()
		bodies = append(bodies, string(body))
		call := len(bodies)
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"id":"chatcmpl-ctx-%d","object":"chat.completion","created":1,"model":"m",`+
			`"choices":[{"index":0,"message":{"role":"assistant","content":"answer"},"finish_reason":"stop"}],`+
			`"usage":{"prompt_tokens":3,"completion_tokens":1,"total_tokens":4}}`, call)
	}))
	t.Cleanup(server.Close)
	return server, func() []string {
		mu.Lock()
		defer mu.Unlock()
		out := make([]string, len(bodies))
		copy(out, bodies)
		return out
	}
}

func callResponses(t *testing.T, h *Handler, body string) (*httptest.ResponseRecorder, map[string]any) {
	t.Helper()
	c, w := newTestContext(http.MethodPost, "/v1/responses")
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(body))
	responsesContextCaller(c, 7, "sk-context")
	h.HandleResponses(c)

	var decoded map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &decoded)
	return w, decoded
}

// TestResponsesPersistsAndRestoresPreviousContext is the end-to-end contract for
// previous_response_id: the first answer is stored under its response id, and a
// follow-up is answered with the prior conversation replayed.
func TestResponsesPersistsAndRestoresPreviousContext(t *testing.T) {
	repo := newResponsesContextRepo(t)
	upstream, upstreamBodies := responsesChatUpstream(t)
	h := newResponsesContextHandler(t, upstream.URL)

	first, firstBody := callResponses(t, h, `{"model":"m","input":"first question"}`)
	if first.Code != http.StatusOK {
		t.Fatalf("first response status = %d, body = %s", first.Code, first.Body.String())
	}
	firstID, _ := firstBody["id"].(string)
	if firstID == "" {
		t.Fatalf("first response has no id: %s", first.Body.String())
	}

	stored, err := repo.LoadResponseContext(t.Context(), firstID, "user:7")
	if err != nil {
		t.Fatalf("context was not persisted for %s: %v", firstID, err)
	}
	var conversation []model.ChatMessage
	if err := json.Unmarshal(stored.Messages, &conversation); err != nil {
		t.Fatalf("stored conversation is not decodable: %v", err)
	}
	if len(conversation) != 2 || conversation[0].Role != "user" || conversation[0].GetContentString() != "first question" {
		t.Fatalf("stored conversation = %#v", conversation)
	}
	if conversation[1].Role != "assistant" || conversation[1].GetContentString() != "answer" {
		t.Fatalf("assistant turn not stored: %#v", conversation)
	}

	second, secondBody := callResponses(t, h,
		fmt.Sprintf(`{"model":"m","previous_response_id":%q,"instructions":"be terse","input":"second question"}`, firstID))
	if second.Code != http.StatusOK {
		t.Fatalf("follow-up status = %d, body = %s", second.Code, second.Body.String())
	}
	secondID, _ := secondBody["id"].(string)
	if secondID == "" || secondID == firstID {
		t.Fatalf("follow-up response id = %q", secondID)
	}

	bodies := upstreamBodies()
	if len(bodies) != 2 {
		t.Fatalf("upstream saw %d requests, want 2", len(bodies))
	}
	var forwarded model.ChatCompletionRequest
	if err := json.Unmarshal([]byte(bodies[1]), &forwarded); err != nil {
		t.Fatalf("decode forwarded request: %v", err)
	}
	wantRoles := []string{"system", "user", "assistant", "user"}
	if len(forwarded.Messages) != len(wantRoles) {
		t.Fatalf("forwarded messages = %#v, want roles %v", forwarded.Messages, wantRoles)
	}
	for i, role := range wantRoles {
		if forwarded.Messages[i].Role != role {
			t.Fatalf("forwarded message %d role = %q, want %q (%#v)", i, forwarded.Messages[i].Role, role, forwarded.Messages)
		}
	}
	if forwarded.Messages[0].GetContentString() != "be terse" {
		t.Fatalf("new instructions were not kept first: %#v", forwarded.Messages[0])
	}
	if forwarded.Messages[1].GetContentString() != "first question" || forwarded.Messages[2].GetContentString() != "answer" {
		t.Fatalf("previous conversation was not replayed: %#v", forwarded.Messages)
	}
	if forwarded.Messages[3].GetContentString() != "second question" {
		t.Fatalf("new input was not appended last: %#v", forwarded.Messages[3])
	}

	// The follow-up itself becomes the new context tail.
	stored, err = repo.LoadResponseContext(t.Context(), secondID, "user:7")
	if err != nil {
		t.Fatalf("follow-up context not persisted: %v", err)
	}
	if err := json.Unmarshal(stored.Messages, &conversation); err != nil {
		t.Fatalf("decode follow-up context: %v", err)
	}
	if len(conversation) != 4 {
		t.Fatalf("follow-up stored %d messages, want 4: %#v", len(conversation), conversation)
	}
	if conversation[3].Role != "assistant" || conversation[3].GetContentString() != "answer" {
		t.Fatalf("follow-up stored assistant turn = %#v", conversation[3])
	}
}

// An unknown previous_response_id must fail loudly instead of silently dropping
// the conversation.
func TestResponsesUnknownPreviousResponseIDIsBadRequest(t *testing.T) {
	newResponsesContextRepo(t)
	upstream, upstreamBodies := responsesChatUpstream(t)
	h := newResponsesContextHandler(t, upstream.URL)

	w, body := callResponses(t, h, `{"model":"m","previous_response_id":"resp_missing","input":"hi"}`)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 (body %s)", w.Code, w.Body.String())
	}
	errObj, _ := body["error"].(map[string]any)
	if errObj["type"] != "invalid_request_error" || errObj["code"] != "previous_response_not_found" {
		t.Fatalf("error shape = %#v", body["error"])
	}
	if len(upstreamBodies()) != 0 {
		t.Fatal("a request with an unknown previous_response_id must not reach the upstream")
	}
}

// A response id belongs to the caller that created it.
func TestResponsesPreviousResponseIDOwnerMismatchIsBadRequest(t *testing.T) {
	repo := newResponsesContextRepo(t)
	upstream, upstreamBodies := responsesChatUpstream(t)
	h := newResponsesContextHandler(t, upstream.URL)

	if err := repo.SaveResponseContext(t.Context(), "resp_owned", "user:7", []byte(`[{"role":"user","content":"secret"}]`)); err != nil {
		t.Fatalf("seed context: %v", err)
	}

	c, w := newTestContext(http.MethodPost, "/v1/responses")
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses",
		strings.NewReader(`{"model":"m","previous_response_id":"resp_owned","input":"hi"}`))
	responsesContextCaller(c, 8, "sk-other")
	h.HandleResponses(c)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 (body %s)", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "previous_response_not_found") {
		t.Fatalf("owner mismatch did not report previous_response_not_found: %s", w.Body.String())
	}
	if len(upstreamBodies()) != 0 {
		t.Fatal("an owner-mismatched previous_response_id must not reach the upstream")
	}
}

// Without any context store the id cannot be resolved; say so instead of
// pretending the conversation was empty.
func TestResponsesPreviousResponseIDWithoutStoreIsBadRequest(t *testing.T) {
	previous := storage.GetGlobalRepository()
	storage.SetGlobalRepository(nil)
	t.Cleanup(func() { storage.SetGlobalRepository(previous) })

	upstream, upstreamBodies := responsesChatUpstream(t)
	h := newResponsesContextHandler(t, upstream.URL)
	w, _ := callResponses(t, h, `{"model":"m","previous_response_id":"resp_any","input":"hi"}`)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 (%s)", w.Code, w.Body.String())
	}
	if len(upstreamBodies()) != 0 {
		t.Fatal("unresolvable previous_response_id must not reach the upstream")
	}
}

// TestResponsesStreamPersistsContextAndCachedTokens drives the streaming path:
// the conversation must be stored before response.completed is emitted, and the
// upstream-reported prompt-cache tokens must reach the audit row and the usage
// event (finding #3).
func TestResponsesStreamPersistsContextAndCachedTokens(t *testing.T) {
	repo := newResponsesContextRepo(t)
	previousLogger := storage.GlobalAsyncLogger
	logger := storage.InitAsyncLogger(repo, 64, 8, 5*time.Millisecond)
	t.Cleanup(func() {
		logger.Stop()
		storage.GlobalAsyncLogger = previousLogger
	})
	storage.GlobalAsyncLogger = logger

	upstream := fixBatch2SSEServer(t,
		`data: {"id":"c1","choices":[{"index":0,"delta":{"role":"assistant","content":"answer"}}]}`,
		`data: {"id":"c1","choices":[],"usage":{"prompt_tokens":32,"completion_tokens":8,"total_tokens":40,"prompt_tokens_details":{"cached_tokens":16}}}`,
		`data: [DONE]`,
	)
	h := newResponsesContextHandler(t, upstream.URL)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(`{"model":"m","stream":true,"input":"hi"}`))
	responsesContextCaller(c, 7, "sk-context")

	req := model.ResponseRequest{Model: "m", Stream: true, Input: "hi"}
	chatReq, err := req.ToChatCompletionRequest()
	if err != nil {
		t.Fatal(err)
	}
	h.handleResponsesStreaming(c, c.Request.Context(), req, chatReq, "sess-context", time.Now())

	body := w.Body.String()
	events := fixBatch2ParseSSE(body)
	var completedID string
	var cachedTokens float64
	for _, ev := range events {
		if ev.Event != "response.completed" {
			continue
		}
		response, _ := ev.Data["response"].(map[string]any)
		completedID, _ = response["id"].(string)
		usage, _ := response["usage"].(map[string]any)
		details, _ := usage["input_tokens_details"].(map[string]any)
		cachedTokens, _ = details["cached_tokens"].(float64)
	}
	if completedID == "" {
		t.Fatalf("no response.completed id in stream:\n%s", body)
	}
	if cachedTokens != 16 {
		t.Fatalf("response.completed cached_tokens = %v, want 16 (body %s)", cachedTokens, body)
	}

	stored, err := repo.LoadResponseContext(t.Context(), completedID, "user:7")
	if err != nil {
		t.Fatalf("stream context was not persisted for %s: %v", completedID, err)
	}
	var conversation []model.ChatMessage
	if err := json.Unmarshal(stored.Messages, &conversation); err != nil {
		t.Fatalf("decode stream context: %v", err)
	}
	if len(conversation) != 2 || conversation[0].GetContentString() != "hi" {
		t.Fatalf("stream conversation = %#v", conversation)
	}
	if conversation[1].Role != "assistant" || conversation[1].GetContentString() != "answer" {
		t.Fatalf("stream assistant turn = %#v", conversation[1])
	}

	logger.Stop()
	logs, err := repo.ListUsageLogs(10, 0)
	if err != nil || len(logs) != 1 {
		t.Fatalf("usage logs = %v, err = %v", logs, err)
	}
	if logs[0].CachedTokens != 16 || logs[0].PromptTokens != 32 {
		t.Fatalf("audit row lost cached tokens: %+v", logs[0])
	}
	if logs[0].SessionID != "sess-context" {
		t.Fatalf("session affinity changed: %+v", logs[0])
	}
}
