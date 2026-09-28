package tests

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/ifnodoraemon/airoute/internal/api"
	"github.com/ifnodoraemon/airoute/internal/controlplane"
	"github.com/ifnodoraemon/airoute/internal/model"
	"github.com/ifnodoraemon/airoute/internal/router"
)

func TestAPI_Rerank_SuccessAndFallback(t *testing.T) {
	gin.SetMode(gin.TestMode)

	// Primary mock server fails with 503
	faultyServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"error":"rerank model busy"}`, http.StatusServiceUnavailable)
	}))
	defer faultyServer.Close()

	// Backup mock server responds with re-scored items
	var receivedModel string
	backupServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/rerank" && r.URL.Path != "/rerank" {
			http.NotFound(w, r)
			return
		}
		body, _ := io.ReadAll(r.Body)
		var req model.RerankRequest
		_ = json.Unmarshal(body, &req)
		receivedModel = req.Model

		doc1 := &model.RerankResultDocument{Text: "Deep learning is a subset of machine learning"}
		doc2 := &model.RerankResultDocument{Text: "Neural networks with deep architectures"}

		resp := model.RerankResponse{
			Model: req.Model,
			Results: []model.RerankItem{
				{Index: 0, RelevanceScore: 0.982, Document: doc1},
				{Index: 1, RelevanceScore: 0.875, Document: doc2},
			},
			Usage: model.RerankUsage{TotalTokens: 25},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer backupServer.Close()

	ch1 := model.ChannelConfig{
		Name:      "rerank-primary",
		Type:      model.ProviderOpenAI,
		BaseURL:   faultyServer.URL,
		Models:    []string{"bge-reranker-large"},
		Protocols: []string{"rerank"},
		Priority:  1,
	}

	ch2 := model.ChannelConfig{
		Name:      "rerank-secondary",
		Type:      model.ProviderOpenAI,
		BaseURL:   backupServer.URL,
		Models:    []string{"bge-reranker-large"},
		ModelMapping: map[string]string{
			"bge-reranker-large": "BAAI/bge-reranker-v2-m3",
		},
		Protocols: []string{"rerank"},
		Priority:  2,
	}

	dispatcher := router.NewDispatcher([]model.ChannelConfig{ch1, ch2})
	engine := api.SetupRouter(dispatcher, nil)

	reqPayload := model.RerankRequest{
		Model: "bge-reranker-large",
		Query: "What is deep learning?",
		Documents: []any{
			"Deep learning is a subset of machine learning",
			"Neural networks with deep architectures",
		},
	}
	reqBytes, _ := json.Marshal(reqPayload)

	httpReq, _ := http.NewRequest(http.MethodPost, "/v1/rerank", bytes.NewReader(reqBytes))
	httpReq.Header.Set("Authorization", "Bearer sk-test")
	httpReq.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()
	engine.ServeHTTP(w, httpReq)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 OK from rerank fallback, got %d: %s", w.Code, w.Body.String())
	}

	if receivedModel != "BAAI/bge-reranker-v2-m3" {
		t.Errorf("expected mapped model 'BAAI/bge-reranker-v2-m3', got '%s'", receivedModel)
	}

	var resp model.RerankResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode rerank response: %v", err)
	}

	if resp.Model != "bge-reranker-large" {
		t.Errorf("expected user model 'bge-reranker-large', got '%s'", resp.Model)
	}
	if len(resp.Results) != 2 || resp.Results[0].RelevanceScore < 0.9 {
		t.Errorf("unexpected results structure: %+v", resp.Results)
	}
}

func TestAPI_AnthropicCountTokens(t *testing.T) {
	gin.SetMode(gin.TestMode)

	dispatcher := router.NewDispatcher(nil)
	engine := api.SetupRouter(dispatcher, nil)

	reqBody := []byte(`{
		"model": "claude-3-5-sonnet",
		"messages": [
			{"role": "user", "content": "How many tokens are in this sentence?"}
		],
		"system": "You are a concise AI assistant."
	}`)

	httpReq, _ := http.NewRequest(http.MethodPost, "/v1/messages/count_tokens", bytes.NewReader(reqBody))
	httpReq.Header.Set("Authorization", "Bearer sk-test")
	httpReq.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()
	engine.ServeHTTP(w, httpReq)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for /v1/messages/count_tokens, got %d: %s", w.Code, w.Body.String())
	}

	var parsed struct {
		InputTokens int `json:"input_tokens"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &parsed)
	if parsed.InputTokens <= 0 {
		t.Errorf("expected positive input_tokens count, got %d", parsed.InputTokens)
	}
}

func TestAutoProbe_ActiveRerankAndSchemeNormalization(t *testing.T) {
	// Server responds to /v1/models with standard chat model
	// and responds to /v1/rerank with 400 Bad Request (confirming route is active)
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/models" || r.URL.Path == "/models" {
			w.Header().Set("Content-Type", "application/json")
			w.Header().Set("Server", "TEI/v1.4.0")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"data": []map[string]string{
					{"id": "bge-reranker-v2-m3"},
				},
			})
			return
		}
		if r.URL.Path == "/v1/rerank" || r.URL.Path == "/rerank" {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"error":"validation_error"}`))
			return
		}
		http.NotFound(w, r)
	}))
	defer mockServer.Close()

	prober := controlplane.NewDownstreamProber(mockServer.Client())
	// Test probing without scheme (raw host:port)
	rawHost := mockServer.Listener.Addr().String()
	res, err := prober.Probe(t.Context(), &controlplane.ProbeRequest{
		BaseURL: rawHost,
		APIKey:  "test-key",
	})
	if err != nil {
		t.Fatalf("Probe failed: %v", err)
	}

	hasRerank := false
	for _, p := range res.Protocols {
		if p == "rerank" {
			hasRerank = true
			break
		}
	}

	if !hasRerank {
		t.Errorf("expected 'rerank' protocol to be automatically detected, got: %v", res.Protocols)
	}
}
