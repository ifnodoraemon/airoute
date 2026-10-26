package api

import (
	"fmt"
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

func TestResolveChatUsageEstimatesWhenUpstreamOmitsUsage(t *testing.T) {
	req := &model.ChatCompletionRequest{
		Model:    "m",
		Messages: []model.ChatMessage{{Role: "user", Content: strings.Repeat("a", 400)}},
	}
	resp := &model.ChatCompletionResponse{
		Choices: []model.ChatCompletionChoice{{
			Message: model.ChatMessage{Content: strings.Repeat("b", 200)},
		}},
	}
	prompt, completion, cached, source := resolveChatUsage(req, resp)
	if prompt == 0 || completion == 0 {
		t.Fatalf("estimated usage = %d/%d, want both positive", prompt, completion)
	}
	if cached != 0 {
		t.Fatalf("cached tokens = %d, want 0", cached)
	}
	if source != usageSourceEstimated {
		t.Fatalf("usage source = %q, want %q", source, usageSourceEstimated)
	}
}

func TestResolveChatUsageKeepsReportedUsage(t *testing.T) {
	req := &model.ChatCompletionRequest{Messages: []model.ChatMessage{{Role: "user", Content: "hi"}}}
	resp := &model.ChatCompletionResponse{
		Choices: []model.ChatCompletionChoice{{Message: model.ChatMessage{Content: "hello"}}},
		Usage:   &model.Usage{PromptTokens: 12, CompletionTokens: 7, PromptTokensDetails: &model.PromptTokensDetails{CachedTokens: 3}},
	}
	prompt, completion, cached, source := resolveChatUsage(req, resp)
	if prompt != 12 || completion != 7 || cached != 3 {
		t.Fatalf("usage = %d/%d/%d, want the upstream values 12/7/3", prompt, completion, cached)
	}
	if source != usageSourceUpstream {
		t.Fatalf("usage source = %q, want %q", source, usageSourceUpstream)
	}
}

func TestResolveEmbeddingAndRerankTokens(t *testing.T) {
	tokens, source := resolveEmbeddingTokens(&model.EmbeddingRequest{Input: []any{strings.Repeat("a", 400)}}, model.EmbeddingUsage{})
	if tokens == 0 || source != usageSourceEstimated {
		t.Fatalf("embedding estimate = %d/%q", tokens, source)
	}
	if tokens, source := resolveEmbeddingTokens(nil, model.EmbeddingUsage{PromptTokens: 9, TotalTokens: 9}); tokens != 9 || source != usageSourceUpstream {
		t.Fatalf("reported embedding usage = %d/%q", tokens, source)
	}
	tokens, source = resolveRerankTokens(&model.RerankRequest{Query: "q", Documents: []any{strings.Repeat("d", 400)}}, model.RerankUsage{})
	if tokens == 0 || source != usageSourceEstimated {
		t.Fatalf("rerank estimate = %d/%q", tokens, source)
	}
}

// End to end: a unary upstream that omits the usage block must still produce a
// billable audit row. Before this, such a request was recorded with 0 tokens and
// cost 0 (148 observed rows, about ¥2.2 unbilled).
func TestUnaryChatWithoutUpstreamUsageIsStillBilled(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"id":"chatcmpl-1","object":"chat.completion","created":1,"model":"m","choices":[{"index":0,"message":{"role":"assistant","content":%q},"finish_reason":"stop"}]}`,
			strings.Repeat("x", 400))
	}))
	defer upstream.Close()

	db, err := storage.OpenDB(filepath.Join(t.TempDir(), "unary.db"))
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

	prevEngine, prevLogger, prevRepo := billing.GlobalEngine, storage.GlobalAsyncLogger, storage.GetGlobalRepository()
	billing.GlobalEngine = billing.NewEngine(repo)
	logger := storage.InitAsyncLogger(repo, 64, 8, 10*time.Millisecond)
	storage.SetGlobalRepository(repo)
	defer func() {
		logger.Stop()
		billing.GlobalEngine = prevEngine
		storage.GlobalAsyncLogger = prevLogger
		storage.SetGlobalRepository(prevRepo)
	}()

	dispatcher := router.NewDispatcher([]model.ChannelConfig{{
		Name: "ch", Type: model.ProviderOpenAI, BaseURL: upstream.URL,
		Models: []string{"m"}, Status: "active", Weight: 1,
	}})
	handler := NewHandler(dispatcher)
	c, _ := newTestContext(http.MethodPost, "/v1/chat/completions")
	req := &model.ChatCompletionRequest{
		Model:    "m",
		Messages: []model.ChatMessage{{Role: "user", Content: strings.Repeat("y", 400)}},
	}
	handler.handleChatUnary(c, c.Request.Context(), req, "sess-unary", time.Now())

	var prompt, completion int
	var cost float64
	var source string
	deadline := time.Now().Add(3 * time.Second)
	for {
		err := db.QueryRow(`SELECT prompt_tokens, completion_tokens, cost, COALESCE(usage_source, '') FROM usage_logs ORDER BY id DESC LIMIT 1`).
			Scan(&prompt, &completion, &cost, &source)
		if err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("no usage row persisted: %v", err)
		}
		time.Sleep(20 * time.Millisecond)
	}

	if prompt == 0 || completion == 0 {
		t.Fatalf("usage row recorded %d prompt / %d completion tokens, want estimated values", prompt, completion)
	}
	if cost <= 0 {
		t.Fatalf("cost = %v, want a positive charge for the estimated usage", cost)
	}
	if source != usageSourceEstimated {
		t.Fatalf("usage_source = %q, want %q", source, usageSourceEstimated)
	}
}
