package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"
)

// handleMCP manages MCP gateway settings and runs the stdio bridge.
func handleMCP(endpoint, token string, args []string) {
	if len(args) == 0 || args[0] == "status" {
		client := &http.Client{Timeout: 5 * time.Second}
		req, _ := http.NewRequest(http.MethodGet, endpoint+"/api/v1/admin/mcp/settings", nil)
		if token != "" {
			req.Header.Set("Authorization", getAuthHeader(token))
		}
		resp, err := client.Do(req)
		if err != nil {
			fmt.Printf("%s查询 MCP 状态失败: %v%s\n", colorRed, err, colorReset)
			return
		}
		defer resp.Body.Close()

		var res struct {
			Code int `json:"code"`
			Data struct {
				MCPEnabled         bool   `json:"mcp_enabled"`
				EnabledSkillsCount int    `json:"enabled_skills_count"`
				ActiveToolsCount   int    `json:"active_tools_count"`
				SSEEndpoint        string `json:"sse_endpoint"`
			} `json:"data"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&res); err == nil && res.Code == 0 {
			statusStr := colorGreen + "● 已开启 (ENABLED)" + colorReset
			if !res.Data.MCPEnabled {
				statusStr = colorRed + "○ 已按需关闭 (DISABLED)" + colorReset
			}
			fmt.Printf("\n%sMCP (Model Context Protocol) 服务状态:%s\n", colorBold, colorReset)
			fmt.Printf("  • 服务开关: %s\n", statusStr)
			fmt.Printf("  • 挂载技能: %d 个活跃技能\n", res.Data.EnabledSkillsCount)
			fmt.Printf("  • 暴露工具: %d 个动态工具\n", res.Data.ActiveToolsCount)
			fmt.Printf("  • SSE 路径: %s%s\n", endpoint, res.Data.SSEEndpoint)
		}
		return
	}

	subCmd := args[0]
	if subCmd == "enable" || subCmd == "disable" {
		client := &http.Client{Timeout: 5 * time.Second}
		payload, _ := json.Marshal(map[string]bool{"mcp_enabled": subCmd == "enable"})
		req, _ := http.NewRequest(http.MethodPost, endpoint+"/api/v1/admin/mcp/settings", bytes.NewReader(payload))
		req.Header.Set("Content-Type", "application/json")
		if token != "" {
			req.Header.Set("Authorization", getAuthHeader(token))
		}
		resp, err := client.Do(req)
		if err != nil {
			fmt.Printf("%s更新 MCP 设置失败: %v%s\n", colorRed, err, colorReset)
			return
		}
		defer resp.Body.Close()
		fmt.Printf("✔ MCP 服务已成功更新为: %s\n", subCmd)
		return
	}

	if subCmd == "stdio" {
		// Run stdio JSON-RPC bridge for Claude Desktop / Cursor
		runMCPStdioBridge(endpoint)
		return
	}

	fmt.Printf("%s未知 mcp 子命令: %s (可用: status, enable, disable, stdio)%s\n", colorRed, subCmd, colorReset)
}

// runMCPStdioBridge translates local Stdin/Stdout JSON-RPC into HTTP MCP messages.
func runMCPStdioBridge(endpoint string) {
	scanner := bufio.NewScanner(os.Stdin)
	client := &http.Client{Timeout: 60 * time.Second}

	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}

		req, err := http.NewRequest(http.MethodPost, endpoint+"/mcp/messages", bytes.NewReader(line))
		if err != nil {
			continue
		}
		req.Header.Set("Content-Type", "application/json")

		resp, err := client.Do(req)
		if err != nil {
			errResp := fmt.Sprintf(`{"jsonrpc":"2.0","error":{"code":-32603,"message":"Failed to connect to Airoute Gateway: %s"}}`, err.Error())
			fmt.Println(errResp)
			continue
		}

		respBody, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		fmt.Println(string(respBody))
	}
}
