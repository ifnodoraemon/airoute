package tests

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ifnodoraemon/nano-gateway/internal/api"
	"github.com/ifnodoraemon/nano-gateway/internal/controlplane"
	"github.com/ifnodoraemon/nano-gateway/internal/router"
	"github.com/ifnodoraemon/nano-gateway/internal/storage"
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

	// 1. Initial skills listing
	skills, err := repo.ListSkills()
	assert.NoError(t, err)
	assert.Equal(t, 5, len(skills))

	// 1.1 Test MCP 2026-07-28 server/discover (Stateless Discovery RPC)
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

	// 2. Query MCP tools/list - all tools should be present initially
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
	assert.Equal(t, 11, len(mcpResp.Result.Tools))

	// 2.1 Test Progressive Stage 1: nano_search_skills
	searchCallReq := map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      101,
		"method":  "tools/call",
		"params": map[string]interface{}{
			"name": "nano_search_skills",
			"arguments": map[string]interface{}{
				"query": "计算",
			},
		},
	}
	bSearch, _ := json.Marshal(searchCallReq)
	reqSearch := httptest.NewRequest(http.MethodPost, "/mcp/messages", bytes.NewReader(bSearch))
	wSearch := httptest.NewRecorder()
	engine.ServeHTTP(wSearch, reqSearch)
	assert.Equal(t, http.StatusOK, wSearch.Code)
	assert.Contains(t, wSearch.Body.String(), "code_runner")

	// 2.2 Test Progressive Stage 2: nano_inspect_skill
	inspCallReq := map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      102,
		"method":  "tools/call",
		"params": map[string]interface{}{
			"name": "nano_inspect_skill",
			"arguments": map[string]interface{}{
				"skill_id": "code_runner",
			},
		},
	}
	bInsp, _ := json.Marshal(inspCallReq)
	reqInsp := httptest.NewRequest(http.MethodPost, "/mcp/messages", bytes.NewReader(bInsp))
	wInsp := httptest.NewRecorder()
	engine.ServeHTTP(wInsp, reqInsp)
	assert.Equal(t, http.StatusOK, wInsp.Code)
	assert.Contains(t, wInsp.Body.String(), "nano_calc_eval")
	assert.Contains(t, wInsp.Body.String(), "Stage 2 Confirmed")

	// 2.3 Test Progressive Stage 3: nano_get_skill_manifest
	manCallReq := map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      103,
		"method":  "tools/call",
		"params": map[string]interface{}{
			"name": "nano_get_skill_manifest",
			"arguments": map[string]interface{}{
				"skill_id": "code_runner",
			},
		},
	}
	bMan, _ := json.Marshal(manCallReq)
	reqMan := httptest.NewRequest(http.MethodPost, "/mcp/messages", bytes.NewReader(bMan))
	wMan := httptest.NewRecorder()
	engine.ServeHTTP(wMan, reqMan)
	assert.Equal(t, http.StatusOK, wMan.Code)
	assert.Contains(t, wMan.Body.String(), "# 轻量代码执行与数学表达式计算")
	assert.Contains(t, wMan.Body.String(), "nano_calc_eval")

	// 3. Test nano_get_current_time tool
	timeCallReq := map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      2,
		"method":  "tools/call",
		"params": map[string]interface{}{
			"name":      "nano_get_current_time",
			"arguments": map[string]interface{}{},
		},
	}
	bTime, _ := json.Marshal(timeCallReq)
	reqTime := httptest.NewRequest(http.MethodPost, "/mcp/messages", bytes.NewReader(bTime))
	wTime := httptest.NewRecorder()
	engine.ServeHTTP(wTime, reqTime)
	assert.Equal(t, http.StatusOK, wTime.Code)
	assert.Contains(t, wTime.Body.String(), "CST")

	// 4. Test nano_calc_eval tool
	calcCallReq := map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      3,
		"method":  "tools/call",
		"params": map[string]interface{}{
			"name": "nano_calc_eval",
			"arguments": map[string]interface{}{
				"expression": "(10 + 20) * 2",
			},
		},
	}
	bCalc, _ := json.Marshal(calcCallReq)
	reqCalc := httptest.NewRequest(http.MethodPost, "/mcp/messages", bytes.NewReader(bCalc))
	wCalc := httptest.NewRecorder()
	engine.ServeHTTP(wCalc, reqCalc)
	assert.Equal(t, http.StatusOK, wCalc.Code)
	assert.Contains(t, wCalc.Body.String(), "60")

	// 5. Turn OFF web_search skill on-demand
	err = repo.SetSkillEnabled("web_search", false)
	assert.NoError(t, err)
	assert.False(t, repo.IsSkillEnabled("web_search"))
	assert.False(t, repo.IsToolEnabled("nano_web_search"))

	// tools/list should now have 10 tools (nano_web_search omitted)
	reqList2 := httptest.NewRequest(http.MethodPost, "/mcp/messages", bytes.NewReader(body))
	wList2 := httptest.NewRecorder()
	engine.ServeHTTP(wList2, reqList2)
	var mcpResp2 struct {
		Result struct {
			Tools []struct {
				Name string `json:"name"`
			} `json:"tools"`
		} `json:"result"`
	}
	_ = json.Unmarshal(wList2.Body.Bytes(), &mcpResp2)
	assert.Equal(t, 10, len(mcpResp2.Result.Tools))
	for _, tItem := range mcpResp2.Result.Tools {
		assert.NotEqual(t, "nano_web_search", tItem.Name)
	}

	// 6. Calling disabled tool should return error
	webCallReq := map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      4,
		"method":  "tools/call",
		"params": map[string]interface{}{
			"name": "nano_web_search",
			"arguments": map[string]interface{}{
				"query": "golang 1.26 features",
			},
		},
	}
	bWeb, _ := json.Marshal(webCallReq)
	reqWeb := httptest.NewRequest(http.MethodPost, "/mcp/messages", bytes.NewReader(bWeb))
	wWeb := httptest.NewRecorder()
	engine.ServeHTTP(wWeb, reqWeb)
	assert.Contains(t, wWeb.Body.String(), "已被按需停用")

	// 7. Turn OFF Master MCP Switch
	_ = repo.SetSetting("mcp_enabled", "false")
	reqMCPDisabled := httptest.NewRequest(http.MethodPost, "/mcp/messages", bytes.NewReader(body))
	wMCPDisabled := httptest.NewRecorder()
	engine.ServeHTTP(wMCPDisabled, reqMCPDisabled)
	assert.Contains(t, wMCPDisabled.Body.String(), "MCP 服务在 AI 路由器中已按需关闭")
}
