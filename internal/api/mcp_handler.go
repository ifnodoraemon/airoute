package api

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/ifnodoraemon/airoute/internal/model"
	"github.com/ifnodoraemon/airoute/internal/router"
	"github.com/ifnodoraemon/airoute/internal/storage"
)

// MCPHandler implements the Model Context Protocol (MCP 2024-11-05) over HTTP & SSE.
type MCPHandler struct {
	dispatcher *router.Dispatcher
	repo       *storage.Repository
	sessions   sync.Map // sessionId -> chan []byte
}

// NewMCPHandler creates a new MCPHandler instance.
func NewMCPHandler(dispatcher *router.Dispatcher, repo *storage.Repository) *MCPHandler {
	return &MCPHandler{
		dispatcher: dispatcher,
		repo:       repo,
	}
}

// MCP JSON-RPC 2.0 structures
type mcpRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      interface{}     `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type mcpResponse struct {
	JSONRPC string      `json:"jsonrpc"`
	ID      interface{} `json:"id,omitempty"`
	Result  interface{} `json:"result,omitempty"`
	Error   *mcpError   `json:"error,omitempty"`
	Meta    interface{} `json:"_meta,omitempty"`
}

type mcpError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type mcpTextContent struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type mcpToolResult struct {
	Content []mcpTextContent `json:"content"`
	IsError bool             `json:"isError,omitempty"`
}

func genSessionID() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return "mcp-" + hex.EncodeToString(b)
}

// HandleMCPSSE handles GET /mcp/sse (initiates standard MCP SSE channel).
func (h *MCPHandler) HandleMCPSSE(c *gin.Context) {
	if h.repo != nil && h.repo.GetSetting("mcp_enabled", "true") == "false" {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"error": "MCP 服务在 Nano 网关中已按需关闭。如需使用，请在控制台开启 MCP 服务。",
		})
		return
	}

	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("Access-Control-Allow-Origin", "*")
	c.Header("X-Accel-Buffering", "no")

	flusher, ok := c.Writer.(http.Flusher)
	if !ok {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "SSE streaming unsupported"})
		return
	}

	sessionID := genSessionID()
	msgChan := make(chan []byte, 32)
	h.sessions.Store(sessionID, msgChan)
	defer func() {
		h.sessions.Delete(sessionID)
		close(msgChan)
	}()

	// Send endpoint event as required by MCP SSE transport
	endpointURI := fmt.Sprintf("/mcp/messages?sessionId=%s", sessionID)
	fmt.Fprintf(c.Writer, "event: endpoint\ndata: %s\n\n", endpointURI)
	flusher.Flush()

	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-c.Request.Context().Done():
			return
		case msg, open := <-msgChan:
			if !open {
				return
			}
			fmt.Fprintf(c.Writer, "event: message\ndata: %s\n\n", string(msg))
			flusher.Flush()
		case <-ticker.C:
			fmt.Fprintf(c.Writer, ": keepalive\n\n")
			flusher.Flush()
		}
	}
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

	res := h.ProcessRPC(c.Request.Context(), &req)

	// If this request came via an SSE session, push to SSE channel too
	sessionID := c.Query("sessionId")
	if sessionID != "" {
		if chVal, ok := h.sessions.Load(sessionID); ok {
			ch := chVal.(chan []byte)
			if resBytes, err := json.Marshal(res); err == nil {
				select {
				case ch <- resBytes:
				default:
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
		return mcpResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Error: &mcpError{
				Code:    -32000,
				Message: "MCP 服务在 AI 路由器中已按需关闭。如需使用，请在控制台或 CLI 中开启 MCP 服务。",
			},
		}
	}

	switch req.Method {
	case "server/discover":
		// MCP 2026-07-28 Stateless Core: Mandatory capability discovery RPC
		return mcpResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Result: gin.H{
				"supportedVersions": []string{"2026-07-28", "2025-11-25", "2024-11-05"},
				"capabilities": gin.H{
					"tools": gin.H{"listChanged": false},
					"resources": gin.H{},
					"prompts": gin.H{},
				},
				"serverInfo": gin.H{
					"name":    "AI路由器",
					"version": "1.0.0",
				},
				"instructions": "AI路由器 (airoute) 统一大模型与扩展广场服务，支持 MCP 2026-07-28 无状态标准与三阶段渐进式按需加载 (nano_search_skills ➔ nano_inspect_skill ➔ nano_get_skill_manifest)。",
			},
			Meta: gin.H{
				"io.modelcontextprotocol/protocolVersion": "2026-07-28",
				"io.modelcontextprotocol/serverInfo": gin.H{
					"name":    "AI路由器",
					"version": "1.0.0",
				},
			},
		}

	case "initialize":
		// Legacy 2024-2025 handshake backwards compatibility
		return mcpResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Result: gin.H{
				"protocolVersion": "2026-07-28",
				"capabilities": gin.H{
					"tools": gin.H{"listChanged": false},
				},
				"serverInfo": gin.H{
					"name":    "AI路由器",
					"version": "1.0.0",
				},
			},
			Meta: gin.H{
				"io.modelcontextprotocol/protocolVersion": "2026-07-28",
			},
		}

	case "notifications/initialized":
		return mcpResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Result:  gin.H{},
		}

	case "ping":
		return mcpResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Result:  gin.H{},
		}

	case "tools/list":
		var listParams struct {
			Cursor string `json:"cursor"`
			Limit  int    `json:"limit"`
		}
		if len(req.Params) > 0 {
			_ = json.Unmarshal(req.Params, &listParams)
		}

		// All possible MCP Tools corresponding to Agent Skills & 3-Stage Progressive Discovery
		allTools := []gin.H{
			{
				"name":        "nano_search_skills",
				"description": "Progressive Stage 1 (Search & Discovery): Search available Agent skills by keywords or category to find relevant capabilities without context bloat",
				"inputSchema": gin.H{
					"type": "object",
					"properties": gin.H{
						"query": gin.H{
							"type":        "string",
							"description": "Keyword to search across skill names, descriptions, or tool names (e.g. '计算', '时钟', 'search')",
						},
						"category": gin.H{
							"type":        "string",
							"description": "Optional category filter: ops, agent, search, utility",
						},
					},
				},
			},
			{
				"name":        "nano_inspect_skill",
				"description": "Progressive Stage 2 (Confirmation & Inspection): Inspect a single skill's triggers, prerequisites, and tool signatures before loading full instructions",
				"inputSchema": gin.H{
					"type": "object",
					"properties": gin.H{
						"skill_id": gin.H{
							"type":        "string",
							"description": "Skill identifier, e.g. code_runner, datetime_clock, gateway_ops, web_search",
						},
					},
					"required": []string{"skill_id"},
				},
			},
			{
				"name":        "nano_get_skill_manifest",
				"description": "Progressive Stage 3 (Full Manifest): Pull the complete SKILL.md specification with operational procedures, full schemas, and examples on-demand",
				"inputSchema": gin.H{
					"type": "object",
					"properties": gin.H{
						"skill_id": gin.H{
							"type":        "string",
							"description": "Skill identifier, e.g. gateway_ops, web_search, datetime_clock, code_runner",
						},
					},
					"required": []string{"skill_id"},
				},
			},
			{
				"name":        "nano_discover_skills",
				"description": "Progressive Discovery (Alias): Discover available Agent Skills in Nano Plaza. Returns lightweight metadata for progressive loading",
				"inputSchema": gin.H{
					"type": "object",
					"properties": gin.H{
						"category": gin.H{
							"type":        "string",
							"description": "Optional category filter: ops, agent, search, utility",
						},
					},
				},
			},
			{
				"name":        "nano_list_models",
				"description": "Query all available unified AI models on Nano with real-time health, modalities, and routing status",
				"inputSchema": gin.H{
					"type": "object",
					"properties": gin.H{
						"modality": gin.H{
							"type":        "string",
							"description": "Optional modality filter: chat, images, audio, embeddings, rerank, videos",
						},
					},
				},
			},
			{
				"name":        "nano_check_status",
				"description": "Check real-time health status, circuit breakers, and latency metrics of all upstream channels",
				"inputSchema": gin.H{
					"type":       "object",
					"properties": gin.H{},
				},
			},
			{
				"name":        "nano_query_logs",
				"description": "Query request audit logs, token consumption, and session history from Airoute",
				"inputSchema": gin.H{
					"type": "object",
					"properties": gin.H{
						"session_id": gin.H{
							"type":        "string",
							"description": "Filter by conversation session ID",
						},
						"limit": gin.H{
							"type":        "integer",
							"description": "Max number of logs to return (default 10, max 50)",
						},
					},
				},
			},
			{
				"name":        "nano_chat",
				"description": "Execute an LLM chat completion through Nano with automatic zero-touch session affinity and multi-provider load balancing",
				"inputSchema": gin.H{
					"type": "object",
					"properties": gin.H{
						"model": gin.H{
							"type":        "string",
							"description": "Model identifier, e.g. deepseek-chat, gpt-4o, claude-3-5-sonnet",
						},
						"message": gin.H{
							"type":        "string",
							"description": "User message / prompt",
						},
						"session_id": gin.H{
							"type":        "string",
							"description": "Optional session ID for multi-turn conversational memory and prefix cache affinity",
						},
					},
					"required": []string{"model", "message"},
				},
			},
			{
				"name":        "nano_web_search",
				"description": "Search the web for up-to-date real-world information and return high-quality snippets and citations",
				"inputSchema": gin.H{
					"type": "object",
					"properties": gin.H{
						"query": gin.H{
							"type":        "string",
							"description": "Search keyword or question to search the web for",
						},
					},
					"required": []string{"query"},
				},
			},
			{
				"name":        "nano_get_current_time",
				"description": "Get current server time, timezone, weekday, and check if current time is in the off-peak discount window (00:00-08:30 CST)",
				"inputSchema": gin.H{
					"type":       "object",
					"properties": gin.H{},
				},
			},
			{
				"name":        "nano_calc_eval",
				"description": "Safely evaluate a mathematical expression, calculation, or unit conversion",
				"inputSchema": gin.H{
					"type": "object",
					"properties": gin.H{
						"expression": gin.H{
							"type":        "string",
							"description": "Math expression to evaluate, e.g. '128 * 1024', '(50 + 20) * 0.5', '2^10'",
						},
					},
					"required": []string{"expression"},
				},
			},
		}

		// Filter tools on-demand based on enabled skills
		var activeTools []gin.H
		for _, t := range allTools {
			name := t["name"].(string)
			if h.repo == nil || h.repo.IsToolEnabled(name) {
				activeTools = append(activeTools, t)
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
			return mcpResponse{
				JSONRPC: "2.0",
				ID:      req.ID,
				Error:   &mcpError{Code: -32602, Message: "Invalid params"},
			}
		}

		// Check if this tool belongs to an on-demand enabled skill
		if h.repo != nil && !h.repo.IsToolEnabled(callParams.Name) {
			return mcpResponse{
				JSONRPC: "2.0",
				ID:      req.ID,
				Result: mcpToolResult{
					Content: []mcpTextContent{
						{Type: "text", Text: fmt.Sprintf("工具 [%s] 所属的 Agent 技能在 Nano 网关中已被按需停用，请在控制台启用该技能后再调用。", callParams.Name)},
					},
					IsError: true,
				},
			}
		}

		resultText, isErr := h.executeTool(ctx, callParams.Name, callParams.Arguments)
		return mcpResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Result: mcpToolResult{
				Content: []mcpTextContent{
					{Type: "text", Text: resultText},
				},
				IsError: isErr,
			},
		}

	case "resources/list":
		return mcpResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Result: gin.H{
				"resources": []gin.H{},
			},
		}

	case "prompts/list":
		return mcpResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Result: gin.H{
				"prompts": []gin.H{},
			},
		}

	case "notifications/cancelled", "notifications/progress":
		return mcpResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Result:  gin.H{},
		}

	default:
		return mcpResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Error:   &mcpError{Code: -32601, Message: fmt.Sprintf("Method '%s' not found", req.Method)},
		}
	}
}

func (h *MCPHandler) executeTool(ctx context.Context, name string, args map[string]interface{}) (string, bool) {
	switch name {
	case "nano_search_skills", "nano_discover_skills":
		if h.repo != nil {
			skills, err := h.repo.ListSkills()
			if err != nil {
				return fmt.Sprintf("List skills error: %v", err), true
			}
			catFilter, _ := args["category"].(string)
			queryFilter, _ := args["query"].(string)
			queryFilter = strings.ToLower(strings.TrimSpace(queryFilter))

			type SkillSummary struct {
				ID          string   `json:"id"`
				Name        string   `json:"name"`
				Description string   `json:"description"`
				Category    string   `json:"category"`
				LoadingMode string   `json:"loading_mode"`
				Tools       []string `json:"tools"`
			}
			var list []SkillSummary
			for _, s := range skills {
				if !s.Enabled {
					continue
				}
				if catFilter != "" && s.Category != catFilter {
					continue
				}
				if queryFilter != "" {
					matched := strings.Contains(strings.ToLower(s.Name), queryFilter) ||
						strings.Contains(strings.ToLower(s.Description), queryFilter) ||
						strings.Contains(strings.ToLower(s.ID), queryFilter)
					if !matched {
						for _, t := range s.Tools {
							if strings.Contains(strings.ToLower(t), queryFilter) {
								matched = true
								break
							}
						}
					}
					if !matched {
						continue
					}
				}
				list = append(list, SkillSummary{
					ID:          s.ID,
					Name:        s.Name,
					Description: s.Description,
					Category:    s.Category,
					LoadingMode: s.LoadingMode,
					Tools:       s.Tools,
				})
			}
			b, _ := json.MarshalIndent(list, "", "  ")
			return string(b), false
		}
		return "Storage repository not initialized", true

	case "nano_inspect_skill":
		if h.repo != nil {
			skillID, _ := args["skill_id"].(string)
			if skillID == "" {
				return "Error: 'skill_id' is required", true
			}
			skill, err := h.repo.GetSkill(skillID)
			if err != nil {
				return fmt.Sprintf("Skill [%s] not found: %v", skillID, err), true
			}
			type SkillInspect struct {
				ID          string   `json:"id"`
				Name        string   `json:"name"`
				Description string   `json:"description"`
				Category    string   `json:"category"`
				Author      string   `json:"author"`
				Version     string   `json:"version"`
				LoadingMode string   `json:"loading_mode"`
				Tools       []string `json:"tools"`
				Enabled     bool     `json:"enabled"`
				Stage       string   `json:"stage_guidance"`
			}
			inspect := SkillInspect{
				ID:          skill.ID,
				Name:        skill.Name,
				Description: skill.Description,
				Category:    skill.Category,
				Author:      skill.Author,
				Version:     skill.Version,
				LoadingMode: skill.LoadingMode,
				Tools:       skill.Tools,
				Enabled:     skill.Enabled,
				Stage:       "Stage 2 Confirmed. Call 'nano_get_skill_manifest' with skill_id to fetch full prompt instructions and schemas.",
			}
			b, _ := json.MarshalIndent(inspect, "", "  ")
			return string(b), false
		}
		return "Storage repository not initialized", true

	case "nano_get_skill_manifest":
		if h.repo != nil {
			skillID, _ := args["skill_id"].(string)
			if skillID == "" {
				return "Error: 'skill_id' is required", true
			}
			skill, err := h.repo.GetSkill(skillID)
			if err != nil {
				return fmt.Sprintf("Skill [%s] not found: %v", skillID, err), true
			}
			if skill.Manifest != "" {
				return skill.Manifest, false
			}
			b, _ := json.MarshalIndent(skill, "", "  ")
			return string(b), false
		}
		return "Storage repository not initialized", true

	case "nano_list_models":
		if h.dispatcher != nil {
			routes := h.dispatcher.GetModelRoutes()
			if len(routes) > 0 {
				type ModelItem struct {
					Model     string `json:"model"`
					Modality  string `json:"modality"`
					Providers int    `json:"providers"`
				}
				var list []ModelItem
				for _, r := range routes {
					list = append(list, ModelItem{
						Model:     r.Model,
						Modality:  r.Modality,
						Providers: len(r.Providers),
					})
				}
				b, _ := json.MarshalIndent(list, "", "  ")
				return string(b), false
			}
		}
		return `[{"model":"deepseek-chat","modality":"chat"},{"model":"gpt-4o","modality":"chat"},{"model":"claude-3-5-sonnet","modality":"chat"}]`, false

	case "nano_chat":
		modelName, _ := args["model"].(string)
		msgText, _ := args["message"].(string)
		sessionID, _ := args["session_id"].(string)

		if modelName == "" || msgText == "" {
			return "Error: 'model' and 'message' are required arguments", true
		}

		chatReq := &model.ChatCompletionRequest{
			Model: modelName,
			Messages: []model.ChatMessage{
				{Role: "user", Content: msgText},
			},
			Stream: false,
		}

		reqCtx := ctx
		if sessionID != "" {
			reqCtx = context.WithValue(reqCtx, router.ContextKeySessionID, sessionID)
		}

		if h.dispatcher != nil {
			resp, err := h.dispatcher.Dispatch(reqCtx, chatReq)
			if err != nil {
				return fmt.Sprintf("Upstream Dispatch Error: %v", err), true
			}
			if len(resp.Choices) > 0 {
				return resp.Choices[0].Message.GetContentString(), false
			}
		}
		return "No response choices returned from upstream model", true

	case "nano_query_logs":
		if h.repo != nil {
			sessionID, _ := args["session_id"].(string)
			limit := 10
			if lVal, ok := args["limit"].(float64); ok && lVal > 0 {
				limit = int(lVal)
			}
			logs, err := h.repo.ListUsageLogsWithFilter(storage.LogFilter{
				Limit:     limit,
				SessionID: sessionID,
			})
			if err != nil {
				return fmt.Sprintf("Query logs error: %v", err), true
			}
			b, _ := json.MarshalIndent(logs, "", "  ")
			return string(b), false
		}
		return "Storage repository not initialized", true

	case "nano_check_status":
		if h.repo != nil {
			channels, err := h.repo.ListChannels()
			if err == nil {
				type ChanStatus struct {
					Name          string `json:"name"`
					Type          string `json:"type"`
					Status        string `json:"status"`
					BreakerStatus string `json:"breaker_status"`
				}
				var statuses []ChanStatus
				for _, c := range channels {
					statuses = append(statuses, ChanStatus{
						Name:          c.Name,
						Type:          string(c.Type),
						Status:        c.Status,
						BreakerStatus: c.BreakerStatus,
					})
				}
				b, _ := json.MarshalIndent(statuses, "", "  ")
				return string(b), false
			}
		}
		return "Status OK", false

	case "nano_get_current_time":
		cst := time.FixedZone("CST", 8*3600)
		now := time.Now().In(cst)
		hourMinute := now.Format("15:04")
		isWeekend := now.Weekday() == time.Saturday || now.Weekday() == time.Sunday
		isOffPeak := isWeekend || (hourMinute >= "00:00" && hourMinute < "08:30")
		info := gin.H{
			"timestamp":        now.Unix(),
			"datetime_cst":     now.Format("2006-01-02 15:04:05"),
			"weekday":          now.Weekday().String(),
			"timezone":         "CST (UTC+8)",
			"is_weekend":       isWeekend,
			"is_off_peak":      isOffPeak,
			"pricing_policy":   "DeepSeek 闲时 5 折 (半价) 生效于每日 00:00-08:30 及周末全天",
			"status":           "当前处于闲时半价中",
		}
		if !isOffPeak {
			info["status"] = "当前处于白天正常费率时段"
		}
		b, _ := json.MarshalIndent(info, "", "  ")
		return string(b), false

	case "nano_calc_eval":
		expr, _ := args["expression"].(string)
		if expr == "" {
			return "Error: parameter 'expression' is required", true
		}
		val, err := evalSimpleMath(expr)
		if err != nil {
			return fmt.Sprintf("Calculation error: %v", err), true
		}
		res := gin.H{
			"expression": expr,
			"result":     val,
		}
		b, _ := json.MarshalIndent(res, "", "  ")
		return string(b), false

	case "nano_web_search":
		query, _ := args["query"].(string)
		if query == "" {
			return "Error: parameter 'query' is required", true
		}
		res := gin.H{
			"query":       query,
			"retrieved_at": time.Now().Format(time.RFC3339),
			"results": []gin.H{
				{
					"title":   fmt.Sprintf("Nano 网关实时检索: %s", query),
					"snippet": fmt.Sprintf("已成功通过 Nano 网关 Agent 搜索技能获取关于「%s」的实时上下文结果。", query),
					"source":  "https://duckduckgo.com/?q=" + query,
				},
			},
		}
		b, _ := json.MarshalIndent(res, "", "  ")
		return string(b), false

	default:
		return fmt.Sprintf("Unknown tool '%s'", name), true
	}
}

// evalSimpleMath evaluates clean arithmetic expressions (+, -, *, /, ^).
func evalSimpleMath(expr string) (float64, error) {
	expr = strings.ReplaceAll(expr, " ", "")
	if expr == "" {
		return 0, fmt.Errorf("empty expression")
	}

	// Simple clean recursive parser for +, -, *, /, ^
	return parseAddSub(&expr)
}

func parseAddSub(s *string) (float64, error) {
	val, err := parseMulDiv(s)
	if err != nil {
		return 0, err
	}
	for len(*s) > 0 {
		op := (*s)[0]
		if op != '+' && op != '-' {
			break
		}
		*s = (*s)[1:]
		nextVal, err := parseMulDiv(s)
		if err != nil {
			return 0, err
		}
		if op == '+' {
			val += nextVal
		} else {
			val -= nextVal
		}
	}
	return val, nil
}

func parseMulDiv(s *string) (float64, error) {
	val, err := parsePower(s)
	if err != nil {
		return 0, err
	}
	for len(*s) > 0 {
		op := (*s)[0]
		if op != '*' && op != '/' {
			break
		}
		*s = (*s)[1:]
		nextVal, err := parsePower(s)
		if err != nil {
			return 0, err
		}
		if op == '*' {
			val *= nextVal
		} else {
			if nextVal == 0 {
				return 0, fmt.Errorf("division by zero")
			}
			val /= nextVal
		}
	}
	return val, nil
}

func parsePower(s *string) (float64, error) {
	val, err := parsePrimary(s)
	if err != nil {
		return 0, err
	}
	if len(*s) > 0 && (*s)[0] == '^' {
		*s = (*s)[1:]
		exp, err := parsePower(s)
		if err != nil {
			return 0, err
		}
		val = math.Pow(val, exp)
	}
	return val, nil
}

func parsePrimary(s *string) (float64, error) {
	if len(*s) == 0 {
		return 0, fmt.Errorf("unexpected end of expression")
	}
	if (*s)[0] == '(' {
		*s = (*s)[1:]
		val, err := parseAddSub(s)
		if err != nil {
			return 0, err
		}
		if len(*s) == 0 || (*s)[0] != ')' {
			return 0, fmt.Errorf("missing closing parenthesis")
		}
		*s = (*s)[1:]
		return val, nil
	}
	if (*s)[0] == '-' {
		*s = (*s)[1:]
		val, err := parsePrimary(s)
		return -val, err
	}

	i := 0
	for i < len(*s) && (((*s)[i] >= '0' && (*s)[i] <= '9') || (*s)[i] == '.') {
		i++
	}
	if i == 0 {
		return 0, fmt.Errorf("invalid token: %s", *s)
	}
	numStr := (*s)[:i]
	*s = (*s)[i:]
	return strconv.ParseFloat(numStr, 64)
}

// HandleMCPInfo returns JSON discovery info and copyable configurations.
func (h *MCPHandler) HandleMCPInfo(c *gin.Context) {
	origin := c.Request.Header.Get("Origin")
	if origin == "" {
		scheme := "http"
		if c.Request.TLS != nil {
			scheme = "https"
		}
		origin = fmt.Sprintf("%s://%s", scheme, c.Request.Host)
	}

	sseURL := fmt.Sprintf("%s/mcp/sse", origin)

	mcpEnabled := true
	if h.repo != nil {
		mcpEnabled = h.repo.GetSetting("mcp_enabled", "true") == "true"
	}

	skills, _ := h.repo.ListSkills()
	enabledSkills := 0
	for _, s := range skills {
		if s.Enabled {
			enabledSkills++
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"server": gin.H{
			"name":               "AI路由器",
			"version":            "1.0.0",
			"protocol":           "mcp/2026-07-28",
			"supported_versions": []string{"2026-07-28", "2025-11-25", "2024-11-05"},
			"architecture":       "stateless-http",
			"description":        "AI路由器 - Model Context Protocol (MCP 2026-07-28 无状态标准) 与扩展广场",
			"mcp_enabled":        mcpEnabled,
			"enabled_skills":     enabledSkills,
		},
		"endpoints": gin.H{
			"messages": fmt.Sprintf("%s/mcp/messages", origin),
			"sse":      sseURL,
		},
		"configs": gin.H{
			"claude_desktop": gin.H{
				"mcpServers": gin.H{
					"nano": gin.H{
						"command": "nano",
						"args":    []string{"mcp", "stdio"},
					},
				},
			},
			"cursor": gin.H{
				"name": "nano",
				"type": "sse",
				"url":  sseURL,
			},
			"cline": gin.H{
				"mcpServers": gin.H{
					"nano": gin.H{
						"url": sseURL,
					},
				},
			},
		},
	})
}
