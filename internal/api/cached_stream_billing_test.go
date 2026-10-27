package api

import (
	"fmt"
	"math"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/ifnodoraemon/airoute/internal/model"
	"github.com/ifnodoraemon/airoute/internal/router"
	"github.com/ifnodoraemon/airoute/internal/storage"
)

// Both gateway-owned streaming entrypoints (legacy /v1/completions and Gemini
// streamGenerateContent) read only PromptTokens/CompletionTokens from the
// upstream usage frame and dropped PromptTokensDetails.CachedTokens. The cached
// share of the prompt was therefore billed at the full prompt rate: 900 cached
// tokens cost 0.002080 instead of 0.000460 at prompt=2, cache_read=0.2,
// completion=8 (all per 1M tokens).
func TestStreamingEntriesBillCachedTokensAtCacheRate(t *testing.T) {
	const modelName = "cached-stream-model"

	// prompt_tokens includes the cached reads, exactly like OpenAI-compatible
	// upstreams report them.
	sseBody := `data: {"id":"s1","object":"chat.completion.chunk","created":1,"model":"cached-stream-model","choices":[{"index":0,"delta":{"content":"hi"},"finish_reason":null}]}

data: {"id":"s1","object":"chat.completion.chunk","created":1,"model":"cached-stream-model","choices":[],"usage":{"prompt_tokens":1000,"completion_tokens":10,"total_tokens":1010,"prompt_tokens_details":{"cached_tokens":900}}}

data: [DONE]

`

	assertCachedCost := func(t *testing.T, repo *storage.Repository) {
		t.Helper()
		rows := edgeUsageRows(t, repo, 1)
		row := rows[0]
		if row.PromptTokens != 1000 || row.CachedTokens != 900 || row.CompletionTokens != 10 {
			t.Fatalf("audit row recorded prompt=%d cached=%d completion=%d, want 1000/900/10 (cached tokens were dropped before billing)",
				row.PromptTokens, row.CachedTokens, row.CompletionTokens)
		}
		// (100 uncached * 2 + 900 cached * 0.2 + 10 completion * 8) / 1e6
		const want = 0.00046
		if math.Abs(row.Cost-want) > 1e-9 {
			t.Fatalf("cost = %v, want %v (cached prompt tokens must bill at the cache rate; full prompt rate would be 0.002080)", row.Cost, want)
		}
	}

	newFixture := func(t *testing.T) (*Handler, *storage.Repository) {
		t.Helper()
		repo := edgeBillingRepo(t, &storage.ModelPriceRecord{
			Model: modelName, GroupName: "default",
			PromptPrice: 2, CompletionPrice: 8, CacheReadPrice: 0.2, Currency: "CNY",
		})
		upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/event-stream")
			w.WriteHeader(http.StatusOK)
			fmt.Fprint(w, sseBody)
		}))
		t.Cleanup(upstream.Close)
		dispatcher := router.NewDispatcher([]model.ChannelConfig{{
			Name: "cached-stream", Type: model.ProviderOpenAI, BaseURL: upstream.URL,
			Models: []string{modelName}, Status: "active", Weight: 1,
		}})
		return NewHandler(dispatcher), repo
	}

	t.Run("gemini", func(t *testing.T) {
		handler, repo := newFixture(t)
		canonical := &model.ChatCompletionRequest{
			Model:    modelName,
			Stream:   true,
			Messages: []model.ChatMessage{{Role: "user", Content: "hello"}},
		}
		c, _ := newTestContext(http.MethodPost, "/v1beta/models/"+modelName+":streamGenerateContent")
		handler.handleGeminiStream(c, c.Request.Context(), canonical, modelName, "sess-gemini", time.Now())
		assertCachedCost(t, repo)
	})

	t.Run("completions", func(t *testing.T) {
		handler, repo := newFixture(t)
		chatReq := model.ChatCompletionRequest{
			Model:    modelName,
			Stream:   true,
			Messages: []model.ChatMessage{{Role: "user", Content: "hello"}},
			Protocol: "openai_text",
		}
		req := LegacyCompletionRequest{Model: modelName, Prompt: "hello", Stream: true}
		c, _ := newTestContext(http.MethodPost, "/v1/completions")
		handler.handleCompletionsStream(c, c.Request.Context(), req, chatReq, "sess-completions", time.Now())
		assertCachedCost(t, repo)
	})
}
