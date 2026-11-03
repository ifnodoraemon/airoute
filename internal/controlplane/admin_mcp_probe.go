package controlplane

import (
	"fmt"
	"github.com/gin-gonic/gin"
	"github.com/ifnodoraemon/airoute/internal/model"
	"net/http"
	"os/exec"
	"strings"
	"time"
)

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
		status = "configured"
		if !srv.Enabled || h.repo.GetSetting("mcp_enabled", "true") == "false" {
			status, probeErr = "offline", "MCP 服务已停用"
		}
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
				if resp.StatusCode >= 200 && resp.StatusCode < 300 {
					status = "online"
				} else {
					status = "offline"
					probeErr = fmt.Sprintf("HTTP status %d", resp.StatusCode)
				}
			}
		}
	} else if srv.Transport == "stdio" {
		// Verify local runtime binary for stdio command
		parts, parseErr := model.ParseMCPCommand(endpoint)
		if parseErr != nil {
			status, probeErr = "offline", parseErr.Error()
		} else if len(parts) > 0 {
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
