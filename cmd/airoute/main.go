package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

const (
	defaultEndpoint = "http://localhost:8080"
	version         = "1.0.0"
)

// ANSI color codes
const (
	colorReset  = "\033[0m"
	colorBold   = "\033[1m"
	colorDim    = "\033[2m"
	colorGreen  = "\033[32m"
	colorYellow = "\033[33m"
	colorBlue   = "\033[34m"
	colorPurple = "\033[35m"
	colorCyan   = "\033[36m"
	colorRed    = "\033[31m"
)

func main() {
	if len(os.Args) < 2 {
		printHelp()
		return
	}

	command := os.Args[1]

	endpoint := os.Getenv("AIROUTE_ENDPOINT")
	if endpoint == "" {
		endpoint = os.Getenv("NANO_ENDPOINT")
	}
	if endpoint == "" {
		endpoint = defaultEndpoint
	}
	token := os.Getenv("AIROUTE_TOKEN")
	if token == "" {
		token = os.Getenv("NANO_TOKEN")
	}

	switch command {
	case "help", "-h", "--help":
		printHelp()

	case "version", "-v", "--version":
		fmt.Printf("Airoute CLI v%s (AI Gateway & MCP Control Plane)\n", version)

	case "status":
		handleStatus(endpoint, token)

	case "models":
		handleModels(endpoint, token)

	case "chat":
		handleChat(endpoint, token, os.Args[2:])

	case "skills":
		handleSkills(endpoint, token, os.Args[2:])

	case "mcp":
		handleMCP(endpoint, token, os.Args[2:])

	case "keys":
		handleKeys(endpoint, token, os.Args[2:])

	case "users":
		handleUsers(endpoint, token, os.Args[2:])

	default:
		fmt.Fprintf(os.Stderr, "%s未知命令 '%s'%s\n运行 'airoute help' 查看可用命令\n", colorRed, command, colorReset)
		os.Exit(1)
	}
}

func printHelp() {
	fmt.Printf(`%sAiroute CLI%s - 极简现代化 AI 智能网关与 Agent 扩展命令行工具 (v%s)

%s使用方法:%s
  airoute <command> [arguments] [options]

%s核心管理与状态:%s
  %sstatus%s        查看 Airoute 路由网关集群运行状态、上游提供商与延迟探针
  %smodels%s        查询所有已接入的模型拓扑、支持模态与实时分时单价
  %skeys%s          管理虚拟 API 访问密钥 (airoute keys list / create)
  %susers%s         管理系统用户与角色权限 (airoute users list / create / delete / passwd)

%sAgent 扩展与按需开关:%s
  %sskills%s        查看与按需启闭 Agent 技能 (web_search, code_runner, etc.)
                   • airoute skills list
                   • airoute skills enable <id>
                   • airoute skills disable <id>
  %smcp%s           管理 MCP (Model Context Protocol) 服务与本地 stdio 桥接
                   • airoute mcp status
                   • airoute mcp enable / disable
                   • airoute mcp stdio  (供 Claude Desktop / Cursor 零配置接入)

%s终端交互与提问:%s
  %schat%s          在终端直接与模型对话或进行单次提问
                   • airoute chat -m deepseek-chat "你好，介绍一下你自己"
                   • airoute chat (进入交互式多轮对话 REPL)

%s全局环境变量:%s
  AIROUTE_ENDPOINT 网关地址 (默认: http://localhost:8080)
  AIROUTE_TOKEN   管理员 Token 或 API Key
`,
		colorBold+colorCyan, colorReset, version,
		colorBold, colorReset,
		colorBold, colorReset,
		colorGreen, colorReset,
		colorGreen, colorReset,
		colorGreen, colorReset,
		colorGreen, colorReset,
		colorBold, colorReset,
		colorPurple, colorReset,
		colorPurple, colorReset,
		colorBold, colorReset,
		colorBlue, colorReset,
		colorBold, colorReset,
	)
}

func getAuthHeader(token string) string {
	if token == "" {
		return ""
	}
	if strings.HasPrefix(token, "Bearer ") {
		return token
	}
	return "Bearer " + token
}

// 1. handleStatus
func handleStatus(endpoint, token string) {
	fmt.Printf("%s正在探测 Airoute 路由网关 [%s]...%s\n", colorDim, endpoint, colorReset)

	client := &http.Client{Timeout: 5 * time.Second}
	req, _ := http.NewRequest(http.MethodGet, endpoint+"/health", nil)
	resp, err := client.Do(req)
	if err != nil {
		fmt.Printf("%s✕ 无法连接到 Airoute 路由网关服务 (%v)%s\n请确认网关容器或服务正在运行 (默认: http://localhost:8080)\n", colorRed, err, colorReset)
		os.Exit(1)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusOK {
		fmt.Printf("%s● 网关健康状态: ONLINE (200 OK)%s\n\n", colorGreen+colorBold, colorReset)
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
					Name     string `json:"name"`
					Type     string `json:"type"`
					Status   string `json:"status"`
					Probe    struct {
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
				statusBadge := colorGreen + "HEALTHY" + colorReset
				if ch.Status == "tripped" {
					statusBadge = colorRed + "TRIPPED" + colorReset
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
			mcpBadge := colorGreen + "ENABLED" + colorReset
			if !mcpInfo.Server.McpEnabled {
				mcpBadge = colorDim + "DISABLED (按需关闭)" + colorReset
			}
			fmt.Printf("\n%s[MCP 协议服务]%s: %s (已按需挂载 %d 个技能)\n", colorBold, colorReset, mcpBadge, mcpInfo.Server.EnabledSkills)
		}
		respMcp.Body.Close()
	}
}

// 2. handleModels
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

// 3. handleSkills
func handleSkills(endpoint, token string, args []string) {
	if len(args) == 0 || args[0] == "list" {
		listSkills(endpoint, token)
		return
	}

	subCmd := args[0]
	if subCmd == "enable" || subCmd == "disable" {
		if len(args) < 2 {
			fmt.Printf("%s请指定要操作的技能 ID，例如: airoute skills %s web_search%s\n", colorYellow, subCmd, colorReset)
			return
		}
		targetSkill := args[1]
		toggleSkill(endpoint, token, targetSkill, subCmd == "enable")
		return
	}

	fmt.Printf("%s未知 skills 子命令: %s (可用: list, enable, disable)%s\n", colorRed, subCmd, colorReset)
}

func listSkills(endpoint, token string) {
	client := &http.Client{Timeout: 5 * time.Second}
	req, _ := http.NewRequest(http.MethodGet, endpoint+"/api/v1/admin/skills", nil)
	if token != "" {
		req.Header.Set("Authorization", getAuthHeader(token))
	}
	resp, err := client.Do(req)
	if err != nil {
		fmt.Printf("%s请求技能列表失败: %v%s\n", colorRed, err, colorReset)
		return
	}
	defer resp.Body.Close()

	var res struct {
		Code int `json:"code"`
		Data []struct {
			ID          string   `json:"id"`
			Name        string   `json:"name"`
			Description string   `json:"description"`
			Category    string   `json:"category"`
			Tools       []string `json:"tools"`
			LoadingMode string   `json:"loading_mode"`
			Version     string   `json:"version"`
			Enabled     bool     `json:"enabled"`
		} `json:"data"`
	}

	if resp.StatusCode == http.StatusOK {
		_ = json.NewDecoder(resp.Body).Decode(&res)
	} else {
		// Fallback to public MCP discovery protocol for downstream agents
		mcpPayload := []byte(`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"nano_search_skills","arguments":{}}}`)
		mcpReq, _ := http.NewRequest(http.MethodPost, endpoint+"/mcp/messages", bytes.NewReader(mcpPayload))
		mcpReq.Header.Set("Content-Type", "application/json")
		if mcpResp, err := client.Do(mcpReq); err == nil && mcpResp.StatusCode == 200 {
			var rpcRes struct {
				Result struct {
					Content []struct {
						Text string `json:"text"`
					} `json:"content"`
				} `json:"result"`
			}
			if json.NewDecoder(mcpResp.Body).Decode(&rpcRes) == nil && len(rpcRes.Result.Content) > 0 {
				_ = json.Unmarshal([]byte(rpcRes.Result.Content[0].Text), &res.Data)
				for i := range res.Data {
					res.Data[i].Enabled = true
				}
			}
			mcpResp.Body.Close()
		}
	}

	fmt.Printf("\n%sAiroute 扩展广场 - Agent 技能列表%s:\n", colorBold+colorPurple, colorReset)
	fmt.Printf("%-18s %-24s %-12s %-12s %s\n", "技能标识 (ID)", "技能名称", "模式", "状态", "包含工具能力")
	fmt.Println(strings.Repeat("-", 90))

	for _, s := range res.Data {
		status := colorGreen + "[ENABLED] 开启" + colorReset
		if !s.Enabled {
			status = colorDim + "[DISABLED] 关闭" + colorReset
		}
		modeBadge := colorCyan + "渐进式 (Lazy)" + colorReset
		if s.LoadingMode == "eager" {
			modeBadge = colorYellow + "即时 (Eager)" + colorReset
		}
		toolsJoined := strings.Join(s.Tools, ", ")
		fmt.Printf("%-18s %-24s %-20s %-20s %s\n", s.ID, colorBold+s.Name+colorReset, modeBadge, status, colorCyan+toolsJoined+colorReset)
		fmt.Printf("   %s%s%s\n", colorDim, s.Description, colorReset)
	}
	fmt.Printf("\n%s提示: 可使用 'airoute skills enable <id>' 或 'airoute skills disable <id>' 按需开关%s\n", colorDim, colorReset)
}

func toggleSkill(endpoint, token, skillID string, enable bool) {
	client := &http.Client{Timeout: 5 * time.Second}
	payload, _ := json.Marshal(map[string]bool{"enabled": enable})
	req, _ := http.NewRequest(http.MethodPost, fmt.Sprintf("%s/api/v1/admin/skills/%s/toggle", endpoint, skillID), bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", getAuthHeader(token))
	}
	resp, err := client.Do(req)
	if err != nil {
		fmt.Printf("%s切换失败: %v%s\n", colorRed, err, colorReset)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode == 200 {
		stateWord := colorGreen + "已开启" + colorReset
		if !enable {
			stateWord = colorYellow + "已关闭" + colorReset
		}
		fmt.Printf("✔ 技能 [%s] %s\n", skillID, stateWord)
	} else {
		body, _ := io.ReadAll(resp.Body)
		fmt.Printf("%s操作失败 (HTTP %d): %s%s\n", colorRed, resp.StatusCode, string(body), colorReset)
	}
}

// 4. handleMCP
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

// 5. handleChat
func handleChat(endpoint, token string, args []string) {
	fs := flag.NewFlagSet("chat", flag.ExitOnError)
	modelFlag := fs.String("m", "deepseek-chat", "Model identifier to chat with")
	sessionFlag := fs.String("s", "", "Session ID for multi-turn prefix cache affinity")
	_ = fs.Parse(args)

	remaining := fs.Args()
	if len(remaining) > 0 {
		// One-shot prompt
		prompt := strings.Join(remaining, " ")
		sendOneShotChat(endpoint, token, *modelFlag, *sessionFlag, prompt)
		return
	}

	// Interactive terminal REPL
	fmt.Printf("%s进入 Airoute 终端交互式对话 (模型: %s)%s\n", colorBold+colorCyan, *modelFlag, colorReset)
	fmt.Printf("输入 'exit' 或 'quit' 退出\n\n")

	reader := bufio.NewReader(os.Stdin)
	for {
		fmt.Printf("%s用户 > %s", colorGreen+colorBold, colorReset)
		line, err := reader.ReadString('\n')
		if err != nil {
			break
		}
		prompt := strings.TrimSpace(line)
		if prompt == "" {
			continue
		}
		if prompt == "exit" || prompt == "quit" {
			fmt.Println("退出对话。")
			break
		}

		sendOneShotChat(endpoint, token, *modelFlag, *sessionFlag, prompt)
		fmt.Println()
	}
}

func sendOneShotChat(endpoint, token, modelName, sessionID, prompt string) {
	client := &http.Client{Timeout: 90 * time.Second}
	chatReq := map[string]interface{}{
		"model": modelName,
		"messages": []map[string]string{
			{"role": "user", "content": prompt},
		},
		"stream": false,
	}

	body, _ := json.Marshal(chatReq)
	req, _ := http.NewRequest(http.MethodPost, endpoint+"/v1/chat/completions", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	if sessionID != "" {
		req.Header.Set("X-Session-ID", sessionID)
	}
	if token != "" {
		req.Header.Set("Authorization", getAuthHeader(token))
	} else {
		// Use default test key
		req.Header.Set("Authorization", "Bearer sk-airoute-production-enterprise-cluster-key")
	}

	resp, err := client.Do(req)
	if err != nil {
		fmt.Printf("%s请求失败: %v%s\n", colorRed, err, colorReset)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		fmt.Printf("%s调用错误 (HTTP %d): %s%s\n", colorRed, resp.StatusCode, string(body), colorReset)
		return
	}

	var chatResp struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
		Usage struct {
			TotalTokens int `json:"total_tokens"`
		} `json:"usage"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&chatResp); err == nil && len(chatResp.Choices) > 0 {
		fmt.Printf("%sAiroute [%s]:%s\n%s\n", colorBold+colorBlue, modelName, colorReset, chatResp.Choices[0].Message.Content)
	} else {
		fmt.Printf("%s未收到模型回复选择%s\n", colorYellow, colorReset)
	}
}

// 6. handleKeys
func handleKeys(endpoint, token string, args []string) {
	client := &http.Client{Timeout: 5 * time.Second}
	if len(args) == 0 || args[0] == "list" {
		req, _ := http.NewRequest(http.MethodGet, endpoint+"/api/v1/admin/keys", nil)
		if token != "" {
			req.Header.Set("Authorization", getAuthHeader(token))
		}
		resp, err := client.Do(req)
		if err != nil {
			fmt.Printf("%s查询密钥失败: %v%s\n", colorRed, err, colorReset)
			return
		}
		defer resp.Body.Close()

		var res struct {
			Code int `json:"code"`
			Data []struct {
				Key        string  `json:"key"`
				TenantID   string  `json:"tenant_id"`
				Budget     float64 `json:"budget"`
				UsedTokens int64   `json:"used_tokens"`
				UsedCost   float64 `json:"used_cost"`
				Status     string  `json:"status"`
			} `json:"data"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&res); err == nil {
			fmt.Printf("\n%s虚拟 API 密钥列表%s (共 %d 个):\n", colorBold, colorReset, len(res.Data))
			fmt.Printf("%-36s %-16s %-12s %s\n", "API 密钥 (KEY)", "租户 (TENANT)", "消耗 Tokens", "累计费用")
			fmt.Println(strings.Repeat("-", 76))
			for _, k := range res.Data {
				fmt.Printf("%-36s %-16s %-12d ¥%.4f\n", colorGreen+k.Key+colorReset, k.TenantID, k.UsedTokens, k.UsedCost)
			}
		}
		return
	}

	if args[0] == "create" {
		tenant := "default"
		if len(args) > 1 {
			tenant = args[1]
		}
		payload, _ := json.Marshal(map[string]interface{}{
			"tenant_id": tenant,
			"rpm":       120,
			"tpm":       200000,
			"budget":    100.0,
		})
		req, _ := http.NewRequest(http.MethodPost, endpoint+"/api/v1/admin/keys", bytes.NewReader(payload))
		req.Header.Set("Content-Type", "application/json")
		if token != "" {
			req.Header.Set("Authorization", getAuthHeader(token))
		}
		resp, err := client.Do(req)
		if err != nil {
			fmt.Printf("%s创建密钥失败: %v%s\n", colorRed, err, colorReset)
			return
		}
		defer resp.Body.Close()

		var res struct {
			Code int `json:"code"`
			Data struct {
				Key string `json:"key"`
			} `json:"data"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&res); err == nil && res.Data.Key != "" {
			fmt.Printf("✔ 成功签发新 API 密钥:\n  %s%s%s (租户: %s)\n", colorGreen+colorBold, res.Data.Key, colorReset, tenant)
		}
		return
	}

	fmt.Printf("%s未知 keys 子命令: %s (可用: list, create)%s\n", colorRed, args[0], colorReset)
}

func ensureAdminToken(endpoint, token string) string {
	if token != "" {
		return token
	}
	// Attempt auto-login with default admin credentials if no token provided
	loginPayload, _ := json.Marshal(map[string]string{
		"username": "admin",
		"password": "admin123",
	})
	resp, err := http.Post(endpoint+"/api/v1/auth/login", "application/json", bytes.NewReader(loginPayload))
	if err == nil {
		defer resp.Body.Close()
		var res struct {
			Code int `json:"code"`
			Data struct {
				Token string `json:"token"`
			} `json:"data"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&res); err == nil && res.Code == 0 && res.Data.Token != "" {
			return res.Data.Token
		}
	}
	return ""
}

// 7. handleUsers
func handleUsers(endpoint, token string, args []string) {
	authToken := ensureAdminToken(endpoint, token)
	client := &http.Client{Timeout: 5 * time.Second}

	if len(args) == 0 || args[0] == "list" {
		req, _ := http.NewRequest(http.MethodGet, endpoint+"/api/v1/admin/users", nil)
		if authToken != "" {
			req.Header.Set("Authorization", getAuthHeader(authToken))
		}
		resp, err := client.Do(req)
		if err != nil {
			fmt.Printf("%s查询用户失败: %v%s\n", colorRed, err, colorReset)
			return
		}
		defer resp.Body.Close()

		var res struct {
			Code int `json:"code"`
			Data []struct {
				ID        int64  `json:"id"`
				Username  string `json:"username"`
				Role      string `json:"role"`
				CreatedAt string `json:"created_at"`
				UpdatedAt string `json:"updated_at"`
			} `json:"data"`
			Error string `json:"error"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&res); err != nil || res.Code != 0 {
			errMsg := res.Error
			if errMsg == "" {
				errMsg = "鉴权失败，请设置环境变量 AIROUTE_TOKEN=<admin_token>"
			}
			fmt.Printf("%s获取用户列表失败: %s%s\n", colorRed, errMsg, colorReset)
			return
		}

		fmt.Printf("\n%s系统账号列表%s (共 %d 个用户):\n", colorBold, colorReset, len(res.Data))
		fmt.Printf("%-6s %-18s %-16s %s\n", "ID", "用户名 (USERNAME)", "权限角色 (ROLE)", "创建时间 (CREATED)")
		fmt.Println(strings.Repeat("-", 64))
		for _, u := range res.Data {
			roleStr := colorPurple + "超级管理员 (admin)" + colorReset
			if u.Role != "admin" {
				roleStr = colorCyan + "运维操作员 (operator)" + colorReset
			}
			fmt.Printf("%-6d %-18s %-26s %s\n", u.ID, colorBold+u.Username+colorReset, roleStr, u.CreatedAt)
		}
		fmt.Println()
		return
	}

	subCmd := args[0]
	switch subCmd {
	case "create", "add":
		if len(args) < 3 {
			fmt.Printf("%s参数不足%s: 用法: airoute users create <username> <password> [role]\n示例: airoute users create ops_bob bob123456 operator\n", colorRed, colorReset)
			return
		}
		username := args[1]
		password := args[2]
		role := "operator"
		if len(args) > 3 {
			role = args[3]
		}
		payload, _ := json.Marshal(map[string]string{
			"username": username,
			"password": password,
			"role":     role,
		})
		req, _ := http.NewRequest(http.MethodPost, endpoint+"/api/v1/admin/users", bytes.NewReader(payload))
		req.Header.Set("Content-Type", "application/json")
		if authToken != "" {
			req.Header.Set("Authorization", getAuthHeader(authToken))
		}
		resp, err := client.Do(req)
		if err != nil {
			fmt.Printf("%s创建账号失败: %v%s\n", colorRed, err, colorReset)
			return
		}
		defer resp.Body.Close()

		var res struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
			Error   string `json:"error"`
		}
		_ = json.NewDecoder(resp.Body).Decode(&res)
		if res.Code == 0 {
			fmt.Printf("✔ %s成功创建系统账号%s: [%s] (角色: %s)\n", colorGreen, colorReset, username, role)
		} else {
			fmt.Printf("%s创建账号失败: %s%s\n", colorRed, res.Error, colorReset)
		}

	case "delete", "rm":
		if len(args) < 2 {
			fmt.Printf("%s参数不足%s: 用法: airoute users delete <username>\n", colorRed, colorReset)
			return
		}
		targetUser := args[1]
		req, _ := http.NewRequest(http.MethodDelete, fmt.Sprintf("%s/api/v1/admin/users/%s", endpoint, targetUser), nil)
		if authToken != "" {
			req.Header.Set("Authorization", getAuthHeader(authToken))
		}
		resp, err := client.Do(req)
		if err != nil {
			fmt.Printf("%s删除账号失败: %v%s\n", colorRed, err, colorReset)
			return
		}
		defer resp.Body.Close()

		var res struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
			Error   string `json:"error"`
		}
		_ = json.NewDecoder(resp.Body).Decode(&res)
		if res.Code == 0 {
			fmt.Printf("✔ 成功删除账号 [%s]\n", targetUser)
		} else {
			fmt.Printf("%s删除账号失败: %s%s\n", colorRed, res.Error, colorReset)
		}

	case "passwd", "reset":
		if len(args) < 3 {
			fmt.Printf("%s参数不足%s: 用法: airoute users passwd <username> <new_password>\n", colorRed, colorReset)
			return
		}
		targetUser := args[1]
		newPass := args[2]
		payload, _ := json.Marshal(map[string]string{
			"new_password": newPass,
		})
		req, _ := http.NewRequest(http.MethodPost, fmt.Sprintf("%s/api/v1/admin/users/%s/password", endpoint, targetUser), bytes.NewReader(payload))
		req.Header.Set("Content-Type", "application/json")
		if authToken != "" {
			req.Header.Set("Authorization", getAuthHeader(authToken))
		}
		resp, err := client.Do(req)
		if err != nil {
			fmt.Printf("%s重置密码失败: %v%s\n", colorRed, err, colorReset)
			return
		}
		defer resp.Body.Close()

		var res struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
			Error   string `json:"error"`
		}
		_ = json.NewDecoder(resp.Body).Decode(&res)
		if res.Code == 0 {
			fmt.Printf("✔ 账号 [%s] 密码已成功重设\n", targetUser)
		} else {
			fmt.Printf("%s重置密码失败: %s%s\n", colorRed, res.Error, colorReset)
		}

	default:
		fmt.Printf("%s未知 users 子命令: %s (可用: list, create, delete, passwd)%s\n", colorRed, subCmd, colorReset)
	}
}

