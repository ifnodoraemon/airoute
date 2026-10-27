package controlplane

import (
	"fmt"
	"net/http"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/ifnodoraemon/airoute/internal/storage"
)

// resolvePublicBaseURL returns the public base URL for current request.
func (h *AdminHandler) resolvePublicBaseURL(c *gin.Context) string {
	return ResolvePublicBaseURL(c)
}

// normalizeMCPURL trims surrounding whitespace and trailing slashes so the
// gateway's own registered public MCP URL can be compared exactly.
func normalizeMCPURL(raw string) string {
	return strings.TrimRight(strings.TrimSpace(raw), "/")
}

// gatewayMCPPublicURLs lists the URLs that identify this gateway's own MCP
// endpoint. They are compared exactly (after normalising trailing slashes),
// never by substring: an external server such as https://host/mcp or
// https://mcp.example.com/sse is a different service that must be dialled and
// displayed as-is.
func gatewayMCPPublicURLs(baseURL string) []string {
	base := normalizeMCPURL(baseURL)
	if base == "" {
		return nil
	}
	return []string{
		base + "/mcp",
		base + "/mcp/sse",
		base + "/mcp/messages",
	}
}

// isLocalGatewayMCPServer reports whether a registered MCP server is this
// gateway itself rather than a third-party server. The historical check used
// strings.Contains(endpoint, "/mcp"), which classified every external
// https://host/mcp endpoint as local and then rewrote its displayed URL and
// faked its probe result. Local detection is now exact:
//   - the built-in gateway record id ("airoute-gateway"), or
//   - a relative endpoint (the gateway serves it, e.g. "/mcp/sse"), or
//   - an absolute endpoint that equals this gateway's own public MCP URL.
func isLocalGatewayMCPServer(id, endpoint, baseURL string) bool {
	if id == "airoute-gateway" {
		return true
	}
	trimmed := strings.TrimSpace(endpoint)
	if strings.HasPrefix(trimmed, "/") {
		return true
	}
	candidate := normalizeMCPURL(trimmed)
	if candidate == "" {
		return false
	}
	for _, own := range gatewayMCPPublicURLs(baseURL) {
		if strings.EqualFold(candidate, own) {
			return true
		}
	}
	return false
}

// GetMCPSettings returns the master MCP enable/disable switch and config.
func (h *AdminHandler) GetMCPSettings(c *gin.Context) {
	enabled := h.repo.GetSetting("mcp_enabled", "true") == "true"
	skills, _ := h.repo.ListSkills()
	servers, _ := h.repo.ListMCPServers()
	enabledSkillsCount := 0
	totalToolsCount := 0
	for _, s := range skills {
		if s.Enabled {
			enabledSkillsCount++
			totalToolsCount += len(s.Tools)
		}
	}
	activeServersCount := 0
	for _, s := range servers {
		if s.Enabled {
			activeServersCount++
		}
	}

	baseURL := h.resolvePublicBaseURL(c)

	c.JSON(http.StatusOK, gin.H{
		"code": 0,
		"data": gin.H{
			"mcp_enabled":          enabled,
			"enabled_skills_count": enabledSkillsCount,
			"total_skills_count":   len(skills),
			"active_servers_count": activeServersCount,
			"total_servers_count":  len(servers),
			"active_tools_count":   totalToolsCount,
			"sse_endpoint":         baseURL + "/mcp/sse",
			"messages_endpoint":    baseURL + "/mcp/messages",
			"public_base_url":      baseURL,
		},
	})
}

// UpdateMCPSettingsRequest defines update payload.
type UpdateMCPSettingsRequest struct {
	MCPEnabled bool `json:"mcp_enabled"`
}

// UpdateMCPSettings sets the master MCP enable/disable switch.
func (h *AdminHandler) UpdateMCPSettings(c *gin.Context) {
	var req UpdateMCPSettingsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "error": "请求参数不合法"})
		return
	}

	val := "false"
	if req.MCPEnabled {
		val = "true"
	}
	if err := h.repo.SetSetting("mcp_enabled", val); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "error": "更新 MCP 配置失败: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"code": 0, "message": "MCP 服务状态已更新", "mcp_enabled": req.MCPEnabled})
}

// ListMCPServers returns all registered ModelScope-style MCP Servers.
func (h *AdminHandler) ListMCPServers(c *gin.Context) {
	servers, err := h.repo.ListMCPServers()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "error": "获取 MCP 服务器列表失败: " + err.Error()})
		return
	}

	baseURL := h.resolvePublicBaseURL(c)
	for _, s := range servers {
		// Only the gateway's own record is rewritten to the current public URL;
		// an external server keeps the endpoint the operator registered.
		if isLocalGatewayMCPServer(s.ID, s.Endpoint, baseURL) {
			s.Endpoint = baseURL + "/mcp/sse"
		}
	}

	c.JSON(http.StatusOK, gin.H{"code": 0, "data": servers})
}

// SaveMCPServerRequest payload for adding or updating an MCP Server.
type SaveMCPServerRequest struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Category    string   `json:"category"`
	Transport   string   `json:"transport"`
	Endpoint    string   `json:"endpoint"`
	Author      string   `json:"author"`
	Version     string   `json:"version"`
	Tools       []string `json:"tools"`
	Prompts     []string `json:"prompts"`
	Resources   []string `json:"resources"`
	EnvVars     string   `json:"env_vars"`
	Enabled     bool     `json:"enabled"`
}

// SaveMCPServer creates or updates a custom MCP Server.
func (h *AdminHandler) SaveMCPServer(c *gin.Context) {
	var req SaveMCPServerRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "error": "请求参数不合法: " + err.Error()})
		return
	}
	if strings.TrimSpace(req.Name) == "" || strings.TrimSpace(req.Endpoint) == "" {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "error": "MCP 服务名称和 Endpoint 不能为空"})
		return
	}
	if req.ID == "" {
		req.ID = "custom-" + strconv.FormatInt(time.Now().Unix(), 10)
	}
	if req.Category == "" {
		req.Category = "dev"
	}
	if req.Transport == "" {
		req.Transport = "sse"
	}
	if len(req.Tools) == 0 {
		req.Tools = []string{req.ID + "_tool"}
	}

	record := &storage.MCPServerRecord{
		ID:          req.ID,
		Name:        req.Name,
		Description: req.Description,
		Category:    req.Category,
		Transport:   req.Transport,
		Endpoint:    req.Endpoint,
		Status:      "online",
		Author:      req.Author,
		Version:     req.Version,
		Tools:       req.Tools,
		Prompts:     req.Prompts,
		Resources:   req.Resources,
		EnvVars:     req.EnvVars,
		Enabled:     req.Enabled,
	}

	if err := h.repo.SaveMCPServer(record); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "error": "保存 MCP 服务器失败: " + err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 0, "message": "MCP 服务保存成功", "data": record})
}

// ToggleMCPServer enables or disables an MCP Server.
func (h *AdminHandler) ToggleMCPServer(c *gin.Context) {
	id := c.Param("id")
	if id == "" {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "error": "MCP 服务 ID 不能为空"})
		return
	}

	var req ToggleSkillRequest
	if err := c.ShouldBindJSON(&req); err == nil && req.Enabled != nil {
		if err := h.repo.SetMCPServerEnabled(id, *req.Enabled); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "error": "更新状态失败: " + err.Error()})
			return
		}
	} else {
		srv, err := h.repo.GetMCPServer(id)
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"code": 404, "error": "未找到该 MCP 服务"})
			return
		}
		if err := h.repo.SetMCPServerEnabled(id, !srv.Enabled); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "error": "切换状态失败: " + err.Error()})
			return
		}
	}

	c.JSON(http.StatusOK, gin.H{"code": 0, "message": "MCP 服务状态更新成功"})
}

// DeleteMCPServer deletes an MCP Server.
func (h *AdminHandler) DeleteMCPServer(c *gin.Context) {
	id := c.Param("id")
	if id == "" {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "error": "MCP 服务 ID 不能为空"})
		return
	}
	if err := h.repo.DeleteMCPServer(id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "error": "删除 MCP 服务失败: " + err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 0, "message": "MCP 服务已删除"})
}

// ProbeMCPServer tests connectivity to an MCP Server with genuine network probes.
func (h *AdminHandler) ProbeMCPServer(c *gin.Context) {
	id := c.Param("id")
	srv, err := h.repo.GetMCPServer(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"code": 404, "error": "未找到该 MCP 服务"})
		return
	}

	baseURL := h.resolvePublicBaseURL(c)
	endpoint := srv.Endpoint
	isLocalGateway := isLocalGatewayMCPServer(srv.ID, endpoint, baseURL)

	var status string
	var latencyMs int64
	var probeErr string

	if isLocalGateway {
		endpoint = baseURL + "/mcp/sse"
		status = "online"
		latencyMs = 1
	} else if strings.HasPrefix(endpoint, "http://") || strings.HasPrefix(endpoint, "https://") {
		if err := validateProbeURL(endpoint); err != nil {
			status = "offline"
			probeErr = err.Error()
			c.JSON(http.StatusOK, gin.H{"code": 0, "data": gin.H{"status": status, "latency_ms": latencyMs, "error": probeErr}})
			return
		}
		start := time.Now()
		probeClient := safeProbeClient(5 * time.Second)
		probeReq, reqErr := http.NewRequestWithContext(c.Request.Context(), http.MethodGet, endpoint, nil)
		if reqErr != nil {
			status = "offline"
			probeErr = reqErr.Error()
		} else {
			probeReq.Header.Set("User-Agent", "Airoute-MCP-Probe/1.0")
			probeReq.Header.Set("Accept", "text/event-stream, application/json, */*")
			resp, doErr := probeClient.Do(probeReq)
			latencyMs = time.Since(start).Milliseconds()
			if doErr != nil {
				status = "offline"
				probeErr = doErr.Error()
			} else {
				resp.Body.Close()
				if resp.StatusCode < 500 {
					status = "online"
				} else {
					status = "offline"
					probeErr = fmt.Sprintf("HTTP status %d", resp.StatusCode)
				}
			}
		}
	} else if srv.Transport == "stdio" {
		// Verify local runtime binary for stdio command
		parts := strings.Fields(endpoint)
		if len(parts) > 0 {
			if _, lookErr := exec.LookPath(parts[0]); lookErr == nil {
				status = "online"
			} else {
				status = "configured"
				probeErr = fmt.Sprintf("Command '%s' not found in PATH", parts[0])
			}
		} else {
			status = "configured"
		}
		latencyMs = 0
	} else {
		status = "configured"
		latencyMs = 0
	}

	// Update status in storage
	_ = h.repo.UpdateMCPServerStatus(srv.ID, status)

	res := gin.H{
		"id":          srv.ID,
		"name":        srv.Name,
		"endpoint":    endpoint,
		"status":      status,
		"latency_ms":  latencyMs,
		"transport":   srv.Transport,
		"tools_count": len(srv.Tools),
		"checked_at":  time.Now().Format("2006-01-02 15:04:05"),
	}
	if probeErr != "" {
		res["probe_error"] = probeErr
	}

	c.JSON(http.StatusOK, gin.H{
		"code": 0,
		"data": res,
	})
}
