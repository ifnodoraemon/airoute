package api

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/ifnodoraemon/airoute/internal/provider"
	"github.com/ifnodoraemon/airoute/internal/router"
	"github.com/ifnodoraemon/airoute/internal/storage"
)

// Regression test for the "channel: provider" audit rows: the message below
// contains no channel name at all. Recovering one by scanning the text reported
// the English word "provider" as the serving channel.
func TestChannelFromErrorUsesStructuredFactsOnly(t *testing.T) {
	plain := errors.New("no upstream provider available for requested model 'llm-rerank-shim'")

	if got := channelFromError(plain); got != "" {
		t.Errorf("channelFromError(plain dispatch message) = %q, want empty", got)
	}
	if got := channelFromError(fmt.Errorf("no upstream provider available for model 'x' and protocol 'images'")); got != "" {
		t.Errorf("channelFromError(no-route message) = %q, want empty", got)
	}

	t.Run("dispatch failure carries the full trail", func(t *testing.T) {
		de := &router.DispatchError{
			Model: "gpt-4o",
			Trail: "罗总→gpustack-cluster",
			Err:   errors.New("all 2 candidate channels failed"),
		}
		if got := channelFromError(fmt.Errorf("handler: %w", de)); got != "罗总→gpustack-cluster" {
			t.Errorf("channelFromError = %q, want the trail", got)
		}
	})

	t.Run("single upstream rejection names its channel", func(t *testing.T) {
		ue := provider.NewUpstreamError("罗总", http.StatusBadGateway, []byte("boom"))
		if got := channelFromError(ue); got != "罗总" {
			t.Errorf("channelFromError = %q, want %q", got, "罗总")
		}
	})

	t.Run("no route leaves the channel empty", func(t *testing.T) {
		de := &router.DispatchError{Model: "llm-rerank-shim", NoRoute: true, Err: plain}
		if got := channelFromError(de); got != "" {
			t.Errorf("channelFromError(no-route) = %q, want empty", got)
		}
	})
}

func TestResolveUpstreamFailureStatusNoRouteIs404(t *testing.T) {
	de := &router.DispatchError{
		Model:   "llm-rerank-shim",
		NoRoute: true,
		Err:     errors.New("no upstream provider available for requested model 'llm-rerank-shim'"),
	}

	status, errType, errCode, channel := resolveUpstreamFailureStatus(de)
	if status != http.StatusNotFound {
		t.Errorf("status = %d, want %d", status, http.StatusNotFound)
	}
	if errType != "invalid_request_error" || errCode != "model_not_found" {
		t.Errorf("type/code = %s/%s, want invalid_request_error/model_not_found", errType, errCode)
	}
	if channel != "" {
		t.Errorf("channel = %q, want empty", channel)
	}
}

// An exhausted dispatch still reports the upstream status, but the audit column
// keeps the routing trail rather than collapsing to the last channel tried.
func TestResolveUpstreamFailureStatusKeepsTrail(t *testing.T) {
	de := &router.DispatchError{
		Model: "gpt-4o",
		Trail: "罗总→gpustack-cluster",
		Err:   provider.NewUpstreamError("gpustack-cluster", http.StatusBadGateway, []byte("boom")),
	}

	status, _, _, channel := resolveUpstreamFailureStatus(de)
	if status != http.StatusBadGateway {
		t.Errorf("status = %d, want %d", status, http.StatusBadGateway)
	}
	if channel != "罗总→gpustack-cluster" {
		t.Errorf("channel = %q, want the full trail", channel)
	}
}

// End-to-end regression for the "llm-rerank-shim / channel: provider" rows in
// production: an unroutable model must be audited as a 404 with an EMPTY
// channel. It used to record "provider", scraped out of the error sentence
// "no upstream provider available for requested model ...".
func TestUnroutableModelIsAuditedWithEmptyChannel(t *testing.T) {
	db, err := storage.OpenDB(filepath.Join(t.TempDir(), "audit.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	repo := storage.NewRepository(db)
	previousRepo, previousLogger := storage.GetGlobalRepository(), storage.GlobalAsyncLogger
	logger := storage.InitAsyncLogger(repo, 16, 16, time.Hour)
	storage.SetGlobalRepository(repo)
	defer func() {
		logger.Stop()
		storage.GlobalAsyncLogger = previousLogger
		storage.SetGlobalRepository(previousRepo)
	}()

	// No channel serves this model, which is exactly the reported situation.
	handler := NewHandler(router.NewDispatcher(nil))

	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	body := `{"model":"llm-rerank-shim","messages":[{"role":"user","content":"hi"}]}`
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	handler.HandleChatCompletions(c)

	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d (body: %s)", w.Code, http.StatusNotFound, w.Body.String())
	}

	logger.Stop()
	logs, err := repo.ListUsageLogs(10, 0)
	if err != nil {
		t.Fatalf("list usage logs: %v", err)
	}
	if len(logs) != 1 {
		t.Fatalf("got %d audit rows, want 1", len(logs))
	}
	got := logs[0]
	if got.Model != "llm-rerank-shim" {
		t.Errorf("model = %q, want %q", got.Model, "llm-rerank-shim")
	}
	if got.Channel != "" {
		t.Errorf("channel = %q, want empty: the request never reached a channel", got.Channel)
	}
	if got.StatusCode != http.StatusNotFound {
		t.Errorf("status_code = %d, want %d", got.StatusCode, http.StatusNotFound)
	}
	if got.PromptTokens != 0 || got.CompletionTokens != 0 {
		t.Errorf("tokens = %d/%d, want 0/0", got.PromptTokens, got.CompletionTokens)
	}
}
