package tests

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/ifnodoraemon/nano-gateway/internal/api"
	"github.com/ifnodoraemon/nano-gateway/internal/config"
	"github.com/ifnodoraemon/nano-gateway/internal/controlplane"
	"github.com/ifnodoraemon/nano-gateway/internal/model"
	"github.com/ifnodoraemon/nano-gateway/internal/router"
	"github.com/ifnodoraemon/nano-gateway/internal/storage"
)

func setupTestEnvironment(t *testing.T) (*router.Dispatcher, *controlplane.AdminHandler, *storage.Repository) {
	db, err := storage.OpenDB(":memory:")
	if err != nil {
		t.Fatalf("failed to open in-memory db: %v", err)
	}

	repo := storage.NewRepository(db)
	dispatcher := router.NewDispatcher(nil)
	sync := controlplane.NewSynchronizer(repo, dispatcher)
	adminHandler := controlplane.NewAdminHandler(repo, sync, dispatcher)

	return dispatcher, adminHandler, repo
}

func TestAPI_HealthMetricsAndWebUI(t *testing.T) {
	dispatcher, adminHandler, _ := setupTestEnvironment(t)
	engine := api.SetupRouter(dispatcher, adminHandler)

	// 1. Test GET /health
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for /health, got %d", w.Code)
	}

	// 2. Test GET /metrics (temporarily disabled for public access -> returns 404)
	reqMetrics := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	wMetrics := httptest.NewRecorder()
	engine.ServeHTTP(wMetrics, reqMetrics)

	if wMetrics.Code != http.StatusNotFound {
		t.Fatalf("expected 404 NotFound for public /metrics (temporarily disabled), got %d", wMetrics.Code)
	}

	// 2b. Test GET /api/v1/public/status (standalone public status page API)
	reqStatus := httptest.NewRequest(http.MethodGet, "/api/v1/public/status", nil)
	wStatus := httptest.NewRecorder()
	engine.ServeHTTP(wStatus, reqStatus)

	if wStatus.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for /api/v1/public/status, got %d", wStatus.Code)
	}

	// 3. Test GET /ui/ (embedded Web UI)
	reqUI := httptest.NewRequest(http.MethodGet, "/ui/", nil)
	wUI := httptest.NewRecorder()
	engine.ServeHTTP(wUI, reqUI)

	if wUI.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for embedded Web UI /ui/, got %d", wUI.Code)
	}
	if !bytes.Contains(wUI.Body.Bytes(), []byte("AI路由器")) && !bytes.Contains(wUI.Body.Bytes(), []byte("Nano")) {
		t.Errorf("expected Web UI to contain 'AI路由器' or 'Nano'")
	}
}

func TestAPI_AdminCRUDAndHotReload(t *testing.T) {
	dispatcher, adminHandler, repo := setupTestEnvironment(t)
	engine := api.SetupRouter(dispatcher, adminHandler)

	// 1. Create a channel via Admin REST API
	newCh := storage.ChannelRecord{
		Name:     "deepseek-admin-ch",
		Type:     model.ProviderOpenAI,
		BaseURL:  "https://api.deepseek.com/v1",
		APIKey:   "sk-test-secret",
		Models:   []string{"deepseek-chat", "deepseek-coder"},
		Priority: 1,
		Weight:   10,
	}
	payload, _ := json.Marshal(newCh)

	reqCreate := httptest.NewRequest(http.MethodPost, "/api/v1/admin/channels", bytes.NewReader(payload))
	reqCreate.Header.Set("Content-Type", "application/json")
	wCreate := httptest.NewRecorder()
	engine.ServeHTTP(wCreate, reqCreate)

	if wCreate.Code != http.StatusOK {
		t.Fatalf("expected 200 OK on create channel, got %d: %s", wCreate.Code, wCreate.Body.String())
	}

	// 2. Verify channel is in SQLite DB
	dbChannels, err := repo.ListChannels()
	if err != nil || len(dbChannels) != 1 {
		t.Fatalf("expected 1 channel in DB, got %d", len(dbChannels))
	}

	// 3. Verify channel is instantly hot-reloaded into Data Plane Memory
	supportedModels := dispatcher.GetAllSupportedModels()
	hasModel := false
	for _, m := range supportedModels {
		if m == "deepseek-chat" {
			hasModel = true
			break
		}
	}
	if !hasModel {
		t.Errorf("expected Data Plane memory to hot-reload 'deepseek-chat', found: %v", supportedModels)
	}

	// 4. Create a virtual key via Admin API
	keyPayload := []byte(`{"tenant_id": "test-team", "key": "sk-gw-admin-test", "rpm": 120}`)
	reqKey := httptest.NewRequest(http.MethodPost, "/api/v1/admin/keys", bytes.NewReader(keyPayload))
	reqKey.Header.Set("Content-Type", "application/json")
	wKey := httptest.NewRecorder()
	engine.ServeHTTP(wKey, reqKey)

	if wKey.Code != http.StatusOK {
		t.Fatalf("expected 200 OK on create key, got %d", wKey.Code)
	}

	// 5. Test Data Plane authorization with newly created key
	reqDataPlane := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	reqDataPlane.Header.Set("Authorization", "Bearer sk-gw-admin-test")
	wDataPlane := httptest.NewRecorder()
	engine.ServeHTTP(wDataPlane, reqDataPlane)

	if wDataPlane.Code != http.StatusOK {
		t.Errorf("expected 200 OK from Data Plane with hot-reloaded key, got %d: %s", wDataPlane.Code, wDataPlane.Body.String())
	}

	// 6. Test GET /api/v1/admin/logs
	_ = repo.RecordUsageLog(&storage.UsageLogRecord{
		VirtualKey: "sk-gw-admin-test",
		TenantID:   "dev-team",
		Model:      "deepseek-chat",
		Channel:    "upstream-primary",
		DurationMs: 15,
		StatusCode: 200,
	})
	reqLogs := httptest.NewRequest(http.MethodGet, "/api/v1/admin/logs?limit=10", nil)
	wLogs := httptest.NewRecorder()
	engine.ServeHTTP(wLogs, reqLogs)
	if wLogs.Code != http.StatusOK {
		t.Errorf("expected 200 OK for /api/v1/admin/logs, got %d: %s", wLogs.Code, wLogs.Body.String())
	}
	if !bytes.Contains(wLogs.Body.Bytes(), []byte("deepseek-chat")) {
		t.Errorf("expected logs response to contain logged model 'deepseek-chat'")
	}
}

func TestAPI_ChatCompletions_ModelForbidden(t *testing.T) {
	testCfg := &config.Config{
		VirtualKeys: []model.VirtualKeyConfig{
			{
				Key:           "sk-gw-restricted",
				TenantID:      "restricted-tenant",
				AllowedModels: []string{"deepseek-chat"},
			},
		},
		Channels: []model.ChannelConfig{
			{
				Name:     "ch1",
				Type:     model.ProviderOpenAI,
				Models:   []string{"deepseek-chat", "gpt-4o"},
				Priority: 1,
			},
		},
	}
	config.SetGlobalConfig(testCfg)

	dispatcher := router.NewDispatcher(testCfg.Channels)
	engine := api.SetupRouter(dispatcher, nil)

	body := model.ChatCompletionRequest{
		Model: "gpt-4o",
		Messages: []model.ChatMessage{
			{Role: "user", Content: "Hello"},
		},
	}
	bodyBytes, _ := json.Marshal(body)

	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(bodyBytes))
	req.Header.Set("Authorization", "Bearer sk-gw-restricted")
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Errorf("expected 403 Forbidden for disallowed model, got %d: %s", w.Code, w.Body.String())
	}
}

func TestAPI_TextCompletions(t *testing.T) {
	// Mock upstream OpenAI provider
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := model.ChatCompletionResponse{
			ID:      "cmpl-test-123",
			Object:  "chat.completion",
			Created: time.Now().Unix(),
			Model:   "deepseek-coder",
			Choices: []model.ChatCompletionChoice{
				{
					Index: 0,
					Message: model.ChatMessage{
						Role:    "assistant",
						Content: "    return x + y",
					},
				},
			},
			Usage: &model.Usage{
				PromptTokens:     5,
				CompletionTokens: 5,
				TotalTokens:      10,
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer mockServer.Close()

	channels := []model.ChannelConfig{
		{
			Name:     "upstream-coder",
			Type:     model.ProviderOpenAI,
			BaseURL:  mockServer.URL,
			APIKey:   "none",
			Models:   []string{"deepseek-coder"},
			Priority: 1,
		},
	}
	testCfg := &config.Config{
		Channels: channels,
	}
	config.SetGlobalConfig(testCfg)

	dispatcher := router.NewDispatcher(channels)
	engine := api.SetupRouter(dispatcher, nil)

	// Call OpenAI legacy text completion endpoint /v1/completions
	body := model.TextCompletionRequest{
		Model:  "deepseek-coder",
		Prompt: "def add(x, y):",
	}
	bodyBytes, _ := json.Marshal(body)

	req := httptest.NewRequest(http.MethodPost, "/v1/completions", bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for /v1/completions, got %d: %s", w.Code, w.Body.String())
	}

	var resp model.TextCompletionResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse /v1/completions response: %v", err)
	}

	if resp.Object != "text_completion" {
		t.Errorf("expected object text_completion, got %s", resp.Object)
	}
	if len(resp.Choices) == 0 || resp.Choices[0].Text != "    return x + y" {
		t.Errorf("unexpected text choice: %+v", resp.Choices)
	}
}

var _ = time.Now
