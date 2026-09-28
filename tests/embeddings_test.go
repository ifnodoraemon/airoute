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
	"github.com/ifnodoraemon/airoute/internal/config"
	"github.com/ifnodoraemon/airoute/internal/controlplane"
	"github.com/ifnodoraemon/airoute/internal/model"
	"github.com/ifnodoraemon/airoute/internal/router"
	"github.com/ifnodoraemon/airoute/internal/storage"
)

func TestAPI_Embeddings_OpenAICompatible(t *testing.T) {
	gin.SetMode(gin.TestMode)

	// Upstream mock server
	var receivedUpstreamModel string
	var receivedAuthHeader string
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/embeddings" {
			http.NotFound(w, r)
			return
		}
		receivedAuthHeader = r.Header.Get("Authorization")
		body, _ := io.ReadAll(r.Body)
		var parsed struct {
			Model string `json:"model"`
			Input any    `json:"input"`
		}
		_ = json.Unmarshal(body, &parsed)
		receivedUpstreamModel = parsed.Model

		resp := model.EmbeddingResponse{
			Object: "list",
			Data: []model.EmbeddingItem{
				{
					Object:    "embedding",
					Index:     0,
					Embedding: []float64{0.0023, -0.015, 0.045, 0.088},
				},
			},
			Model: parsed.Model,
			Usage: model.EmbeddingUsage{
				PromptTokens: 8,
				TotalTokens:  8,
			},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer mockServer.Close()

	dispatcher, adminHandler, repo := setupTestEnvironment(t)

	ch := storage.ChannelRecord{
		Name:      "openai-embedding-node",
		Type:      model.ProviderOpenAI,
		BaseURL:   mockServer.URL + "/v1",
		APIKey:    "test-upstream-key",
		Models:    []string{"text-embedding-3-small", "bge-m3"},
		Protocols: []string{"embeddings"},
		ModelMapping: map[string]string{
			"text-embedding-3-small": "bge-m3",
		},
		Status:   "active",
		Priority: 1,
	}
	_ = repo.CreateChannel(&ch)

	vKey := storage.APIKeyRecord{
		Key:           "sk-nano-embed-key",
		TenantID:      "tenant-embed",
		AllowedModels: []string{"*"},
		Status:        "active",
	}
	_ = repo.CreateAPIKey(&vKey)

	// Update dispatcher with channel
	dispatcher.UpdateChannels([]model.ChannelConfig{
		{
			Name:         ch.Name,
			Type:         ch.Type,
			BaseURL:      ch.BaseURL,
			APIKey:       ch.APIKey,
			Models:       ch.Models,
			Protocols:    ch.Protocols,
			ModelMapping: ch.ModelMapping,
			Priority:     ch.Priority,
		},
	})

	engine := api.SetupRouter(dispatcher, adminHandler)

	reqPayload := model.EmbeddingRequest{
		Input: "The quick brown fox jumps over the lazy dog",
		Model: "text-embedding-3-small",
	}
	reqBytes, _ := json.Marshal(reqPayload)

	httpReq, _ := http.NewRequest(http.MethodPost, "/v1/embeddings", bytes.NewReader(reqBytes))
	httpReq.Header.Set("Authorization", "Bearer sk-nano-embed-key")
	httpReq.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()
	engine.ServeHTTP(w, httpReq)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d: %s", w.Code, w.Body.String())
	}

	if receivedUpstreamModel != "bge-m3" {
		t.Errorf("expected mapped model 'bge-m3', got '%s'", receivedUpstreamModel)
	}
	if receivedAuthHeader != "Bearer test-upstream-key" {
		t.Errorf("expected upstream auth 'Bearer test-upstream-key', got '%s'", receivedAuthHeader)
	}

	var embResp model.EmbeddingResponse
	if err := json.Unmarshal(w.Body.Bytes(), &embResp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if embResp.Model != "text-embedding-3-small" {
		t.Errorf("expected response model 'text-embedding-3-small', got '%s'", embResp.Model)
	}
	if len(embResp.Data) != 1 || len(embResp.Data[0].Embedding) != 4 {
		t.Errorf("unexpected embedding data structure: %+v", embResp.Data)
	}
	if embResp.Usage.PromptTokens != 8 {
		t.Errorf("expected prompt tokens 8, got %d", embResp.Usage.PromptTokens)
	}
}

func TestAPI_Embeddings_GeminiNative(t *testing.T) {
	gin.SetMode(gin.TestMode)

	// Mock Gemini endpoint
	var singleCalled bool
	var batchCalled bool
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/v1beta/models/text-embedding-004:embedContent" {
			singleCalled = true
			resp := map[string]any{
				"embedding": map[string]any{
					"values": []float64{0.1, 0.2, 0.3},
				},
			}
			_ = json.NewEncoder(w).Encode(resp)
			return
		}
		if r.URL.Path == "/v1beta/models/text-embedding-004:batchEmbedContents" {
			batchCalled = true
			resp := map[string]any{
				"embeddings": []map[string]any{
					{"values": []float64{0.1, 0.2, 0.3}},
					{"values": []float64{0.4, 0.5, 0.6}},
				},
			}
			_ = json.NewEncoder(w).Encode(resp)
			return
		}
		http.NotFound(w, r)
	}))
	defer mockServer.Close()

	ch := model.ChannelConfig{
		Name:      "gemini-embed-channel",
		Type:      model.ProviderGemini,
		BaseURL:   mockServer.URL,
		APIKey:    "test-gemini-key",
		Models:    []string{"text-embedding-004"},
		Protocols: []string{"embeddings"},
		Priority:  1,
	}

	dispatcher := router.NewDispatcher([]model.ChannelConfig{ch})
	engine := api.SetupRouter(dispatcher, nil)

	// Single input test
	reqSingle := model.EmbeddingRequest{
		Input: "Hello Gemini Embedding",
		Model: "text-embedding-004",
	}
	sBytes, _ := json.Marshal(reqSingle)
	httpReq1, _ := http.NewRequest(http.MethodPost, "/v1/embeddings", bytes.NewReader(sBytes))
	httpReq1.Header.Set("Authorization", "Bearer sk-test-any")
	httpReq1.Header.Set("Content-Type", "application/json")
	w1 := httptest.NewRecorder()
	engine.ServeHTTP(w1, httpReq1)

	if w1.Code != http.StatusOK {
		t.Fatalf("expected single embed 200 OK, got %d: %s", w1.Code, w1.Body.String())
	}
	if !singleCalled {
		t.Errorf("expected single embedContent endpoint to be called")
	}

	var resp1 model.EmbeddingResponse
	_ = json.Unmarshal(w1.Body.Bytes(), &resp1)
	if len(resp1.Data) != 1 || len(resp1.Data[0].Embedding) != 3 {
		t.Errorf("unexpected single embedding result: %+v", resp1)
	}

	// Batch input test
	reqBatch := model.EmbeddingRequest{
		Input: []string{"Text 1", "Text 2"},
		Model: "text-embedding-004",
	}
	bBytes, _ := json.Marshal(reqBatch)
	httpReq2, _ := http.NewRequest(http.MethodPost, "/v1/embeddings", bytes.NewReader(bBytes))
	httpReq2.Header.Set("Authorization", "Bearer sk-test-any")
	httpReq2.Header.Set("Content-Type", "application/json")
	w2 := httptest.NewRecorder()
	engine.ServeHTTP(w2, httpReq2)

	if w2.Code != http.StatusOK {
		t.Fatalf("expected batch embed 200 OK, got %d: %s", w2.Code, w2.Body.String())
	}
	if !batchCalled {
		t.Errorf("expected batch batchEmbedContents endpoint to be called")
	}

	var resp2 model.EmbeddingResponse
	_ = json.Unmarshal(w2.Body.Bytes(), &resp2)
	if len(resp2.Data) != 2 {
		t.Errorf("expected 2 batch embeddings, got %d", len(resp2.Data))
	}
}

func TestAPI_Embeddings_SafeFallback(t *testing.T) {
	gin.SetMode(gin.TestMode)

	// Primary faulty server returns 500
	faultyServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"error":"CUDA out of memory in embedding engine"}`, http.StatusInternalServerError)
	}))
	defer faultyServer.Close()

	// Secondary backup server succeeds
	backupServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := model.EmbeddingResponse{
			Object: "list",
			Data: []model.EmbeddingItem{
				{Index: 0, Embedding: []float64{0.99, 0.88}},
			},
			Model: "text-embedding-3-small",
			Usage: model.EmbeddingUsage{PromptTokens: 5, TotalTokens: 5},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer backupServer.Close()

	ch1 := model.ChannelConfig{
		Name:      "faulty-primary",
		Type:      model.ProviderOpenAI,
		BaseURL:   faultyServer.URL,
		Models:    []string{"text-embedding-3-small"},
		Protocols: []string{"embeddings"},
		Priority:  1,
	}

	ch2 := model.ChannelConfig{
		Name:      "backup-secondary",
		Type:      model.ProviderOpenAI,
		BaseURL:   backupServer.URL,
		Models:    []string{"text-embedding-3-small"},
		Protocols: []string{"embeddings"},
		Priority:  2,
	}

	dispatcher := router.NewDispatcher([]model.ChannelConfig{ch1, ch2})
	engine := api.SetupRouter(dispatcher, nil)

	reqPayload := model.EmbeddingRequest{
		Input: "Fallback test sentence",
		Model: "text-embedding-3-small",
	}
	payloadBytes, _ := json.Marshal(reqPayload)

	httpReq, _ := http.NewRequest(http.MethodPost, "/v1/embeddings", bytes.NewReader(payloadBytes))
	httpReq.Header.Set("Authorization", "Bearer sk-test")
	httpReq.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()
	engine.ServeHTTP(w, httpReq)

	if w.Code != http.StatusOK {
		t.Fatalf("expected fallback to succeed with 200 OK, got %d: %s", w.Code, w.Body.String())
	}

	var resp model.EmbeddingResponse
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if len(resp.Data) != 1 || resp.Data[0].Embedding[0] != 0.99 {
		t.Errorf("expected response from secondary backup channel, got %+v", resp)
	}
}

func TestAPI_Embeddings_ForbiddenModel(t *testing.T) {
	gin.SetMode(gin.TestMode)

	testCfg := &config.Config{
		APIKeys: []model.APIKeyConfig{
			{
				Key:           "sk-restricted-key",
				TenantID:      "tenant-r",
				AllowedModels: []string{"bge-m3"},
			},
		},
		Channels: []model.ChannelConfig{
			{
				Name:      "ch-embed",
				Type:      model.ProviderOpenAI,
				Models:    []string{"text-embedding-3-small", "bge-m3"},
				Protocols: []string{"embeddings"},
			},
		},
	}
	config.SetGlobalConfig(testCfg)

	dispatcher := router.NewDispatcher(testCfg.Channels)
	engine := api.SetupRouter(dispatcher, nil)

	reqPayload := model.EmbeddingRequest{
		Input: "Hello forbidden model",
		Model: "text-embedding-3-small",
	}
	reqBytes, _ := json.Marshal(reqPayload)

	httpReq, _ := http.NewRequest(http.MethodPost, "/v1/embeddings", bytes.NewReader(reqBytes))
	httpReq.Header.Set("Authorization", "Bearer sk-restricted-key")
	httpReq.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()
	engine.ServeHTTP(w, httpReq)

	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403 Forbidden, got %d: %s", w.Code, w.Body.String())
	}
}

func TestAutoProbe_EmbeddingsDetection(t *testing.T) {
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Server", "vLLM-Engine")
		resp := map[string]any{
			"data": []map[string]any{
				{"id": "bge-large-zh-v1.5"},
				{"id": "text-embedding-3-small"},
				{"id": "nomic-embed-text"},
			},
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer mockServer.Close()

	prober := controlplane.NewDownstreamProber(mockServer.Client())
	result, err := prober.Probe(t.Context(), &controlplane.ProbeRequest{
		BaseURL: mockServer.URL,
		APIKey:  "none",
	})
	if err != nil {
		t.Fatalf("probe failed: %v", err)
	}

	foundEmbeddings := false
	for _, p := range result.Protocols {
		if p == "embeddings" {
			foundEmbeddings = true
			break
		}
	}

	if !foundEmbeddings {
		t.Errorf("expected 'embeddings' protocol to be inferred for embedding models, got: %v", result.Protocols)
	}
}
