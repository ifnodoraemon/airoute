package controlplane

import (
	"github.com/gin-gonic/gin"
	"net/http"
	"strings"
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
