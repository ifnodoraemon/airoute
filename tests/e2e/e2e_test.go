//go:build e2e
// +build e2e

package e2e

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"sync"
	"testing"
	"time"
)

var (
	baseURL = func() string {
		if u := os.Getenv("AIROUTE_URL"); u != "" {
			return u
		}
		port := "8080"
		if p := os.Getenv("GATEWAY_INGRESS_PORT"); p != "" {
			port = p
		} else if p := os.Getenv("GATEWAY_PORT"); p != "" {
			port = p
		}
		return fmt.Sprintf("http://127.0.0.1:%s", port)
	}()
	adminUser = getEnv("AIROUTE_ADMIN_USER", getEnv("GATEWAY_ADMIN_USER", "admin"))
	adminPass = getEnv("AIROUTE_ADMIN_PASS", getEnv("GATEWAY_ADMIN_PASSWORD", "admin123"))
)

func getEnv(key, defaultVal string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return defaultVal
}

func getHTTPClient() *http.Client {
	return &http.Client{
		Timeout: 10 * time.Second,
	}
}

func loginAdmin(t *testing.T) string {
	t.Helper()
	client := getHTTPClient()

	payload, _ := json.Marshal(map[string]string{
		"username": adminUser,
		"password": adminPass,
	})

	resp, err := client.Post(baseURL+"/api/v1/auth/login", "application/json", bytes.NewReader(payload))
	if err != nil {
		t.Fatalf("failed to connect to %s/api/v1/auth/login: %v", baseURL, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("admin login failed with status %d: %s", resp.StatusCode, string(body))
	}

	var res struct {
		Code int `json:"code"`
		Data struct {
			Token string `json:"token"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		t.Fatalf("failed to decode login response: %v", err)
	}

	if res.Data.Token == "" {
		t.Fatalf("received empty admin token")
	}
	return res.Data.Token
}

// TestE2E_ClusterHealth verifies the load balancer /health, public status, and web UI.
func TestE2E_ClusterHealth(t *testing.T) {
	client := getHTTPClient()

	t.Run("HealthcheckEndpoint", func(t *testing.T) {
		resp, err := client.Get(baseURL + "/health")
		if err != nil {
			t.Fatalf("GET /health failed: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			t.Fatalf("expected status 200, got %d", resp.StatusCode)
		}

		var body map[string]interface{}
		if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
			t.Fatalf("failed to decode /health json: %v", err)
		}

		if body["status"] != "healthy" {
			t.Fatalf("expected status 'healthy', got %v", body["status"])
		}
	})

	t.Run("PublicStatusEndpoint", func(t *testing.T) {
		resp, err := client.Get(baseURL + "/api/v1/public/status")
		if err != nil {
			t.Fatalf("GET /api/v1/public/status failed: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			t.Fatalf("expected status 200, got %d", resp.StatusCode)
		}
	})

	t.Run("WebUI_SPARoot", func(t *testing.T) {
		resp, err := client.Get(baseURL + "/")
		if err != nil {
			t.Fatalf("GET / failed: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			t.Fatalf("expected status 200, got %d", resp.StatusCode)
		}

		bodyBytes, _ := io.ReadAll(resp.Body)
		if !strings.Contains(strings.ToLower(string(bodyBytes)), "<!doctype html>") {
			t.Fatalf("expected SPA html document, got %s", string(bodyBytes[:100]))
		}
	})
}

// TestE2E_SecurityAndAuth verifies RBAC, unauthorized rejection, and session inspection.
func TestE2E_SecurityAndAuth(t *testing.T) {
	client := getHTTPClient()

	t.Run("UnauthenticatedRejection", func(t *testing.T) {
		resp, err := client.Get(baseURL + "/api/v1/admin/keys")
		if err != nil {
			t.Fatalf("GET /api/v1/admin/keys failed: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusUnauthorized {
			t.Fatalf("expected 401 Unauthorized, got %d", resp.StatusCode)
		}
	})

	t.Run("InvalidCredentialsRejection", func(t *testing.T) {
		payload, _ := json.Marshal(map[string]string{
			"username": "admin",
			"password": "wrong-password-999",
		})
		resp, err := client.Post(baseURL+"/api/v1/auth/login", "application/json", bytes.NewReader(payload))
		if err != nil {
			t.Fatalf("POST /api/v1/auth/login failed: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusUnauthorized {
			t.Fatalf("expected 401 Unauthorized, got %d", resp.StatusCode)
		}
	})

	t.Run("SessionIdentityMe", func(t *testing.T) {
		token := loginAdmin(t)
		req, _ := http.NewRequest(http.MethodGet, baseURL+"/api/v1/auth/me", nil)
		req.Header.Set("Authorization", "Bearer "+token)

		resp, err := client.Do(req)
		if err != nil {
			t.Fatalf("GET /api/v1/auth/me failed: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			t.Fatalf("expected 200 OK, got %d", resp.StatusCode)
		}

		var res struct {
			Data struct {
				Username string `json:"username"`
				Role     string `json:"role"`
			} `json:"data"`
		}
		_ = json.NewDecoder(resp.Body).Decode(&res)
		if res.Data.Username != "admin" || res.Data.Role != "admin" {
			t.Fatalf("expected admin user, got %+v", res.Data)
		}
	})
}

// TestE2E_MCPEcosystem verifies the 15 built-in MCP servers and toggle functionality.
func TestE2E_MCPEcosystem(t *testing.T) {
	client := getHTTPClient()
	token := loginAdmin(t)

	expectedMCPs := []string{
		"airoute-gateway",
		"memory-graph",
		"git-mcp",
		"filesystem-mcp",
		"fetch-mcp",
		"sqlite-mcp",
		"sentry-mcp",
		"github-mcp",
		"postgres-mcp",
		"browser-fetch-mcp",
		"sequential-thinking",
		"brave-search",
		"modelscope-search",
		"feishu-lark-mcp",
		"docker-k8s-mcp",
	}

	req, _ := http.NewRequest(http.MethodGet, baseURL+"/api/v1/admin/mcp/servers", nil)
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("GET /api/v1/admin/mcp/servers failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", resp.StatusCode)
	}

	var res struct {
		Code int `json:"code"`
		Data []struct {
			ID       string `json:"id"`
			Name     string `json:"name"`
			Category string `json:"category"`
			Enabled  bool   `json:"enabled"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		t.Fatalf("failed to decode mcp servers: %v", err)
	}

	foundMap := make(map[string]bool)
	for _, s := range res.Data {
		foundMap[s.ID] = true
	}

	for _, expectedID := range expectedMCPs {
		if !foundMap[expectedID] {
			t.Errorf("missing expected built-in MCP server: %s", expectedID)
		}
	}

	// Test Toggle MCP server
	togglePayload, _ := json.Marshal(map[string]bool{"enabled": true})
	toggleReq, _ := http.NewRequest(http.MethodPost, baseURL+"/api/v1/admin/mcp/servers/memory-graph/toggle", bytes.NewReader(togglePayload))
	toggleReq.Header.Set("Authorization", "Bearer "+token)
	toggleReq.Header.Set("Content-Type", "application/json")

	toggleResp, err := client.Do(toggleReq)
	if err != nil {
		t.Fatalf("POST /api/v1/admin/mcp/servers/memory-graph/toggle failed: %v", err)
	}
	defer toggleResp.Body.Close()

	if toggleResp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 OK for toggle, got %d", toggleResp.StatusCode)
	}
}

// TestE2E_SkillsAndStorage verifies skill download via gateway streaming and edge caching.
func TestE2E_SkillsAndStorage(t *testing.T) {
	client := getHTTPClient()
	token := loginAdmin(t)

	t.Run("ListSkills", func(t *testing.T) {
		req, _ := http.NewRequest(http.MethodGet, baseURL+"/api/v1/admin/skills", nil)
		req.Header.Set("Authorization", "Bearer "+token)

		resp, err := client.Do(req)
		if err != nil {
			t.Fatalf("GET /api/v1/admin/skills failed: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			t.Fatalf("expected 200 OK, got %d", resp.StatusCode)
		}
	})

	t.Run("SkillZIPDownloadAndCache", func(t *testing.T) {
		url := baseURL + "/api/v1/skills/test-driven-development/download"

		// First request (MISS or Stream)
		resp1, err := client.Get(url)
		if err != nil {
			t.Fatalf("GET %s failed: %v", url, err)
		}
		defer resp1.Body.Close()

		if resp1.StatusCode != http.StatusOK {
			t.Fatalf("expected 200 OK, got %d", resp1.StatusCode)
		}

		if ct := resp1.Header.Get("Content-Type"); !strings.Contains(ct, "application/zip") {
			t.Fatalf("expected Content-Type application/zip, got %s", ct)
		}

		bodyBytes, err := io.ReadAll(resp1.Body)
		if err != nil {
			t.Fatalf("failed to read zip payload: %v", err)
		}

		if len(bodyBytes) < 100 || string(bodyBytes[:2]) != "PK" {
			t.Fatalf("expected valid zip file with PK magic header, length: %d", len(bodyBytes))
		}

		// Second request (Edge Cache hit)
		resp2, err := client.Get(url)
		if err != nil {
			t.Fatalf("second GET %s failed: %v", url, err)
		}
		defer resp2.Body.Close()

		if resp2.StatusCode != http.StatusOK {
			t.Fatalf("expected 200 OK for cached request, got %d", resp2.StatusCode)
		}

		cacheStatus := resp2.Header.Get("X-Cache-Status")
		t.Logf("Nginx edge cache status on second request: %s", cacheStatus)
	})
}

// TestE2E_TopologyChannelsPricing verifies routing matrix, channels, and pricing engine.
func TestE2E_TopologyChannelsPricing(t *testing.T) {
	client := getHTTPClient()
	token := loginAdmin(t)

	endpoints := []string{
		"/api/v1/admin/channels",
		"/api/v1/admin/models/routes",
		"/api/v1/admin/pricing",
		"/api/v1/admin/stats/overview",
	}

	for _, ep := range endpoints {
		t.Run(ep, func(t *testing.T) {
			req, _ := http.NewRequest(http.MethodGet, baseURL+ep, nil)
			req.Header.Set("Authorization", "Bearer "+token)

			resp, err := client.Do(req)
			if err != nil {
				t.Fatalf("GET %s failed: %v", ep, err)
			}
			defer resp.Body.Close()

			if resp.StatusCode != http.StatusOK {
				t.Fatalf("expected 200 OK for %s, got %d", ep, resp.StatusCode)
			}
		})
	}
}

// TestE2E_KeyGovernanceAndCompletion verifies virtual key lifecycle and OpenAI chat completions.
func TestE2E_KeyGovernanceAndCompletion(t *testing.T) {
	client := getHTTPClient()
	token := loginAdmin(t)

	testKeyVal := fmt.Sprintf("sk-goe2e-%d", time.Now().Unix())

	// 1. Create temporary virtual key
	createPayload, _ := json.Marshal(map[string]interface{}{
		"key":       testKeyVal,
		"tenant_id": "go-e2e-tenant",
		"status":    "active",
		"rpm":       60,
		"tpm":       100000,
		"budget":    100.0,
	})
	createReq, _ := http.NewRequest(http.MethodPost, baseURL+"/api/v1/admin/keys", bytes.NewReader(createPayload))
	createReq.Header.Set("Authorization", "Bearer "+token)
	createReq.Header.Set("Content-Type", "application/json")

	createResp, err := client.Do(createReq)
	if err != nil {
		t.Fatalf("POST /api/v1/admin/keys failed: %v", err)
	}
	defer createResp.Body.Close()

	if createResp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 OK for create key, got %d", createResp.StatusCode)
	}

	var createData struct {
		Data struct {
			ID int64 `json:"id"`
		} `json:"data"`
	}
	_ = json.NewDecoder(createResp.Body).Decode(&createData)
	keyID := createData.Data.ID

	// Allow Redis pub/sub hot-reload to propagate across all cluster nodes
	time.Sleep(200 * time.Millisecond)

	defer func() {
		if keyID > 0 {
			delReq, _ := http.NewRequest(http.MethodDelete, fmt.Sprintf("%s/api/v1/admin/keys/%d", baseURL, keyID), nil)
			delReq.Header.Set("Authorization", "Bearer "+token)
			_, _ = client.Do(delReq)
		}
	}()

	// 2. Query /v1/models with virtual key
	modelsReq, _ := http.NewRequest(http.MethodGet, baseURL+"/v1/models", nil)
	modelsReq.Header.Set("Authorization", "Bearer "+testKeyVal)

	modelsResp, err := client.Do(modelsReq)
	if err != nil {
		t.Fatalf("GET /v1/models failed: %v", err)
	}
	defer modelsResp.Body.Close()

	if modelsResp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 OK for /v1/models, got %d", modelsResp.StatusCode)
	}

	// 3. Test /v1/chat/completions trace header injection
	chatPayload, _ := json.Marshal(map[string]interface{}{
		"model": "gpt-4o",
		"messages": []map[string]string{
			{"role": "user", "content": "hello e2e"},
		},
	})
	chatReq, _ := http.NewRequest(http.MethodPost, baseURL+"/v1/chat/completions", bytes.NewReader(chatPayload))
	chatReq.Header.Set("Authorization", "Bearer "+testKeyVal)
	chatReq.Header.Set("Content-Type", "application/json")

	chatResp, err := client.Do(chatReq)
	if err != nil {
		t.Fatalf("POST /v1/chat/completions failed: %v", err)
	}
	defer chatResp.Body.Close()

	traceID := chatResp.Header.Get("X-Airoute-Trace-Id")
	if traceID == "" {
		t.Fatalf("missing distributed X-Airoute-Trace-Id in chat completions response")
	}
	t.Logf("Successfully captured X-Airoute-Trace-Id: %s", traceID)
}

// TestE2E_HighAvailabilityAndConcurrency verifies LB stability under concurrent load.
func TestE2E_HighAvailabilityAndConcurrency(t *testing.T) {
	client := getHTTPClient()
	concurrency := 25
	var wg sync.WaitGroup
	errCh := make(chan error, concurrency)

	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			resp, err := client.Get(baseURL + "/health")
			if err != nil {
				errCh <- err
				return
			}
			defer resp.Body.Close()
			if resp.StatusCode != http.StatusOK {
				errCh <- fmt.Errorf("unexpected status %d", resp.StatusCode)
			}
		}()
	}

	wg.Wait()
	close(errCh)

	for err := range errCh {
		t.Fatalf("concurrency probe failed: %v", err)
	}
}
