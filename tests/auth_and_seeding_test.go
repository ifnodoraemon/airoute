package tests

import (
	"database/sql"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/ifnodoraemon/nano-gateway/internal/config"
	"github.com/ifnodoraemon/nano-gateway/internal/middleware"
	"github.com/ifnodoraemon/nano-gateway/internal/model"
	"github.com/ifnodoraemon/nano-gateway/internal/storage"
	_ "modernc.org/sqlite"
)

func TestAuthMiddleware_VirtualKeyContextAndLogging(t *testing.T) {
	gin.SetMode(gin.TestMode)

	oldCfg := config.GetGlobalConfig()
	defer config.SetGlobalConfig(oldCfg)

	// Setup config with a virtual key
	testKey := "sk-gw-audit-test-key"
	testTenant := "tenant-alpha"
	cfg := &config.Config{
		VirtualKeys: []model.VirtualKeyConfig{
			{
				Key:           testKey,
				TenantID:      testTenant,
				AllowedModels: []string{"gpt-4o", "claude/*"},
			},
		},
	}
	config.SetGlobalConfig(cfg)

	r := gin.New()
	r.Use(middleware.AuthMiddleware())

	var capturedKey string
	var capturedTenant string
	var modelAllowedGPT4o bool
	var modelAllowedClaudeSonnet bool
	var modelAllowedDisallowed bool

	r.GET("/test-auth", func(c *gin.Context) {
		capturedKey = c.GetString(middleware.ContextKeyVirtualKey)
		capturedTenant = c.GetString(middleware.ContextKeyTenant)
		modelAllowedGPT4o = middleware.ValidateModelAllowed(c, "gpt-4o")
		modelAllowedClaudeSonnet = middleware.ValidateModelAllowed(c, "claude/3-5-sonnet")
		modelAllowedDisallowed = middleware.ValidateModelAllowed(c, "disallowed-model")
		c.Status(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/test-auth", nil)
	req.Header.Set("Authorization", "Bearer "+testKey)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", w.Code)
	}

	// Verify c.GetString("virtual_key") is NOT empty string
	if capturedKey != testKey {
		t.Fatalf("virtual key was lost in context! expected '%s', got '%s'", testKey, capturedKey)
	}
	if capturedTenant != testTenant {
		t.Fatalf("tenant id was lost in context! expected '%s', got '%s'", testTenant, capturedTenant)
	}

	if !modelAllowedGPT4o {
		t.Errorf("expected gpt-4o to be allowed")
	}
	if !modelAllowedClaudeSonnet {
		t.Errorf("expected claude/3-5-sonnet to be allowed via wildcard 'claude/*'")
	}
	if modelAllowedDisallowed {
		t.Errorf("expected disallowed-model to be blocked")
	}
}

func TestStorageSeeding_ProtocolsPersistence(t *testing.T) {
	tempDB := "test_seeding_" + t.Name() + ".db"
	defer os.Remove(tempDB)

	db, err := storage.OpenDB(tempDB)
	if err != nil {
		t.Fatalf("failed to open test db: %v", err)
	}
	defer db.Close()

	repo := storage.NewRepository(db)

	protocols := []string{"chat", "images", "audio_speech", "videos"}
	ch := &storage.ChannelRecord{
		Name:           "seeding-test-channel",
		Type:           model.ProviderOpenAI,
		BaseURL:        "https://api.openai.com",
		APIKey:         "sk-test",
		Models:         []string{"gpt-4o", "dall-e-3"},
		Protocols:      protocols,
		Priority:       1,
		Weight:         10,
		TimeoutSeconds: 60,
	}

	if err := repo.CreateChannel(ch); err != nil {
		t.Fatalf("CreateChannel failed: %v", err)
	}

	list, err := repo.ListChannels()
	if err != nil {
		t.Fatalf("ListChannels failed: %v", err)
	}

	if len(list) == 0 {
		t.Fatalf("no channels retrieved from db")
	}

	retrieved := list[0]
	if len(retrieved.Protocols) != len(protocols) {
		t.Fatalf("Protocols count mismatch! Expected %d, got %d (%v)", len(protocols), len(retrieved.Protocols), retrieved.Protocols)
	}
	for i, p := range protocols {
		if retrieved.Protocols[i] != p {
			t.Errorf("Protocol index %d mismatch: expected %s, got %s", i, p, retrieved.Protocols[i])
		}
	}
}

func TestRateLimitMiddleware_Enforcement(t *testing.T) {
	gin.SetMode(gin.TestMode)

	oldCfg := config.GetGlobalConfig()
	defer config.SetGlobalConfig(oldCfg)

	testKey := "sk-gw-ratelimit-test"
	cfg := &config.Config{
		VirtualKeys: []model.VirtualKeyConfig{
			{
				Key:      testKey,
				TenantID: "tenant-limit",
				RPM:      2, // Limit to 2 requests per minute
			},
		},
	}
	config.SetGlobalConfig(cfg)

	r := gin.New()
	r.Use(middleware.AuthMiddleware())
	r.Use(middleware.RateLimitMiddleware())
	r.GET("/api/test", func(c *gin.Context) {
		c.Status(http.StatusOK)
	})

	// First 2 requests should succeed
	for i := 0; i < 2; i++ {
		req := httptest.NewRequest(http.MethodGet, "/api/test", nil)
		req.Header.Set("Authorization", "Bearer "+testKey)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("request %d expected 200 OK, got %d", i+1, w.Code)
		}
	}

	// 3rd request should be blocked with 429 Too Many Requests
	req := httptest.NewRequest(http.MethodGet, "/api/test", nil)
	req.Header.Set("Authorization", "Bearer "+testKey)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusTooManyRequests {
		t.Fatalf("expected 429 Too Many Requests after exceeding RPM, got %d: %s", w.Code, w.Body.String())
	}
}

func TestConfig_EnvironmentVariableExpansion(t *testing.T) {
	os.Setenv("TEST_GATEWAY_SECRET", "super-secret-token-12345")
	defer os.Unsetenv("TEST_GATEWAY_SECRET")

	tempYAML := "test_config_expand_" + t.Name() + ".yaml"
	yamlContent := `
server:
  host: "127.0.0.1"
  port: 8080
channels:
  - name: "test-channel"
    type: "openai"
    base_url: "https://api.test.com"
    api_key: "${TEST_GATEWAY_SECRET}"
`
	if err := os.WriteFile(tempYAML, []byte(yamlContent), 0644); err != nil {
		t.Fatalf("failed to write test yaml: %v", err)
	}
	defer os.Remove(tempYAML)

	cfg, err := config.LoadConfig(tempYAML)
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}

	if len(cfg.Channels) == 0 {
		t.Fatalf("expected channels in config")
	}
	if cfg.Channels[0].APIKey != "super-secret-token-12345" {
		t.Errorf("expected expanded API key 'super-secret-token-12345', got '%s'", cfg.Channels[0].APIKey)
	}
}

func init() {
	// Register sqlite3 driver if needed
	_ = sql.Drivers()
}
