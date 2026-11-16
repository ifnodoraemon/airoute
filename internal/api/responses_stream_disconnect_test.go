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
	"github.com/ifnodoraemon/airoute/internal/telemetry"
)

// A Responses client that disconnects mid-stream must be billed from the
// provider's drained usage report and recorded as 499, not as a 200 success. The
// adapted stream used to range over the upstream channel with no disconnect
// branch, so an aborted request kept its 200 label and never reported the abort.
func TestResponsesAbortedStreamIsBilledFromDrainedUsage(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		flusher, _ := w.(http.Flusher)
		for i := 0; i < 3; i++ {
			fmt.Fprint(w, `data: {"id":"resp-drain","choices":[{"index":0,"delta":{"content":"tok"}}]}`+"\n\n")
			if flusher != nil {
				flusher.Flush()
			}
			time.Sleep(30 * time.Millisecond)
		}
		fmt.Fprint(w, `data: {"id":"resp-drain","choices":[],"usage":{"prompt_tokens":321,"completion_tokens":7,"total_tokens":328}}`+"\n\n")
		fmt.Fprint(w, "data: [DONE]\n\n")
		if flusher != nil {
			flusher.Flush()
		}
	}))
	defer upstream.Close()

	db, err := storage.OpenDB(filepath.Join(t.TempDir(), "resp-drain.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	repo := storage.NewRepository(db)
	if err := repo.SaveModelPrice(&storage.ModelPriceRecord{
		Model: "m", GroupName: "default",
		PromptPrice: 1, CompletionPrice: 4, Currency: "CNY",
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
		Name: "resp-drain-upstream", Type: model.ProviderOpenAI, BaseURL: upstream.URL,
		Models: []string{"m"}, Status: "active", Weight: 1,
	}})
	handler := NewHandler(dispatcher)

	c, _ := newTestContext(http.MethodPost, "/v1/responses")
	clientCtx, cancelClient := context.WithCancel(c.Request.Context())
	c.Request = c.Request.WithContext(clientCtx)

	upstreamCtx, cancelUpstream := upstreamRequestContext(c)
	defer cancelUpstream()

	req := model.ResponseRequest{Model: "m", Stream: true, Input: "hello"}
	chatReq := &model.ChatCompletionRequest{
		Model:    "m",
		Stream:   true,
		Messages: []model.ChatMessage{{Role: "user", Content: "hello"}},
	}
	beforeDrainedTokens := telemetry.GlobalMetrics.AbortedDrainedCompletionTokens.Load()
	go func() {
		time.Sleep(40 * time.Millisecond)
		cancelClient()
	}()

	handler.handleResponsesStreaming(c, upstreamCtx, req, chatReq, "sess-resp-drain", time.Now())

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
		t.Fatalf("status = %d, want 499 (the customer disconnected)", status)
	}
	if prompt != 321 || completion != 7 {
		t.Fatalf("billed %d/%d tokens, want the provider's 321/7 from the drained usage frame", prompt, completion)
	}
	if source != usageSourceUpstream {
		t.Fatalf("usage_source = %q, want %q (exact, not estimated)", source, usageSourceUpstream)
	}
	// The disconnect branch must mark the drained completion tokens; this is the
	// handler's own contribution (the 499 status itself is also enforced by the
	// audit layer). Without the clientGone path the counter would not move.
	if got := telemetry.GlobalMetrics.AbortedDrainedCompletionTokens.Load(); got != beforeDrainedTokens+7 {
		t.Fatalf("AbortedDrainedCompletionTokens = %d, want %d (drained completion must be marked)", got, beforeDrainedTokens+7)
	}
}