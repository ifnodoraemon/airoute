package api

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
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

// executeTool dispatches tool execution across domain modules.
func (h *MCPHandler) executeTool(ctx context.Context, name string, args map[string]interface{}) (string, bool) {
	if !h.builtinMCPToolEnabled(name) {
		return "Airoute native MCP server is disabled", true
	}
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
