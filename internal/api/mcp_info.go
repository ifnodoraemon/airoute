package api

import (
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/ifnodoraemon/airoute/internal/controlplane"
)

// HandleMCPInfo returns JSON discovery info and copyable configurations.
func (h *MCPHandler) HandleMCPInfo(c *gin.Context) {
	origin := controlplane.ResolvePublicBaseURL(c)
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
					"airoute-gateway": gin.H{
						"command": "airoute",
						"args":    []string{"mcp", "stdio"},
					},
				},
			},
			"cursor": gin.H{
				"name": "airoute-gateway",
				"type": "sse",
				"url":  sseURL,
			},
			"cline": gin.H{
				"mcpServers": gin.H{
					"airoute-gateway": gin.H{
						"url": sseURL,
					},
				},
			},
		},
	})
}
