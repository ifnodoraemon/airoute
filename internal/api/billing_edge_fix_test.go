package api

import (
	"context"
	"net/http"
	"net/http/httptest"
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

// edgeBillingRepo installs an in-memory repository, the global billing engine and
// the async logger, so a handler-level test can assert on the audit trail.
func edgeBillingRepo(t *testing.T, prices ...*storage.ModelPriceRecord) *storage.Repository {
	t.Helper()
	db, err := storage.OpenDB(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	repo := storage.NewRepository(db)
	for _, price := range prices {
		if err := repo.SaveModelPrice(price); err != nil {
			t.Fatalf("save price %s: %v", price.Model, err)
		}
	}

	previousEngine, previousLogger := billing.GlobalEngine, storage.GlobalAsyncLogger
	previousRepo := storage.GetGlobalRepository()
	billing.GlobalEngine = billing.NewEngine(repo)
	logger := storage.InitAsyncLogger(repo, 64, 8, 5*time.Millisecond)
	storage.SetGlobalRepository(repo)
	t.Cleanup(func() {
		logger.Stop()
		billing.GlobalEngine = previousEngine
		storage.GlobalAsyncLogger = previousLogger
		storage.SetGlobalRepository(previousRepo)
	})
	return repo
}

// edgeUsageRows waits until at least want audit rows exist and returns them.
func edgeUsageRows(t *testing.T, repo *storage.Repository, want int) []*storage.UsageLogRecord {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for {
		logs, err := repo.ListUsageLogs(50, 0)
		if err == nil && len(logs) >= want {
			return logs
		}
		if time.Now().After(deadline) {
			t.Fatalf("expected %d audit rows, got %d (err=%v)", want, len(logs), err)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func edgeSSEStream(content string) string {
	chunk := `{"id":"edge-1","object":"chat.completion.chunk","created":1,"model":"m",` +
		`"choices":[{"index":0,"delta":{"content":"` + content + `"},"finish_reason":null}]}`
	return "data: " + chunk + "\n\ndata: [DONE]\n\n"
}

// A streaming request that never receives a usage report must be billed on the
// same prompt basis as the identical unary request: the canonical estimator covers
// messages, multimodal parts and tool definitions. The chat stream path used to
// count message text only, so a tool-heavy request was billed for a fraction of
// the prompt it actually sent upstream.
func TestStreamPromptFallbackIncludesToolDefinitions(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte(edgeSSEStream(strings.Repeat("x", 400))))
	}))
	defer upstream.Close()

	repo := edgeBillingRepo(t, &storage.ModelPriceRecord{
		Model: "edge-tools-model", GroupName: "default",
		PromptPrice: 1, CompletionPrice: 4, CacheReadPrice: 0.1, Currency: "CNY",
	})
	handler := NewHandler(router.NewDispatcher([]model.ChannelConfig{{
		Name: "edge-stream", Type: model.ProviderOpenAI, BaseURL: upstream.URL,
		Models: []string{"edge-tools-model"}, Status: "active", Weight: 1, Protocols: []string{"openai_chat"},
	}}))

	req := &model.ChatCompletionRequest{
		Model:    "edge-tools-model",
		Messages: []model.ChatMessage{{Role: "user", Content: strings.Repeat("m", 400)}},
		Tools: []model.Tool{{
			Type:     "function",
			Function: map[string]any{"name": "lookup_invoice", "description": strings.Repeat("t", 2000)},
		}},
	}

	c, _ := newTestContext(http.MethodPost, "/v1/chat/completions")
	handler.handleChatStream(c, c.Request.Context(), req, "sess-edge", time.Now())

	logs := edgeUsageRows(t, repo, 1)
	got := logs[0]
	wantPrompt := calibratedPromptEstimate(req.Model, estimateCanonicalPromptTokens(req))
	messagesOnly := estimateTokens(len(req.Messages[0].GetContentString()))
	if messagesOnly >= wantPrompt {
		t.Fatalf("test setup is wrong: the tools block must add prompt tokens (%d vs %d)", messagesOnly, wantPrompt)
	}
	if got.PromptTokens != wantPrompt {
		t.Fatalf("billed prompt tokens = %d, want %d (the unary basis, tools included; message-only was %d)",
			got.PromptTokens, wantPrompt, messagesOnly)
	}
	if got.UsageSource != "estimated" {
		t.Fatalf("usage_source = %q, want estimated", got.UsageSource)
	}
}

// The Anthropic adapter stream estimated completion tokens with a local "/3"
// while every other endpoint used the shared "/4" heuristic, so the same output
// text was billed one third higher on this endpoint alone.
func TestAnthropicAdapterStreamCompletionUsesSharedEstimator(t *testing.T) {
	const contentRunes = 400 // /4 = 100 tokens, /3 = 133
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte(edgeSSEStream(strings.Repeat("y", contentRunes))))
	}))
	defer upstream.Close()

	repo := edgeBillingRepo(t, &storage.ModelPriceRecord{
		Model: "edge-anthropic-model", GroupName: "default",
		PromptPrice: 1, CompletionPrice: 4, CacheReadPrice: 0.1, Currency: "CNY",
	})
	handler := NewHandler(router.NewDispatcher([]model.ChannelConfig{{
		Name: "edge-anthropic", Type: model.ProviderOpenAI, BaseURL: upstream.URL,
		Models: []string{"edge-anthropic-model"}, Status: "active", Weight: 1, Protocols: []string{"openai_chat"},
	}}))

	canonical := &model.ChatCompletionRequest{
		Model:    "edge-anthropic-model",
		Messages: []model.ChatMessage{{Role: "user", Content: "hello"}},
	}
	c, _ := newTestContext(http.MethodPost, "/v1/messages")
	handler.handleAnthropicStream(c, c.Request.Context(),
		&AnthropicInboundRequest{Model: "edge-anthropic-model", Stream: true}, canonical, "sess-anthropic", time.Now())

	logs := edgeUsageRows(t, repo, 1)
	got := logs[0]
	wantCompletion := estimateTokens(contentRunes)
	if got.CompletionTokens != wantCompletion {
		t.Fatalf("billed completion tokens = %d, want %d (the shared chars/4 estimator; an older /3 estimate billed %d)",
			got.CompletionTokens, wantCompletion, contentRunes/3)
	}
}

// A dispatch cancelled because the client went away is a client abort, not a
// gateway failure: the multimodal path answered 502 for every dispatch error.
func TestMultimodalDispatchCancellationIsClientClosed(t *testing.T) {
	h := auditReviewMultimodal(t, []model.ChannelConfig{{
		Name: "edge-cancel", BaseURL: "https://cancel.invalid", Models: []string{"image-model"},
		Protocols: []string{"images"}, Status: "active",
	}}, func(r *http.Request) (*http.Response, error) {
		return nil, context.Canceled
	})

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/images/generations", strings.NewReader(`{"model":"image-model","prompt":"x"}`))
	h.HandleImageGenerations(c)

	if w.Code != statusClientClosed {
		t.Fatalf("status = %d, want %d (a client abort must not be reported as an upstream failure)", w.Code, statusClientClosed)
	}
}

// A cancelled downstream request must not cancel the provider call: the multimodal
// paths passed the client context straight to the dispatcher, so the provider
// request died with the client and the gateway could not bill the work it had
// already paid for.
func TestMultimodalDispatchSurvivesClientCancellation(t *testing.T) {
	var transportSawCanceledContext bool
	h := auditReviewMultimodal(t, []model.ChannelConfig{{
		Name: "edge-drain", BaseURL: "https://drain.invalid", Models: []string{"image-model"},
		Protocols: []string{"images"}, Status: "active",
	}}, func(r *http.Request) (*http.Response, error) {
		if err := r.Context().Err(); err != nil {
			transportSawCanceledContext = true
			return nil, err
		}
		return auditReviewResponse(http.StatusOK, `{"data":[{"url":"https://asset.invalid/image.png"}]}`), nil
	})

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // the customer is already gone before the provider answers
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/images/generations", strings.NewReader(`{"model":"image-model","prompt":"x"}`)).WithContext(ctx)
	h.HandleImageGenerations(c)

	if transportSawCanceledContext {
		t.Fatal("the provider request inherited the cancelled downstream context")
	}
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: the provider call must not be cancelled by the client", w.Code)
	}
}

// Video generation was charged before the task was persisted, so a response that
// could not be parsed left the customer charged with a 502 and no task id to poll
// (and a retry charged twice).
func TestVideoTransformFailureIsNotCharged(t *testing.T) {
	repo := edgeBillingRepo(t, &storage.ModelPriceRecord{
		Model: "edge-video-model", GroupName: "default", FixedPrice: 0.25, Currency: "CNY",
	})

	newVideoHandler := func(transport auditReviewTransport) *MultimodalHandler {
		previousClient := provider.SharedDefaultHTTPClient
		provider.SharedDefaultHTTPClient = &http.Client{Transport: transport}
		t.Cleanup(func() { provider.SharedDefaultHTTPClient = previousClient })
		return NewMultimodalHandler(router.NewDispatcher([]model.ChannelConfig{{
			Name: "edge-video", BaseURL: "https://video.invalid", Models: []string{"edge-video-model"},
			Protocols: []string{"videos"}, Status: "active", Weight: 1, Priority: 1,
		}}), repo)
	}
	videoContext := func(body string) (*gin.Context, *httptest.ResponseRecorder) {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest(http.MethodPost, "/v1/videos/generations", strings.NewReader(body))
		c.Set(middleware.ContextKeyAPIKey, "sk-video-edge")
		c.Set(middleware.ContextKeyAPIKeyConfig, &model.APIKeyConfig{Key: "sk-video-edge", GroupName: "default"})
		return c, w
	}

	// Upstream 2xx with a body that cannot be parsed: the transform fails.
	h := newVideoHandler(func(r *http.Request) (*http.Response, error) {
		return auditReviewResponse(http.StatusOK, `not-json-at-all`), nil
	})
	c, w := videoContext(`{"model":"edge-video-model","prompt":"a cat"}`)
	h.HandleVideoGenerations(c)
	if w.Code != http.StatusBadGateway {
		t.Fatalf("status = %d, want 502 when the task cannot be persisted", w.Code)
	}
	for _, row := range edgeUsageRows(t, repo, 1) {
		if row.StatusCode == http.StatusOK {
			t.Fatalf("a failed task was audited as a 200 success: %+v", row)
		}
		if row.Cost != 0 {
			t.Fatalf("a failed task was billed %v", row.Cost)
		}
	}

	// Control: a parseable response is persisted, delivered and billed once.
	h = newVideoHandler(func(r *http.Request) (*http.Response, error) {
		return auditReviewResponse(http.StatusOK, `{"id":"upstream-task-1","status":"queued"}`), nil
	})
	c, w = videoContext(`{"model":"edge-video-model","prompt":"a cat"}`)
	h.HandleVideoGenerations(c)
	if w.Code != http.StatusOK {
		t.Fatalf("control status = %d, want 200", w.Code)
	}
	var billed *storage.UsageLogRecord
	for _, row := range edgeUsageRows(t, repo, 2) {
		if row.StatusCode == http.StatusOK {
			billed = row
		}
	}
	if billed == nil || billed.Cost <= 0 {
		t.Fatalf("a persisted video task was not billed: %+v", billed)
	}
}

// A moderation provider failure used to be reported as 501 "not implemented" —
// a configuration error — and the provider's sanitized status was lost.
func TestModerationUpstreamFailurePropagatesSanitizedStatus(t *testing.T) {
	const secret = "sk-live1234567890abcdef"
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"error":{"message":"Incorrect API key provided: ` + secret + `"}}`))
	}))
	defer upstream.Close()

	repo := edgeBillingRepo(t)
	dispatcher := router.NewDispatcher([]model.ChannelConfig{{
		Name: "edge-moderation-500", Type: model.ProviderOpenAI, BaseURL: upstream.URL,
		Models: []string{"text-moderation-latest"}, Status: "active", Weight: 1, Protocols: []string{"moderation"},
	}})
	handler := NewHandler(dispatcher)

	c, w := moderationContext(t, `{"input":"hello"}`)
	handler.HandleModerations(c)

	if w.Code == http.StatusNotImplemented {
		t.Fatalf("an upstream failure was reported as 501 (configuration error): %s", w.Body.String())
	}
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want the provider's 500", w.Code)
	}
	if strings.Contains(w.Body.String(), secret) {
		t.Fatalf("the provider credential leaked to the client: %s", w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "[REDACTED]") {
		t.Fatalf("expected a redacted message, got %s", w.Body.String())
	}

	// The failure is audited with the sanitized message and the provider status.
	logs := edgeUsageRows(t, repo, 1)
	row := logs[0]
	if row.StatusCode != http.StatusInternalServerError {
		t.Fatalf("audit status = %d, want 500", row.StatusCode)
	}
	if strings.Contains(row.ErrorMessage, secret) {
		t.Fatalf("the provider credential leaked into usage_logs.error_message: %q", row.ErrorMessage)
	}
}

// A successful moderation call costs the gateway money and used to be billed to
// nobody: it is now audited and charged one fixed-price unit.
func TestModerationSuccessIsAuditedAndBilled(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"mod-1","model":"text-moderation-latest","results":[]}`))
	}))
	defer upstream.Close()

	repo := edgeBillingRepo(t, &storage.ModelPriceRecord{
		Model: "text-moderation-latest", GroupName: "default", FixedPrice: 0.001, Currency: "CNY",
	})
	dispatcher := router.NewDispatcher([]model.ChannelConfig{{
		Name: "edge-moderation-ok", Type: model.ProviderOpenAI, BaseURL: upstream.URL,
		Models: []string{"text-moderation-latest"}, Status: "active", Weight: 1, Protocols: []string{"moderation"},
	}})
	handler := NewHandler(dispatcher)

	c, w := moderationContext(t, `{"input":"hello"}`)
	handler.HandleModerations(c)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	logs := edgeUsageRows(t, repo, 1)
	row := logs[0]
	if row.Model != "text-moderation-latest" || row.StatusCode != http.StatusOK {
		t.Fatalf("moderation call was not audited: %+v", row)
	}
	if row.Cost <= 0 {
		t.Fatalf("moderation cost = %v, want the configured fixed price", row.Cost)
	}
	if row.PromptTokens != 0 || row.CompletionTokens != 0 {
		t.Fatalf("moderation tokens = %d/%d, want 0 (the provider reports no usage)", row.PromptTokens, row.CompletionTokens)
	}
}

// usage_logs.error_message is read by operators and exported to CSV: the raw
// upstream text (which routinely echoes the offending credential) must never be
// persisted.
func TestUsageLogErrorMessageIsSanitized(t *testing.T) {
	repo := edgeBillingRepo(t)
	const secret = "sk-live1234567890abcdef"
	c, _ := newTestContext(http.MethodPost, "/v1/chat/completions")

	RecordUsage(c, AuditRecordParams{
		Model:        "sanitize-model",
		StatusCode:   http.StatusBadGateway,
		ErrorMessage: "upstream rejected: Incorrect API key provided: " + secret + " (channel=edge-upstream)",
	})

	logs := edgeUsageRows(t, repo, 1)
	row := logs[0]
	if strings.Contains(row.ErrorMessage, secret) {
		t.Fatalf("secret persisted in the audit trail: %q", row.ErrorMessage)
	}
	if !strings.Contains(row.ErrorMessage, "[REDACTED]") {
		t.Fatalf("error_message = %q, want the credential redacted", row.ErrorMessage)
	}
	// Diagnostics must survive sanitization.
	if !strings.Contains(row.ErrorMessage, "channel=edge-upstream") {
		t.Fatalf("error_message = %q, want the channel diagnostic kept", row.ErrorMessage)
	}
}
