package tests

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ifnodoraemon/airoute/internal/api"
	"github.com/ifnodoraemon/airoute/internal/config"
	"github.com/ifnodoraemon/airoute/internal/model"
	"github.com/ifnodoraemon/airoute/internal/provider"
	"github.com/ifnodoraemon/airoute/internal/router"
)

func TestAPI_FormatValidation_DefaultOff(t *testing.T) {
	// By default, DefaultLevel is "off". Unconfigured models pass through validation without blocking.
	testCfg := &config.Config{
		APIKeys: []model.APIKeyConfig{
			{
				Key:      "sk-val-default-test",
				TenantID: "val-team",
			},
		},
		Channels: []model.ChannelConfig{
			{
				Name:     "ch-openai",
				Type:     model.ProviderOpenAI,
				Models:   []string{"gpt-3.5-turbo"},
				Priority: 1,
			},
		},
		ModelValidation: config.ModelValidationConfig{
			DefaultLevel: "off",
			Rules:        []config.ModelValidationRule{},
		},
	}
	config.SetGlobalConfig(testCfg)

	dispatcher := router.NewDispatcher(testCfg.Channels)
	engine := api.SetupRouter(dispatcher, nil)

	// Even with temperature 5.0 (out of range), default "off" passes format validation!
	// (It then attempts to dispatch and fails upstream since fake channel has no mock server,
	// returning 502 Bad Gateway instead of 400 Bad Request ValidationError).
	tempOOB := 5.0
	body := model.ChatCompletionRequest{
		Model: "gpt-3.5-turbo",
		Messages: []model.ChatMessage{
			{Role: "user", Content: "Hello"},
		},
		Temperature: &tempOOB,
	}
	bodyBytes, _ := json.Marshal(body)

	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(bodyBytes))
	req.Header.Set("Authorization", "Bearer sk-val-default-test")
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	// Must NOT be 400 Bad Request (which would mean validator intercepted it)
	if w.Code == http.StatusBadRequest {
		t.Fatalf("expected request NOT to be blocked by validator when DefaultLevel is off, got 400: %s", w.Body.String())
	}
}

func TestAPI_FormatValidation_PerModel_OpenAI(t *testing.T) {
	testCfg := &config.Config{
		APIKeys: []model.APIKeyConfig{
			{
				Key:      "sk-val-openai-test",
				TenantID: "val-team",
			},
		},
		Channels: []model.ChannelConfig{
			{
				Name:     "ch-openai",
				Type:     model.ProviderOpenAI,
				Models:   []string{"gpt-4o"},
				Priority: 1,
			},
		},
		ModelValidation: config.ModelValidationConfig{
			DefaultLevel: "off",
			Rules: []config.ModelValidationRule{
				{Model: "gpt-4o", Protocol: "openai", Level: "strict"},
			},
		},
	}
	config.SetGlobalConfig(testCfg)

	dispatcher := router.NewDispatcher(testCfg.Channels)
	engine := api.SetupRouter(dispatcher, nil)

	// Strict mode configured specifically on gpt-4o with temperature 2.8 (> 2.0) -> Expect HTTP 400
	tempOOB := 2.8
	bodyStrict := model.ChatCompletionRequest{
		Model: "gpt-4o",
		Messages: []model.ChatMessage{
			{Role: "user", Content: "Hello"},
		},
		Temperature: &tempOOB,
	}
	bodyBytes, _ := json.Marshal(bodyStrict)

	reqStrict := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(bodyBytes))
	reqStrict.Header.Set("Authorization", "Bearer sk-val-openai-test")
	reqStrict.Header.Set("Content-Type", "application/json")
	wStrict := httptest.NewRecorder()
	engine.ServeHTTP(wStrict, reqStrict)

	if wStrict.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 Bad Request for strict temperature validation on gpt-4o, got %d: %s", wStrict.Code, wStrict.Body.String())
	}

	var errResp struct {
		Error struct {
			Message string `json:"message"`
			Type    string `json:"type"`
			Param   string `json:"param"`
			Code    string `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal(wStrict.Body.Bytes(), &errResp); err != nil {
		t.Fatalf("failed to parse OpenAI error body: %v", err)
	}
	if errResp.Error.Type != "invalid_request_error" || errResp.Error.Param != "temperature" {
		t.Fatalf("expected invalid_request_error on param temperature, got %+v", errResp.Error)
	}
}

func TestAPI_FormatValidation_PerModel_Anthropic(t *testing.T) {
	testCfg := &config.Config{
		APIKeys: []model.APIKeyConfig{
			{
				Key:      "sk-val-claude-test",
				TenantID: "val-team",
			},
		},
		Channels: []model.ChannelConfig{
			{
				Name:     "ch-claude",
				Type:     model.ProviderAnthropic,
				Models:   []string{"claude-3-5-sonnet-20241022"},
				Priority: 1,
			},
		},
		ModelValidation: config.ModelValidationConfig{
			DefaultLevel: "off",
			Rules: []config.ModelValidationRule{
				{Model: "claude-*", Protocol: "anthropic", Level: "strict"},
			},
		},
	}
	config.SetGlobalConfig(testCfg)

	dispatcher := router.NewDispatcher(testCfg.Channels)
	engine := api.SetupRouter(dispatcher, nil)

	// Strict mode: missing max_tokens (max_tokens = 0) -> Expect HTTP 400 with Anthropic error structure
	bodyAnthropic := model.AnthropicInboundRequest{
		Model: "claude-3-5-sonnet-20241022",
		Messages: []model.AnthropicInboundMessage{
			{Role: "user", Content: "Hello Claude"},
		},
		MaxTokens: 0,
	}
	bodyBytes, _ := json.Marshal(bodyAnthropic)

	req := httptest.NewRequest(http.MethodPost, "/v1/messages", bytes.NewReader(bodyBytes))
	req.Header.Set("Authorization", "Bearer sk-val-claude-test")
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 Bad Request for Anthropic missing max_tokens, got %d: %s", w.Code, w.Body.String())
	}

	var errResp struct {
		Type  string `json:"type"`
		Error struct {
			Type    string `json:"type"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &errResp); err != nil {
		t.Fatalf("failed to parse Anthropic error body: %v", err)
	}
	if errResp.Type != "error" || errResp.Error.Type != "invalid_request_error" {
		t.Fatalf("expected Anthropic standard error structure, got %+v", errResp)
	}
}

func TestAPI_FormatValidation_PerModel_Gemini(t *testing.T) {
	testCfg := &config.Config{
		APIKeys: []model.APIKeyConfig{
			{
				Key:      "sk-val-gemini-test",
				TenantID: "val-team",
			},
		},
		Channels: []model.ChannelConfig{
			{
				Name:     "ch-gemini",
				Type:     model.ProviderGemini,
				Models:   []string{"gemini-1.5-pro"},
				Priority: 1,
			},
		},
		ModelValidation: config.ModelValidationConfig{
			DefaultLevel: "off",
			Rules: []config.ModelValidationRule{
				{Model: "gemini-*", Protocol: "gemini", Level: "strict"},
			},
		},
	}
	config.SetGlobalConfig(testCfg)

	dispatcher := router.NewDispatcher(testCfg.Channels)
	engine := api.SetupRouter(dispatcher, nil)

	// Strict mode: role 'assistant' is invalid for Gemini (must be 'model') -> Expect HTTP 400 with Gemini error structure
	bodyGemini := provider.GeminiRequest{
		Contents: []provider.GeminiContent{
			{
				Role:  "assistant",
				Parts: []provider.GeminiPart{{Text: "I am assistant"}},
			},
		},
	}
	bodyBytes, _ := json.Marshal(bodyGemini)

	req := httptest.NewRequest(http.MethodPost, "/v1beta/models/gemini-1.5-pro:generateContent", bytes.NewReader(bodyBytes))
	req.Header.Set("Authorization", "Bearer sk-val-gemini-test")
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 Bad Request for Gemini invalid role, got %d: %s", w.Code, w.Body.String())
	}

	var errResp struct {
		Error struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
			Status  string `json:"status"`
		} `json:"error"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &errResp); err != nil {
		t.Fatalf("failed to parse Gemini error body: %v", err)
	}
	if errResp.Error.Code != 400 || errResp.Error.Status != "INVALID_ARGUMENT" {
		t.Fatalf("expected Gemini standard error structure, got %+v", errResp)
	}
}

func TestAPI_FormatValidation_PerKey_Enable(t *testing.T) {
	// Model has NO rules, DefaultLevel is "off". But API Key has FormatValidation: "strict".
	testCfg := &config.Config{
		APIKeys: []model.APIKeyConfig{
			{
				Key:              "sk-strict-key",
				TenantID:         "strict-user",
				FormatValidation: "strict",
			},
		},
		Channels: []model.ChannelConfig{
			{
				Name:     "ch-openai",
				Type:     model.ProviderOpenAI,
				Models:   []string{"custom-model-x"},
				Priority: 1,
			},
		},
		ModelValidation: config.ModelValidationConfig{
			DefaultLevel: "off",
			Rules:        []config.ModelValidationRule{},
		},
	}
	config.SetGlobalConfig(testCfg)

	dispatcher := router.NewDispatcher(testCfg.Channels)
	engine := api.SetupRouter(dispatcher, nil)

	// Since key specifies FormatValidation: "strict", temperature 3.0 (> 2.0) must be rejected with 400!
	tempOOB := 3.0
	body := model.ChatCompletionRequest{
		Model: "custom-model-x",
		Messages: []model.ChatMessage{
			{Role: "user", Content: "Hello"},
		},
		Temperature: &tempOOB,
	}
	bodyBytes, _ := json.Marshal(body)

	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(bodyBytes))
	req.Header.Set("Authorization", "Bearer sk-strict-key")
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 Bad Request because key has format_validation=strict, got %d: %s", w.Code, w.Body.String())
	}
}

func TestAPI_FormatValidation_PerKey_OverrideOff(t *testing.T) {
	// Model has strict rule, but API Key explicitly sets FormatValidation: "off".
	testCfg := &config.Config{
		APIKeys: []model.APIKeyConfig{
			{
				Key:              "sk-bypass-key",
				TenantID:         "trusted-client",
				FormatValidation: "off",
			},
		},
		Channels: []model.ChannelConfig{
			{
				Name:     "ch-openai",
				Type:     model.ProviderOpenAI,
				Models:   []string{"gpt-4o"},
				Priority: 1,
			},
		},
		ModelValidation: config.ModelValidationConfig{
			DefaultLevel: "off",
			Rules: []config.ModelValidationRule{
				{Model: "gpt-4o", Protocol: "openai", Level: "strict"},
			},
		},
	}
	config.SetGlobalConfig(testCfg)

	dispatcher := router.NewDispatcher(testCfg.Channels)
	engine := api.SetupRouter(dispatcher, nil)

	tempOOB := 3.0
	body := model.ChatCompletionRequest{
		Model: "gpt-4o",
		Messages: []model.ChatMessage{
			{Role: "user", Content: "Hello"},
		},
		Temperature: &tempOOB,
	}
	bodyBytes, _ := json.Marshal(body)

	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(bodyBytes))
	req.Header.Set("Authorization", "Bearer sk-bypass-key")
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	// Since key bypassed validation with format_validation: "off", it should NOT return 400!
	if w.Code == http.StatusBadRequest {
		t.Fatalf("expected key with format_validation=off to bypass strict model rule, but got 400: %s", w.Body.String())
	}
}
