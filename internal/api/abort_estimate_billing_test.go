package api

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ifnodoraemon/airoute/internal/billing"
	"github.com/ifnodoraemon/airoute/internal/model"
	"github.com/ifnodoraemon/airoute/internal/router"
	"github.com/ifnodoraemon/airoute/internal/storage"
)

// A client that disconnects while the gateway is waiting on the upstream still
// causes real provider cost: the prompt was accepted. Recording such a request as
// zero tokens / zero cost means the gateway absorbs that cost.
func TestClientAbortDuringDispatchBillsEstimatedPrompt(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Consume the body so net/http arms its disconnect detection: otherwise
		// the server only notices the aborted call when this handler returns,
		// which would make the test wait for its own fallback timer.
		_, _ = io.Copy(io.Discard, r.Body)
		select {
		case <-r.Context().Done():
		case <-time.After(2 * time.Second):
		}
	}))
	defer upstream.Close()

	db, err := storage.OpenDB(filepath.Join(t.TempDir(), "abort.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	repo := storage.NewRepository(db)
	if err := repo.SaveModelPrice(&storage.ModelPriceRecord{
		Model: "m", GroupName: "default",
		PromptPrice: 1, CompletionPrice: 4, CacheReadPrice: 0.1, Currency: "CNY",
	}); err != nil {
		t.Fatal(err)
	}

	prevEngine, prevLogger := billing.GlobalEngine, storage.GlobalAsyncLogger
	billing.GlobalEngine = billing.NewEngine(repo)
	logger := storage.InitAsyncLogger(repo, 64, 8, 10*time.Millisecond)
	defer func() {
		logger.Stop()
		billing.GlobalEngine = prevEngine
		storage.GlobalAsyncLogger = prevLogger
	}()

	dispatcher := router.NewDispatcher([]model.ChannelConfig{{
		Name: "slow-upstream", Type: model.ProviderOpenAI, BaseURL: upstream.URL,
		Models: []string{"m"}, Status: "active", Weight: 1,
	}})
	handler := NewHandler(dispatcher)

	c, _ := newTestContext(http.MethodPost, "/v1/chat/completions")
	ctx, cancel := context.WithCancel(c.Request.Context())
	c.Request = c.Request.WithContext(ctx)
	go func() {
		time.Sleep(150 * time.Millisecond)
		cancel()
	}()

	// 4000 characters ≈ 1000 prompt tokens at the shared chars-per-token ratio.
	req := &model.ChatCompletionRequest{
		Model:    "m",
		Messages: []model.ChatMessage{{Role: "user", Content: strings.Repeat("x", 4000)}},
	}
	handler.handleChatUnary(c, ctx, req, "sess-abort-dispatch", time.Now())

	var status, prompt int
	var cost float64
	var source, channel string
	deadline := time.Now().Add(3 * time.Second)
	for {
		err := db.QueryRow(`SELECT status_code, prompt_tokens, cost, COALESCE(usage_source,''), COALESCE(channel,'') FROM usage_logs ORDER BY id DESC LIMIT 1`).
			Scan(&status, &prompt, &cost, &source, &channel)
		if err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("no usage row persisted: %v", err)
		}
		time.Sleep(20 * time.Millisecond)
	}

	if status != statusClientClosed {
		t.Fatalf("status = %d, want %d", status, statusClientClosed)
	}
	if channel != "slow-upstream" {
		t.Fatalf("channel = %q, want the in-flight channel", channel)
	}
	if prompt == 0 {
		t.Fatal("prompt tokens = 0: the provider cost was absorbed by the gateway")
	}
	if cost <= 0 {
		t.Fatalf("cost = %v, want a positive charge for the consumed prompt", cost)
	}
	if source != usageSourceEstimated {
		t.Fatalf("usage_source = %q, want %q", source, usageSourceEstimated)
	}
}
