package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"
)

// handleStatus probes the Airoute gateway health and active channel topology.
func handleStatus(endpoint, token string) {
	fmt.Printf("%s正在探测 Airoute 路由网关 [%s]...%s\n", colorDim, endpoint, colorReset)

	client := &http.Client{Timeout: 5 * time.Second}
	req, _ := http.NewRequest(http.MethodGet, endpoint+"/health", nil)
	resp, err := client.Do(req)
	if err != nil {
		fmt.Printf("%s✕ 无法连接到 Airoute 路由网关服务 (%v)%s\n请确认网关容器或服务正在运行 (当前目标端点: %s)\n", colorRed, err, colorReset, endpoint)
		os.Exit(1)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusOK {
		fmt.Printf("%s● 网关健康状态: 在线正常 (200 OK)%s\n\n", colorGreen+colorBold, colorReset)
	} else {
		fmt.Printf("%s▲ 网关响应异常 (HTTP %d)%s\n\n", colorYellow, resp.StatusCode, colorReset)
	}

	// Fetch public status
	reqStatus, _ := http.NewRequest(http.MethodGet, endpoint+"/api/v1/public/status", nil)
	if respStatus, err := client.Do(reqStatus); err == nil && respStatus.StatusCode == 200 {
		var res struct {
			Code int `json:"code"`
			Data struct {
				Status   string `json:"status"`
				Channels []struct {
					Name   string `json:"name"`
					Type   string `json:"type"`
					Status string `json:"status"`
					Probe  struct {
						LatencyMs int `json:"latency_ms"`
					} `json:"probe"`
				} `json:"channels"`
				Models []struct {
					Model string `json:"model"`
				} `json:"models"`
			} `json:"data"`
		}
		if err := json.NewDecoder(respStatus.Body).Decode(&res); err == nil && res.Code == 0 {
			fmt.Printf("%s[上游渠道提供商]%s (共 %d 个):\n", colorBold, colorReset, len(res.Data.Channels))
			for _, ch := range res.Data.Channels {
				statusBadge := colorGreen + "健康 (HEALTHY)" + colorReset
				if ch.Status == "tripped" {
					statusBadge = colorRed + "熔断 (TRIPPED)" + colorReset
				}
				fmt.Printf("  • %-20s %-10s %s (%d ms)\n", ch.Name, "["+ch.Type+"]", statusBadge, ch.Probe.LatencyMs)
			}
			fmt.Printf("\n%s[已激活可用模型]%s: %d 个模型路由\n", colorBold, colorReset, len(res.Data.Models))
		}
		respStatus.Body.Close()
	}

	// Fetch MCP status
	reqMcp, _ := http.NewRequest(http.MethodGet, endpoint+"/mcp", nil)
	if respMcp, err := client.Do(reqMcp); err == nil && respMcp.StatusCode == 200 {
		var mcpInfo struct {
			Server struct {
				Name          string `json:"name"`
				McpEnabled    bool   `json:"mcp_enabled"`
				EnabledSkills int    `json:"enabled_skills"`
			} `json:"server"`
		}
		if err := json.NewDecoder(respMcp.Body).Decode(&mcpInfo); err == nil {
			mcpBadge := colorGreen + "已启用 (ENABLED)" + colorReset
			if !mcpInfo.Server.McpEnabled {
				mcpBadge = colorDim + "已关闭 (DISABLED)" + colorReset
			}
			fmt.Printf("\n%s[MCP 协议服务]%s: %s (已按需挂载 %d 个技能)\n", colorBold, colorReset, mcpBadge, mcpInfo.Server.EnabledSkills)
		}
		respMcp.Body.Close()
	}
}

// handleModels lists all available models registered on the gateway.
func handleModels(endpoint, token string) {
	client := &http.Client{Timeout: 5 * time.Second}
	req, _ := http.NewRequest(http.MethodGet, endpoint+"/v1/models", nil)
	if token != "" {
		req.Header.Set("Authorization", getAuthHeader(token))
	}
	resp, err := client.Do(req)
	if err != nil {
		fmt.Printf("%s请求失败: %v%s\n", colorRed, err, colorReset)
		return
	}
	defer resp.Body.Close()

	var modelsResp struct {
		Data []struct {
			ID      string `json:"id"`
			OwnedBy string `json:"owned_by"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&modelsResp); err != nil {
		fmt.Printf("%s解析响应失败: %v%s\n", colorRed, err, colorReset)
		return
	}

	fmt.Printf("%sAiroute 统一模型目录%s (共 %d 个模型):\n", colorBold+colorCyan, colorReset, len(modelsResp.Data))
	fmt.Printf("%-32s %-20s %s\n", "模型标识 (MODEL)", "归属渠道 (OWNER)", "计费策略")
	fmt.Println(strings.Repeat("-", 72))

	for _, m := range modelsResp.Data {
		rateDesc := "标准计费 (夜间 00:00-08:30 享 5 折)"
		if strings.Contains(m.ID, "dall-e") || strings.Contains(m.ID, "whisper") || strings.Contains(m.ID, "tts") {
			rateDesc = "按次固定计费"
		}
		fmt.Printf("%-32s %-20s %s\n", colorGreen+m.ID+colorReset, m.OwnedBy, colorDim+rateDesc+colorReset)
	}
}
