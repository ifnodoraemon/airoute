package api

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"

	"github.com/gin-gonic/gin"
	"github.com/ifnodoraemon/airoute/internal/router"
	"github.com/ifnodoraemon/airoute/internal/storage"
)

// MCPHandler implements the Model Context Protocol over HTTP & SSE.
//
// It speaks every revision in mcpSupportedVersions (newest first), negotiates
// down to the revision a client asks for, and defaults to
// mcpDefaultProtocolVersion.
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

// mcpSupportedVersions lists the protocol revisions this server implements,
// newest first. Negotiation answers with the client's version when it is in this
// list and with the newest supported version otherwise (MCP lifecycle spec).
var mcpSupportedVersions = []string{"2026-07-28", "2025-11-25", "2024-11-05"}

// mcpDefaultProtocolVersion is the newest revision this server speaks.
const mcpDefaultProtocolVersion = "2026-07-28"

// mcpProtocolSupported reports whether version is one we implement.
func mcpProtocolSupported(version string) bool {
	for _, v := range mcpSupportedVersions {
		if v == version {
			return true
		}
	}
	return false
}

// negotiateMCPProtocolVersion picks the version to answer with. An empty/unknown
// request falls back to the newest supported revision.
func negotiateMCPProtocolVersion(requested string) string {
	requested = strings.TrimSpace(requested)
	if requested != "" && mcpProtocolSupported(requested) {
		return requested
	}
	return mcpDefaultProtocolVersion
}

// requestedProtocolVersion extracts params.protocolVersion from an initialize
// request.
func requestedProtocolVersion(req *mcpRequest) string {
	if req == nil || len(req.Params) == 0 {
		return ""
	}
	var params struct {
		ProtocolVersion string `json:"protocolVersion"`
	}
	if err := json.Unmarshal(req.Params, &params); err != nil {
		return ""
	}
	return params.ProtocolVersion
}

// isJSONRPCNotification reports whether req is a JSON-RPC notification. Per the
// JSON-RPC 2.0 spec a request without an id is a notification and the server MUST
// NOT send a response for it; MCP's Streamable HTTP transport answers 202 with an
// empty body.
func isJSONRPCNotification(req *mcpRequest) bool {
	return req != nil && req.ID == nil
}

// isMCPNotificationMethod reports whether the method is one of MCP's
// fire-and-forget notifications.
func isMCPNotificationMethod(method string) bool {
	return strings.HasPrefix(method, "notifications/")
}

// authenticateCaller verifies that the caller has a valid active API key once
// any key has been registered. Open mode is only for a deployment that has
// never created a key: otherwise disabling every key would silently reopen MCP
// to anonymous callers (and with it tools/list, skills and remote MCP servers).
func (h *MCPHandler) authenticateCaller(key string) (*storage.APIKeyRecord, error) {
	if h.repo == nil || h.repo.CountAPIKeys() == 0 {
		return nil, nil // Open mode only when no key has ever been created
	}
	if key == "" {
		return nil, fmt.Errorf("API key required for MCP access")
	}
	// A presented credential is cleartext; compare in digest space.
	return h.authenticateCallerDigest(storage.HashAPIKey(key))
}

// authenticateCallerDigest resolves a caller whose key is already the stored
// digest (the SSE session path stores caller.Key, which is the digest).
func (h *MCPHandler) authenticateCallerDigest(keyDigest string) (*storage.APIKeyRecord, error) {
	if h.repo == nil || h.repo.CountAPIKeys() == 0 {
		return nil, nil // Open mode only when no key has ever been created
	}
	if keyDigest == "" {
		return nil, fmt.Errorf("API key required for MCP access")
	}
	rec, err := h.repo.GetAPIKeyByKey(keyDigest)
	if err != nil || rec == nil || rec.Status != "active" {
		return nil, fmt.Errorf("invalid or inactive API key")
	}
	return rec, nil
}

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

	// Normalize the presented credential to the stored digest immediately: session
	// pinning, the caller context and the downstream lookups all compare/record
	// digests, never cleartext.
	callerKey := storage.HashAPIKey(extractMCPKey(c))
	sessionID := c.Query("sessionId")
	var sess *mcpSession
	if sessionID != "" {
		if sVal, ok := h.sessions.Load(sessionID); ok {
			sess, _ = sVal.(*mcpSession)
		}
	}
	// Standard MCP SSE transport: with no credential on the POST, the sessionId
	// is the capability and the session's key authenticates the request. When an
	// explicit key IS supplied it must be the session owner's: falling back to the
	// session key would let any valid key inject responses into another caller's
	// stream (A17). The mismatch is rejected below instead.
	if callerKey == "" && sess != nil {
		callerKey = sess.apiKey
	}

	reqCtx := c.Request.Context()
	caller, accessErr := h.resolveCallerDigest(callerKey)
	if accessErr != nil {
		respondMCPAccessError(c, accessErr)
		return
	}
	if sess != nil && sess.apiKey != "" && callerKey != sess.apiKey {
		c.JSON(http.StatusForbidden, mcpResponse{
			JSONRPC: "2.0",
			Error:   &mcpError{Code: -32000, Message: "Forbidden: this SSE session belongs to a different API key"},
		})
		return
	}
	if !h.checkCallerRateLimits(reqCtx, c, caller) {
		return
	}
	reqCtx = context.WithValue(reqCtx, contextKeyCallerKey, caller.Key)
	reqCtx = context.WithValue(reqCtx, contextKeyCallerRecord, caller)

	// A10: a JSON-RPC request without an id is a notification. The spec forbids
	// replying to it; the Streamable HTTP transport acknowledges with 202 and an
	// empty body. Answering with a JSON-RPC result made strict clients see a
	// response to a message that must not have one.
	if isJSONRPCNotification(&req) {
		protocolVersion := negotiateMCPProtocolVersion(c.GetHeader("MCP-Protocol-Version"))
		c.Header("MCP-Protocol-Version", protocolVersion)
		c.Header("Access-Control-Expose-Headers", "MCP-Protocol-Version")
		c.Status(http.StatusAccepted)
		return
	}

	// A10: protocol version is negotiated per request. initialize answers with
	// the client's requested revision when supported; every other call echoes the
	// version the client declared in the MCP-Protocol-Version header.
	protocolVersion := negotiateMCPProtocolVersion(c.GetHeader("MCP-Protocol-Version"))
	if req.Method == "initialize" {
		protocolVersion = negotiateMCPProtocolVersion(requestedProtocolVersion(&req))
	}

	res := h.ProcessRPC(reqCtx, &req)

	// If this request came via an SSE session, push to SSE channel too
	if sessionID != "" && sess != nil {
		if resBytes, err := json.Marshal(res); err == nil {
			sess.Send(resBytes)
		}
	}

	c.Header("MCP-Protocol-Version", protocolVersion)
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
	if res, isErr, handled := h.executeBuiltinUtilityTools(ctx, name, args); handled {
		return res, isErr
	}
	return h.executeDynamicMCPTool(ctx, name, args)
}
