package api

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"sync"

	"github.com/gin-gonic/gin"
	"github.com/ifnodoraemon/airoute/internal/router"
	"github.com/ifnodoraemon/airoute/internal/storage"
)

// MCPHandler implements the Model Context Protocol (MCP 2024-11-05) over HTTP & SSE.
type MCPHandler struct {
	dispatcher *router.Dispatcher
	repo       *storage.Repository
	sessions   sync.Map // sessionId -> *mcpSession
}

// NewMCPHandler creates a new MCPHandler instance.
func NewMCPHandler(dispatcher *router.Dispatcher, repo *storage.Repository) *MCPHandler {
	return &MCPHandler{
		dispatcher: dispatcher,
		repo:       repo,
	}
}

type mcpContextKey string

const contextKeyCallerKey mcpContextKey = "mcp_caller_key"

// HandleMCPMessages handles POST /mcp/messages (JSON-RPC 2.0 over HTTP).
func (h *MCPHandler) HandleMCPMessages(c *gin.Context) {
	body, err := io.ReadAll(c.Request.Body)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Failed to read body"})
		return
	}

	var req mcpRequest
	if err := json.Unmarshal(body, &req); err != nil {
		c.JSON(http.StatusBadRequest, mcpResponse{
			JSONRPC: "2.0",
			Error:   &mcpError{Code: -32700, Message: "Parse error"},
		})
		return
	}

	callerKey := extractMCPKey(c)
	sessionID := c.Query("sessionId")
	if sessionID != "" {
		if sVal, ok := h.sessions.Load(sessionID); ok {
			if sess, ok := sVal.(*mcpSession); ok && callerKey == "" {
				callerKey = sess.apiKey
			}
		}
	}

	reqCtx := c.Request.Context()
	if callerKey != "" {
		reqCtx = context.WithValue(reqCtx, contextKeyCallerKey, callerKey)
	}

	res := h.ProcessRPC(reqCtx, &req)

	// If this request came via an SSE session, push to SSE channel too
	if sessionID != "" {
		if sVal, ok := h.sessions.Load(sessionID); ok {
			if sess, ok := sVal.(*mcpSession); ok {
				if resBytes, err := json.Marshal(res); err == nil {
					sess.Send(resBytes)
				}
			}
		}
	}

	c.Header("MCP-Protocol-Version", "2026-07-28")
	c.Header("Access-Control-Expose-Headers", "MCP-Protocol-Version")
	c.JSON(http.StatusOK, res)
}

// ProcessRPC handles a JSON-RPC 2.0 request (public for stdio CLI adapter reuse).
func (h *MCPHandler) ProcessRPC(ctx context.Context, req *mcpRequest) mcpResponse {
	// 1. Check MCP Master Switch
	if h.repo != nil && h.repo.GetSetting("mcp_enabled", "true") == "false" {
		return newMCPErrorResponse(req.ID, -32000, "MCP 服务在 AI 路由器中已按需关闭。如需使用，请在控制台或 CLI 中开启 MCP 服务。")
	}

	switch req.Method {
	case "server/discover":
		// MCP 2026-07-28 Stateless Core: Mandatory capability discovery RPC
		return newMCPResultResponse(req.ID, gin.H{
			"supportedVersions": []string{"2026-07-28", "2025-11-25", "2024-11-05"},
			"capabilities": gin.H{
				"tools":     gin.H{"listChanged": false},
				"resources": gin.H{},
				"prompts":   gin.H{},
			},
			"serverInfo": gin.H{
				"name":    "AI路由器",
				"version": "1.0.0",
			},
			"instructions": "AI路由器 (airoute) 统一大模型与扩展广场服务，支持 MCP 2026-07-28 无状态标准与三阶段渐进式按需加载 (airoute_search_skills ➔ airoute_inspect_skill ➔ airoute_get_skill_manifest)。",
		}, gin.H{
			"io.modelcontextprotocol/protocolVersion": "2026-07-28",
			"io.modelcontextprotocol/serverInfo": gin.H{
				"name":    "AI路由器",
				"version": "1.0.0",
			},
		})

	case "initialize":
		// Legacy 2024-2025 handshake backwards compatibility
		return newMCPResultResponse(req.ID, gin.H{
			"protocolVersion": "2026-07-28",
			"capabilities": gin.H{
				"tools": gin.H{"listChanged": false},
			},
			"serverInfo": gin.H{
				"name":    "AI路由器",
				"version": "1.0.0",
			},
		}, gin.H{
			"io.modelcontextprotocol/protocolVersion": "2026-07-28",
		})

	case "notifications/initialized":
		return newMCPResultResponse(req.ID, gin.H{})

	case "ping":
		return newMCPResultResponse(req.ID, gin.H{})

	case "tools/list":
		var listParams struct {
			Cursor string `json:"cursor"`
			Limit  int    `json:"limit"`
		}
		if len(req.Params) > 0 {
			_ = json.Unmarshal(req.Params, &listParams)
		}

		// All possible MCP Tools corresponding to Agent Skills & 3-Stage Progressive Discovery
		allTools := getBuiltinMCPTools()

		// Filter tools on-demand based on enabled skills
		var activeTools []gin.H
		existingToolNames := make(map[string]bool)
		for _, t := range allTools {
			name := t["name"].(string)
			if h.repo == nil || h.repo.IsToolEnabled(name) {
				activeTools = append(activeTools, t)
				existingToolNames[name] = true
			}
		}

		// Dynamically include tools from enabled MCP plaza servers
		if h.repo != nil {
			if servers, err := h.repo.ListMCPServers(); err == nil {
				for _, srv := range servers {
					if !srv.Enabled {
						continue
					}
					for _, tName := range srv.Tools {
						if !existingToolNames[tName] {
							existingToolNames[tName] = true
							activeTools = append(activeTools, gin.H{
								"name":        tName,
								"description": fmt.Sprintf("MCP 工具 [%s] (由 %s 服务提供)", tName, srv.Name),
								"inputSchema": gin.H{
									"type":       "object",
									"properties": gin.H{},
								},
							})
						}
					}
				}
			}
		}

		// Support MCP 2024-11-05 pagination (cursor & nextCursor)
		startIndex := 0
		if listParams.Cursor != "" {
			if idx, err := strconv.Atoi(listParams.Cursor); err == nil && idx >= 0 && idx < len(activeTools) {
				startIndex = idx
			}
		}
		pageSize := len(activeTools)
		if listParams.Limit > 0 && listParams.Limit < pageSize {
			pageSize = listParams.Limit
		}

		endIndex := startIndex + pageSize
		if endIndex > len(activeTools) {
			endIndex = len(activeTools)
		}

		paginatedTools := activeTools[startIndex:endIndex]
		result := gin.H{
			"tools": paginatedTools,
		}
		if endIndex < len(activeTools) {
			result["nextCursor"] = strconv.Itoa(endIndex)
		}

		return mcpResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Result:  result,
		}

	case "tools/call":
		var callParams struct {
			Name      string                 `json:"name"`
			Arguments map[string]interface{} `json:"arguments"`
		}
		if err := json.Unmarshal(req.Params, &callParams); err != nil {
			return newMCPErrorResponse(req.ID, -32602, "Invalid params")
		}

		// Check if this tool belongs to an on-demand enabled skill
		if h.repo != nil && !h.repo.IsToolEnabled(callParams.Name) {
			return newMCPToolResponse(req.ID, fmt.Sprintf("工具 [%s] 所属的 Agent 技能在 Airoute 网关中已被按需停用，请在控制台启用该技能后再调用。", callParams.Name), true)
		}

		resultText, isErr := h.executeTool(ctx, callParams.Name, callParams.Arguments)
		return newMCPToolResponse(req.ID, resultText, isErr)

	case "resources/list":
		return newMCPResultResponse(req.ID, gin.H{
			"resources": []gin.H{},
		})

	case "prompts/list":
		return newMCPResultResponse(req.ID, gin.H{
			"prompts": []gin.H{},
		})

	case "notifications/cancelled", "notifications/progress":
		return newMCPResultResponse(req.ID, gin.H{})

	default:
		return newMCPErrorResponse(req.ID, -32601, fmt.Sprintf("Method '%s' not found", req.Method))
	}
}

// executeTool dispatches tool execution across domain modules.
func (h *MCPHandler) executeTool(ctx context.Context, name string, args map[string]interface{}) (string, bool) {
	if res, isErr, handled := h.executeSkillTools(ctx, name, args); handled {
		return res, isErr
	}
	if res, isErr, handled := h.executeOpsTools(ctx, name, args); handled {
		return res, isErr
	}
	if res, isErr, handled := h.executeSecurityTools(ctx, name, args); handled {
		return res, isErr
	}
	if res, isErr, handled := h.executeRoutingTools(ctx, name, args); handled {
		return res, isErr
	}
	if res, isErr, handled := h.executeUtilityAndMockTools(ctx, name, args); handled {
		return res, isErr
	}
	return h.executeDynamicMCPTool(name, args)
}

