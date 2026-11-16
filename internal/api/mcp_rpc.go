package api

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"

	"github.com/gin-gonic/gin"
)

// ProcessRPC dispatches a JSON-RPC request to the MCP method handlers. Extracted
// from mcp_handler.go to keep the transport/handler file under the size red line.
func (h *MCPHandler) ProcessRPC(ctx context.Context, req *mcpRequest) mcpResponse {
	// 1. Check MCP Master Switch
	if h.repo != nil && h.repo.GetSetting("mcp_enabled", "true") == "false" {
		return newMCPErrorResponse(req.ID, -32000, "MCP service is disabled on this gateway. Enable it in the console or CLI to use MCP tools.")
	}

	switch req.Method {
	case "server/discover":
		// MCP 2026-07-28 Stateless Core: Mandatory capability discovery RPC
		return newMCPResultResponse(req.ID, gin.H{
			"supportedVersions": mcpSupportedVersions,
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
		// Legacy 2024-2025 handshake backwards compatibility. A10: honour the
		// revision the client asked for instead of always claiming the newest one;
		// clients reject a server version they do not implement.
		negotiated := negotiateMCPProtocolVersion(requestedProtocolVersion(req))
		return newMCPResultResponse(req.ID, gin.H{
			"protocolVersion": negotiated,
			"capabilities": gin.H{
				"tools": gin.H{"listChanged": false},
			},
			"serverInfo": gin.H{
				"name":    "AI路由器",
				"version": "1.0.0",
			},
		}, gin.H{
			"io.modelcontextprotocol/protocolVersion": negotiated,
		})

	case "notifications/initialized", "notifications/cancelled", "notifications/progress":
		// A10: notifications must not receive a JSON-RPC response. The HTTP
		// handler answers 202 with an empty body; this empty value is the
		// defensive contract for any other transport that reuses ProcessRPC.
		if isJSONRPCNotification(req) || isMCPNotificationMethod(req.Method) {
			return mcpResponse{}
		}
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
		nativeEnabled := h.nativeMCPServerEnabled()

		// Filter tools on-demand based on enabled skills
		var activeTools []gin.H
		existingToolNames := make(map[string]bool)
		for _, t := range allTools {
			name := t["name"].(string)
			if nativeEnabled && (h.repo == nil || h.repo.IsToolEnabled(name)) {
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
						if !existingToolNames[tName] && (!builtinMCPToolSet[tName] || nativeEnabled) {
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

		// A16: infrastructure-disclosure tools (channel inventory, routing
		// topology) are administrator-only. The gate used to be "is there a valid
		// API key", so any tenant could enumerate the upstream fleet.
		if isAdminOnlyMCPTool(callParams.Name) && !h.callerIsAdmin(ctx, callParams.Arguments) {
			return newMCPErrorResponse(req.ID, -32002,
				fmt.Sprintf("Permission denied: tool '%s' is restricted to gateway administrators", callParams.Name))
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

	default:
		return newMCPErrorResponse(req.ID, -32601, fmt.Sprintf("Method '%s' not found", req.Method))
	}
}