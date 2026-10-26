package api

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/ifnodoraemon/airoute/internal/billing"
	"github.com/ifnodoraemon/airoute/internal/model"
	"github.com/ifnodoraemon/airoute/internal/router"
	"github.com/ifnodoraemon/airoute/internal/storage"
)

func TestUpstreamContextSurvivesDownstreamCancel(t *testing.T) {
	c, _ := newTestContext(http.MethodPost, "/v1/chat/completions")
	clientCtx, cancelClient := context.WithCancel(c.Request.Context())
	c.Request = c.Request.WithContext(clientCtx)

	upstreamCtx, cancelUpstream := upstreamRequestContext(c)
	defer cancelUpstream()

	cancelClient()
	// Give the client cancellation a chance to propagate.
	time.Sleep(20 * time.Millisecond)

	if upstreamCtx.Err() != nil {
		t.Fatalf("upstream context was cancelled with the downstream (%v): the provider call would be aborted before its usage arrives", upstreamCtx.Err())
	}
}

// A customer that disconnects mid-stream must still be billed from the provider's
// own usage report, not from the local estimate: keep the provider stream alive,
// drain it, and record what it says.
func TestAbortedStreamIsBilledFromDrainedUsage(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		flusher, _ := w.(http.Flusher)
		for i := 0; i < 3; i++ {
			fmt.Fprint(w, `data: {"id":"drain-1","choices":[{"index":0,"delta":{"content":"tok"}}]}`+"\n\n")
			if flusher != nil {
				flusher.Flush()
			}
			time.Sleep(30 * time.Millisecond)
		}
		// Final frame carries the authoritative usage, as providers do.
		fmt.Fprint(w, `data: {"id":"drain-1","choices":[],"usage":{"prompt_tokens":321,"completion_tokens":7,"total_tokens":328}}`+"\n\n")
		fmt.Fprint(w, "data: [DONE]\n\n")
		if flusher != nil {
			flusher.Flush()
		}
	}))
	defer upstream.Close()

	db, err := storage.OpenDB(filepath.Join(t.TempDir(), "drain.db"))
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
		Name: "drain-upstream", Type: model.ProviderOpenAI, BaseURL: upstream.URL,
		Models: []string{"m"}, Status: "active", Weight: 1,
	}})
	handler := NewHandler(dispatcher)

	c, _ := newTestContext(http.MethodPost, "/v1/chat/completions")
	clientCtx, cancelClient := context.WithCancel(c.Request.Context())
	c.Request = c.Request.WithContext(clientCtx)

	upstreamCtx, cancelUpstream := upstreamRequestContext(c)
	defer cancelUpstream()

	req := &model.ChatCompletionRequest{
		Model:    "m",
		Stream:   true,
		Messages: []model.ChatMessage{{Role: "user", Content: "hello"}},
	}
	go func() {
		time.Sleep(40 * time.Millisecond)
		cancelClient()
	}()

	handler.handleChatStream(c, upstreamCtx, req, "sess-drain", time.Now())

	var status, prompt, completion int
	var source string
	deadline := time.Now().Add(3 * time.Second)
	for {
		err := db.QueryRow(`SELECT status_code, prompt_tokens, completion_tokens, COALESCE(usage_source,'') FROM usage_logs ORDER BY id DESC LIMIT 1`).
			Scan(&status, &prompt, &completion, &source)
		if err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("no usage row: %v", err)
		}
		time.Sleep(20 * time.Millisecond)
	}

	if status != statusClientClosed {
		t.Fatalf("status = %d, want 499 (the customer did disconnect)", status)
	}
	if prompt != 321 || completion != 7 {
		t.Fatalf("billed %d/%d tokens, want the provider's 321/7 from the drained usage frame", prompt, completion)
	}
	if source != usageSourceUpstream {
		t.Fatalf("usage_source = %q, want %q (exact, not estimated)", source, usageSourceUpstream)
	}
}
