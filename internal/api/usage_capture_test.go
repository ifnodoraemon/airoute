package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/ifnodoraemon/airoute/internal/model"
	"github.com/ifnodoraemon/airoute/internal/router"
	"github.com/ifnodoraemon/airoute/internal/storage"
)

// Exercise the complete GPUStack -> provider -> handler -> audit database path.
// Input/cache usage may arrive before the final output-only trailer.
func TestGPUStackPartialUsageSurvivesIntoAuditLog(t *testing.T) {
	for _, ingress := range []string{"openai", "anthropic"} {
		t.Run(ingress, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "text/event-stream")
				fmt.Fprint(w, "data: {\"id\":\"gpu\",\"choices\":[],\"usage\":{\"prompt_tokens\":32,\"prompt_tokens_details\":{\"cached_tokens\":16}}}\n\n")
				fmt.Fprint(w, "data: {\"id\":\"gpu\",\"choices\":[{\"index\":0,\"delta\":{\"content\":\"answer\"}}]}\n\n")
				fmt.Fprint(w, `data: {"id":"gpu","choices":[],"usage":{"completion_tokens":8}}`)
			}))
			defer server.Close()
			db, err := storage.OpenDB(filepath.Join(t.TempDir(), "usage.db"))
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			previousRepo, previousLogger := storage.GetGlobalRepository(), storage.GlobalAsyncLogger
			repo := storage.NewRepository(db)
			logger := storage.InitAsyncLogger(repo, 16, 16, time.Hour)
			defer func() {
				logger.Stop()
				storage.GlobalAsyncLogger = previousLogger
				storage.SetGlobalRepository(previousRepo)
			}()
			handler := NewHandler(router.NewDispatcher([]model.ChannelConfig{{
				Name: "gpustack", Type: model.ProviderGPUStack, BaseURL: server.URL,
				Models: []string{"m"}, Status: "active", Weight: 1, Protocols: []string{"openai_chat"},
			}}))
			c, _ := newTestContext(http.MethodPost, "/v1/chat/completions")
			req := &model.ChatCompletionRequest{Model: "m", Stream: true, Messages: []model.ChatMessage{{Role: "user", Content: "hello"}}}
			if ingress == "openai" {
				handler.handleChatStream(c, c.Request.Context(), req, "test", time.Now())
			} else {
				handler.handleAnthropicStream(c, c.Request.Context(), &AnthropicInboundRequest{Model: "m", Stream: true}, req, "test", time.Now())
			}
			logger.Stop()
			logs, err := repo.ListUsageLogs(50, 0)
			if err != nil || len(logs) != 1 {
				t.Fatalf("logs=%v err=%v", logs, err)
			}
			got := logs[0]
			if got.PromptTokens != 32 || got.CachedTokens != 16 || got.CompletionTokens != 8 || got.UsageSource != "upstream" || got.Channel != "gpustack" {
				t.Fatalf("usage lost in audit: %+v", got)
			}
		})
	}
}

// A usage-only SSE frame (empty choices array) is what OpenAI-compatible
// upstreams send last when stream_options.include_usage is set. It must be
// parsed into the canonical Usage so prompt/cached tokens are recorded.
func TestUsageOnlyChunkParses(t *testing.T) {
	raw := `{"id":"x","object":"chat.completion.chunk","created":1,"model":"m","choices":[],` +
		`"usage":{"prompt_tokens":32,"completion_tokens":28,"total_tokens":60,` +
		`"prompt_tokens_details":{"cached_tokens":16}}}`

	var chunk model.ChatCompletionChunk
	if err := json.Unmarshal([]byte(raw), &chunk); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(chunk.Choices) != 0 {
		t.Fatalf("expected empty choices, got %d", len(chunk.Choices))
	}
	if chunk.Usage == nil {
		t.Fatal("expected usage to be populated on an empty-choices frame")
	}
	if chunk.Usage.PromptTokens != 32 || chunk.Usage.CompletionTokens != 28 {
		t.Fatalf("unexpected usage: %+v", chunk.Usage)
	}
	if got := chunk.Usage.GetCachedTokens(); got != 16 {
		t.Fatalf("cached tokens = %d, want 16", got)
	}
}

// The canonical request built from an Anthropic stream must opt into usage
// reporting, otherwise OpenAI-compatible upstreams never send the final frame.
func TestAnthropicStreamRequestsUsage(t *testing.T) {
	req := &model.AnthropicInboundRequest{
		Model:     "m",
		Stream:    true,
		MaxTokens: 16,
		Messages:  []model.AnthropicInboundMessage{{Role: "user", Content: "hi"}},
	}
	canonical := ConvertAnthropicToCanonical(req)
	if canonical.StreamOptions == nil || !canonical.StreamOptions.IncludeUsage {
		t.Fatalf("expected include_usage to be requested, got %+v", canonical.StreamOptions)
	}

	// Non-streaming requests do not need stream_options.
	req.Stream = false
	canonical = ConvertAnthropicToCanonical(req)
	if canonical.StreamOptions != nil {
		t.Fatalf("expected no stream_options for unary requests, got %+v", canonical.StreamOptions)
	}
}
