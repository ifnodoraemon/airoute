package tests

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/ifnodoraemon/airoute/internal/api"
	"github.com/ifnodoraemon/airoute/internal/controlplane"
	"github.com/ifnodoraemon/airoute/internal/router"
	"github.com/ifnodoraemon/airoute/internal/storage"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMCPServerAndSessionLogs(t *testing.T) {
	gin.SetMode(gin.TestMode)

	// Set up SQLite memory DB
	db, err := storage.OpenDB(":memory:")
	require.NoError(t, err)
	defer db.Close()

	repo := storage.NewRepository(db)
	dispatcher := router.NewDispatcher(nil)
	sync := controlplane.NewSynchronizer(repo, dispatcher)

	// Seed channels
	ch1 := &storage.ChannelRecord{
		Name:     "DeepSeek Primary",
		Type:     "openai",
		BaseURL:  "https://api.deepseek.com",
		APIKey:   "sk-test",
		Models:   []string{"deepseek-chat", "deepseek-reasoner"},
		Priority: 1,
		Weight:   10,
		Status:   "active",
	}
	require.NoError(t, repo.CreateChannel(ch1))
	require.NoError(t, sync.ReloadFromDB())

	adminHandler := controlplane.NewAdminHandler(repo, sync, dispatcher)
	r := api.SetupRouter(dispatcher, adminHandler)

	t.Run("GET /mcp discovery endpoint", func(t *testing.T) {
		req, _ := http.NewRequest(http.MethodGet, "/mcp", nil)
		resp := httptest.NewRecorder()
		r.ServeHTTP(resp, req)

		assert.Equal(t, http.StatusOK, resp.Code)
		var body map[string]interface{}
		err := json.Unmarshal(resp.Body.Bytes(), &body)
		require.NoError(t, err)
		server := body["server"].(map[string]interface{})
		assert.Equal(t, "AI路由器", server["name"])
		assert.Equal(t, "mcp/2026-07-28", server["protocol"])
		assert.Contains(t, body, "endpoints")
		assert.Contains(t, body, "configs")
	})

	t.Run("POST /mcp/messages initialize", func(t *testing.T) {
		payload := map[string]interface{}{
			"jsonrpc": "2.0",
			"id":      1,
			"method":  "initialize",
			"params": map[string]interface{}{
				"protocolVersion": "2026-07-28",
			},
		}
		data, _ := json.Marshal(payload)
		req, _ := http.NewRequest(http.MethodPost, "/mcp/messages", bytes.NewBuffer(data))
		req.Header.Set("Content-Type", "application/json")
		resp := httptest.NewRecorder()
		r.ServeHTTP(resp, req)

		assert.Equal(t, http.StatusOK, resp.Code)
		var res map[string]interface{}
		err = json.Unmarshal(resp.Body.Bytes(), &res)
		require.NoError(t, err)
		assert.Equal(t, float64(1), res["id"])
		result := res["result"].(map[string]interface{})
		assert.Equal(t, "2026-07-28", result["protocolVersion"])
		serverInfo := result["serverInfo"].(map[string]interface{})
		assert.Equal(t, "AI路由器", serverInfo["name"])
	})

	t.Run("POST /mcp/messages tools/list", func(t *testing.T) {
		payload := map[string]interface{}{
			"jsonrpc": "2.0",
			"id":      2,
			"method":  "tools/list",
		}
		data, _ := json.Marshal(payload)
		req, _ := http.NewRequest(http.MethodPost, "/mcp/messages", bytes.NewBuffer(data))
		req.Header.Set("Content-Type", "application/json")
		resp := httptest.NewRecorder()
		r.ServeHTTP(resp, req)

		assert.Equal(t, http.StatusOK, resp.Code)
		var res map[string]interface{}
		err = json.Unmarshal(resp.Body.Bytes(), &res)
		require.NoError(t, err)
		result := res["result"].(map[string]interface{})
		tools := result["tools"].([]interface{})
		assert.GreaterOrEqual(t, len(tools), 4)

		toolNames := []string{}
		for _, tItem := range tools {
			toolMap := tItem.(map[string]interface{})
			toolNames = append(toolNames, toolMap["name"].(string))
		}
		assert.Contains(t, toolNames, "airoute_model_topology")
		assert.Contains(t, toolNames, "airoute_chat")
		assert.Contains(t, toolNames, "airoute_query_logs")
		assert.Contains(t, toolNames, "airoute_cluster_status")
	})

	t.Run("POST /mcp/messages tools/call airoute_model_topology", func(t *testing.T) {
		payload := map[string]interface{}{
			"jsonrpc": "2.0",
			"id":      3,
			"method":  "tools/call",
			"params": map[string]interface{}{
				"name":      "airoute_model_topology",
				"arguments": map[string]interface{}{},
			},
		}
		data, _ := json.Marshal(payload)
		req, _ := http.NewRequest(http.MethodPost, "/mcp/messages", bytes.NewBuffer(data))
		req.Header.Set("Content-Type", "application/json")
		resp := httptest.NewRecorder()
		r.ServeHTTP(resp, req)

		assert.Equal(t, http.StatusOK, resp.Code)
		var res map[string]interface{}
		err = json.Unmarshal(resp.Body.Bytes(), &res)
		require.NoError(t, err)
		result := res["result"].(map[string]interface{})
		content := result["content"].([]interface{})
		assert.NotEmpty(t, content)
		text := content[0].(map[string]interface{})["text"].(string)
		assert.Contains(t, text, "deepseek-chat")
	})

	t.Run("Usage Logs Session ID and Time-range Filtering", func(t *testing.T) {
		now := time.Now()
		yesterday := now.Add(-24 * time.Hour)

		// Record logs
		log1 := &storage.UsageLogRecord{
			TenantID:         "tenant-a",
			Model:            "deepseek-chat",
			SessionID:        "sess-conv-alpha",
			PromptTokens:     100,
			CompletionTokens: 50,
			TotalTokens:      150,
			Cost:             0.002,
			CreatedAt:        now,
		}
		log2 := &storage.UsageLogRecord{
			TenantID:         "tenant-a",
			Model:            "gpt-4o",
			SessionID:        "sess-conv-beta",
			PromptTokens:     200,
			CompletionTokens: 100,
			TotalTokens:      300,
			Cost:             0.015,
			CreatedAt:        yesterday,
		}

		err := repo.RecordUsageLog(log1)
		require.NoError(t, err)
		err = repo.RecordUsageLog(log2)
		require.NoError(t, err)

		// 1. Filter by SessionID = sess-conv-alpha
		filtered, err := repo.ListUsageLogsWithFilter(storage.LogFilter{
			SessionID: "sess-conv-alpha",
		})
		require.NoError(t, err)
		assert.Len(t, filtered, 1)
		assert.Equal(t, "sess-conv-alpha", filtered[0].SessionID)
		assert.Equal(t, "deepseek-chat", filtered[0].Model)

		// 2. Filter by time range (from 2 hours ago to now)
		startTwoHoursAgo := now.UTC().Add(-2 * time.Hour).Format("2006-01-02 15:04:05")
		filteredTime, err := repo.ListUsageLogsWithFilter(storage.LogFilter{
			StartTime: startTwoHoursAgo,
		})
		require.NoError(t, err)
		assert.Len(t, filteredTime, 1)
		assert.Equal(t, "sess-conv-alpha", filteredTime[0].SessionID)

		// 3. Filter all
		all, err := repo.ListUsageLogsWithFilter(storage.LogFilter{Limit: 10})
		require.NoError(t, err)
		assert.Len(t, all, 2)
	})
}
