package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ifnodoraemon/airoute/internal/model"
	"github.com/ifnodoraemon/airoute/internal/router"
	"github.com/ifnodoraemon/airoute/internal/storage"
)

func TestGeminiInteractionsStreamPersistsOwnerAndMetersThoughtTokens(t *testing.T) {
	repo := edgeBillingRepo(t, &storage.ModelPriceRecord{Model: "m", GroupName: "default", PromptPrice: 1, CompletionPrice: 2, Currency: "CNY"})
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "event: interaction.created\ndata: {\"event_type\":\"interaction.created\",\"interaction\":{\"id\":\"interaction-one\",\"status\":\"in_progress\"}}\n\n")
		fmt.Fprint(w, "event: step.delta\ndata: {\"event_type\":\"step.delta\",\"index\":0,\"delta\":{\"type\":\"thought_signature\",\"signature\":\"opaque-signature==\"},\"vendor\":9007199254740993}\n\n")
		fmt.Fprint(w, "event: interaction.completed\ndata: {\"event_type\":\"interaction.completed\",\"interaction\":{\"id\":\"interaction-one\",\"status\":\"completed\",\"usage\":{\"total_input_tokens\":20,\"total_output_tokens\":7,\"total_thought_tokens\":3,\"total_cached_tokens\":4,\"total_tokens\":30}}}\n\n")
	}))
	defer server.Close()
	h := NewHandler(router.NewDispatcher([]model.ChannelConfig{{Name: "google", Type: model.ProviderGemini, BaseURL: server.URL, Models: []string{"m"}}}))
	c, w := googleResourceContext(t, http.MethodPost, "/v1beta/interactions", `{"model":"m","input":"hi","stream":true,"generation_config":{"thinking_level":"high"}}`, 101)
	h.HandleGeminiResource(c)
	if w.Code != 200 || !strings.Contains(w.Body.String(), "opaque-signature==") || !strings.Contains(w.Body.String(), "9007199254740993") {
		t.Fatalf("interaction stream changed: %d %s", w.Code, w.Body.String())
	}
	rows := edgeUsageRows(t, repo, 1)
	if rows[0].PromptTokens != 20 || rows[0].CompletionTokens != 10 || rows[0].CachedTokens != 4 {
		t.Fatalf("interaction usage lost: %+v", rows[0])
	}
	c, w = googleResourceContext(t, http.MethodPost, "/v1beta/interactions", `{"model":"m","input":"continue","previous_interaction_id":"interaction-one"}`, 202)
	h.HandleGeminiResource(c)
	if w.Code != 404 || calls.Load() != 1 {
		t.Fatalf("foreign continuation accepted: %d %s", w.Code, w.Body.String())
	}
	jobs, err := repo.PendingNativeJobs(c.Request.Context())
	if err != nil || len(jobs) != 0 {
		t.Fatalf("completed interaction remains pending: %v %+v", err, jobs)
	}
}

func TestGoogleResourceReferenceRejectsForeignCacheBeforeGeneration(t *testing.T) {
	repo := edgeBillingRepo(t)
	if err := repo.SaveNativeState(context.Background(), storage.NativeState{Namespace: "gemini-cache", ID: "cachedContents/private", Owner: "user:101", Identity: "original", Model: "m", Data: []byte(`{}`), ExpiresAt: time.Now().Add(time.Hour)}); err != nil {
		t.Fatal(err)
	}
	c, _ := googleResourceContext(t, http.MethodPost, "/v1beta/models/m:generateContent", `{}`, 202)
	native := &model.NativeRequest{Body: json.RawMessage(`{"contents":[{"parts":[{"text":"hi"}]}],"cachedContent":"cachedContents/private"}`)}
	if err := bindNativeState(c, native); err == nil {
		t.Fatal("foreign cache accepted")
	}
}
