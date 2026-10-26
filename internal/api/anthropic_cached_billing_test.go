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

// Native Anthropic unary responses report prompt-cache reads separately from
// input_tokens. The canonical usage therefore carries total prompt tokens plus
// PromptTokensDetails.CachedTokens, and the handler must forward the cached
// count: billing all prompt tokens at the full prompt rate overcharged cached
// prompts by roughly 90% of the cache discount (about 4.5x here).
func TestNativeAnthropicUnaryBillsCachedTokensAtCacheRate(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"id":"msg_1","type":"message","role":"assistant","model":"m",`+
			`"content":[{"type":"text","text":"hi"}],"stop_reason":"end_turn",`+
			`"usage":{"input_tokens":100,"cache_read_input_tokens":900,"output_tokens":10}}`)
	}))
	defer upstream.Close()

	db, err := storage.OpenDB(filepath.Join(t.TempDir(), "anthropic.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	repo := storage.NewRepository(db)
	if err := repo.SaveModelPrice(&storage.ModelPriceRecord{
		Model: "m", GroupName: "default",
		PromptPrice: 2, CompletionPrice: 8, CacheReadPrice: 0.2, Currency: "CNY",
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
		Name: "native-anthropic", Type: model.ProviderAnthropic, BaseURL: upstream.URL,
		Models: []string{"m"}, Status: "active", Weight: 1,
		Protocols: []string{"anthropic_messages"},
	}})
	handler := NewHandler(dispatcher)

	inbound := &model.AnthropicInboundRequest{
		Model:     "m",
		MaxTokens: 16,
		Messages:  []model.AnthropicInboundMessage{{Role: "user", Content: "hi"}},
	}
	// AnthropicOriginal makes the provider use the native passthrough and attach
	// the raw response, which is the branch under test.
	canonical := &model.ChatCompletionRequest{Model: "m", AnthropicOriginal: inbound}

	c, _ := newTestContext(http.MethodPost, "/v1/messages")
	handler.handleAnthropicUnary(c, c.Request.Context(), inbound, canonical, "sess-anthropic", time.Now())

	var cached int
	var cost float64
	var prompt int
	deadline := time.Now().Add(3 * time.Second)
	for {
		err := db.QueryRow(`SELECT COALESCE(cached_tokens, 0), prompt_tokens, cost FROM usage_logs ORDER BY id DESC LIMIT 1`).
			Scan(&cached, &prompt, &cost)
		if err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("no usage row persisted: %v", err)
		}
		time.Sleep(20 * time.Millisecond)
	}

	if prompt != 1000 || cached != 900 {
		t.Fatalf("row recorded prompt=%d cached=%d, want prompt=1000 cached=900", prompt, cached)
	}
	// (100 uncached * 2 + 900 cached * 0.2 + 10 completion * 8) / 1e6
	want := 0.00046
	if cost < want-1e-9 || cost > want+1e-9 {
		t.Fatalf("cost = %v, want %v (cached tokens billed at the cache rate)", cost, want)
	}
	if strings.Contains(c.Writer.Header().Get("X-Airoute-Billing"), "error") {
		t.Fatal("unexpected billing header state")
	}
}
