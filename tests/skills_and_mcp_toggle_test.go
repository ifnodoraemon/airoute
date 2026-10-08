package tests

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ifnodoraemon/airoute/internal/api"
	"github.com/ifnodoraemon/airoute/internal/controlplane"
	"github.com/ifnodoraemon/airoute/internal/router"
	"github.com/ifnodoraemon/airoute/internal/storage"
	"github.com/stretchr/testify/assert"
)

func TestSkills_OnDemandTogglingAndMCP(t *testing.T) {
	db, err := storage.OpenDB(":memory:")
	assert.NoError(t, err)
	defer db.Close()

	repo := storage.NewRepository(db)
	dispatcher := router.NewDispatcher(nil)
	sync := controlplane.NewSynchronizer(repo, dispatcher)
	adminHandler := controlplane.NewAdminHandler(repo, sync, dispatcher)
	engine := api.SetupRouter(dispatcher, adminHandler)

	// 1. Initial open standard agent skills listing (4 high-value curated skills)
	skills, err := repo.ListSkills()
	assert.NoError(t, err)
	assert.Equal(t, 4, len(skills))

	// 1.1 ModelScope-style MCP servers listing
	mcpServers, err := repo.ListMCPServers()
	assert.NoError(t, err)
	assert.GreaterOrEqual(t, len(mcpServers), 6)

	// 1.2 Test MCP 2026-07-28 server/discover (Stateless Discovery RPC)
	discRPCReq := map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      99,
		"method":  "server/discover",
	}
	bDiscRPC, _ := json.Marshal(discRPCReq)
	reqDiscRPC := httptest.NewRequest(http.MethodPost, "/mcp/messages", bytes.NewReader(bDiscRPC))
	reqDiscRPC.Header.Set("Content-Type", "application/json")
	wDiscRPC := httptest.NewRecorder()
	engine.ServeHTTP(wDiscRPC, reqDiscRPC)
	assert.Equal(t, http.StatusOK, wDiscRPC.Code)
	assert.Equal(t, "2026-07-28", wDiscRPC.Header().Get("MCP-Protocol-Version"))
	assert.Contains(t, wDiscRPC.Body.String(), "2026-07-28")
	assert.Contains(t, wDiscRPC.Body.String(), "AI路由器")

	// 2. Query MCP tools/list - active tools should be present
	mcpReq := map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "tools/list",
	}
	body, _ := json.Marshal(mcpReq)
	req := httptest.NewRequest(http.MethodPost, "/mcp/messages", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)

	var mcpResp struct {
		Result struct {
			Tools []struct {
				Name string `json:"name"`
			} `json:"tools"`
		} `json:"result"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &mcpResp)
	assert.GreaterOrEqual(t, len(mcpResp.Result.Tools), 10)

	// 2.1 Test Progressive Stage 1: airoute_search_skills
	searchCallReq := map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      101,
		"method":  "tools/call",
		"params": map[string]interface{}{
			"name": "airoute_search_skills",
			"arguments": map[string]interface{}{
				"query": "git",
			},
		},
	}
	bSearch, _ := json.Marshal(searchCallReq)
	reqSearch := httptest.NewRequest(http.MethodPost, "/mcp/messages", bytes.NewReader(bSearch))
	wSearch := httptest.NewRecorder()
	engine.ServeHTTP(wSearch, reqSearch)
	assert.Equal(t, http.StatusOK, wSearch.Code)
	assert.Contains(t, wSearch.Body.String(), "git-workflow")

	// 2.2 Test Progressive Stage 2: airoute_inspect_skill
	inspCallReq := map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      102,
		"method":  "tools/call",
		"params": map[string]interface{}{
			"name": "airoute_inspect_skill",
			"arguments": map[string]interface{}{
				"skill_id": "git-workflow",
			},
		},
	}
	bInsp, _ := json.Marshal(inspCallReq)
	reqInsp := httptest.NewRequest(http.MethodPost, "/mcp/messages", bytes.NewReader(bInsp))
	wInsp := httptest.NewRecorder()
	engine.ServeHTTP(wInsp, reqInsp)
	assert.Equal(t, http.StatusOK, wInsp.Code)
	assert.Contains(t, wInsp.Body.String(), "run_command")
	assert.Contains(t, wInsp.Body.String(), "Stage 2 Confirmed")

	// 2.3 Test Progressive Stage 3: airoute_get_skill_manifest
	manCallReq := map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      103,
		"method":  "tools/call",
		"params": map[string]interface{}{
			"name": "airoute_get_skill_manifest",
			"arguments": map[string]interface{}{
				"skill_id": "git-workflow",
			},
		},
	}
	bMan, _ := json.Marshal(manCallReq)
	reqMan := httptest.NewRequest(http.MethodPost, "/mcp/messages", bytes.NewReader(bMan))
	wMan := httptest.NewRecorder()
	engine.ServeHTTP(wMan, reqMan)
	assert.Equal(t, http.StatusOK, wMan.Code)
	assert.Contains(t, wMan.Body.String(), "git-workflow")
	assert.Contains(t, wMan.Body.String(), "SOP 执行工作流")

	// 2.1 Test Skill ZIP Download
	reqZip := httptest.NewRequest(http.MethodGet, "/api/v1/skills/git-workflow/download", nil)
	wZip := httptest.NewRecorder()
	engine.ServeHTTP(wZip, reqZip)
	assert.Equal(t, http.StatusOK, wZip.Code)
	assert.Equal(t, "application/zip", wZip.Header().Get("Content-Type"))
	assert.Contains(t, wZip.Header().Get("Content-Disposition"), "git-workflow.zip")
	assert.True(t, wZip.Body.Len() > 0)

	// 3. Test enterprise airoute_data_redact tool
	redactCallReq := map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      2,
		"method":  "tools/call",
		"params": map[string]interface{}{
			"name": "airoute_data_redact",
			"arguments": map[string]interface{}{
				"text": "客户手机 13812345678, 密钥 sk-abcdef123456789012345678",
			},
		},
	}
	bRedact, _ := json.Marshal(redactCallReq)
	reqRedact := httptest.NewRequest(http.MethodPost, "/mcp/messages", bytes.NewReader(bRedact))
	wRedact := httptest.NewRecorder()
	engine.ServeHTTP(wRedact, reqRedact)
	assert.Equal(t, http.StatusOK, wRedact.Code)
	assert.Contains(t, wRedact.Body.String(), "138****5678")
	assert.Contains(t, wRedact.Body.String(), "COMPLIANT_REDACTED")

	// 4. Test enterprise airoute_sql_security_check tool
	sqlCallReq := map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      3,
		"method":  "tools/call",
		"params": map[string]interface{}{
			"name": "airoute_sql_security_check",
			"arguments": map[string]interface{}{
				"sql": "DROP TABLE users;",
			},
		},
	}
	bSql, _ := json.Marshal(sqlCallReq)
	reqSql := httptest.NewRequest(http.MethodPost, "/mcp/messages", bytes.NewReader(bSql))
	wSql := httptest.NewRecorder()
	engine.ServeHTTP(wSql, reqSql)
	assert.Equal(t, http.StatusOK, wSql.Code)
	assert.Contains(t, wSql.Body.String(), "CRITICAL")
	assert.Contains(t, wSql.Body.String(), "DROP")

	// 5. Turn OFF git-workflow skill on-demand
	err = repo.SetSkillEnabled("git-workflow", false)
	assert.NoError(t, err)
	assert.False(t, repo.IsSkillEnabled("git-workflow"))

	// 6. Turn OFF Master MCP Switch
	_ = repo.SetSetting("mcp_enabled", "false")
	reqMCPDisabled := httptest.NewRequest(http.MethodPost, "/mcp/messages", bytes.NewReader(body))
	wMCPDisabled := httptest.NewRecorder()
	engine.ServeHTTP(wMCPDisabled, reqMCPDisabled)
	assert.Contains(t, wMCPDisabled.Body.String(), "MCP 服务在 AI 路由器中已按需关闭")
}
