package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// handleSkills manages MCP skills via CLI.
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
		mcpPayload := []byte(`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"airoute_search_skills","arguments":{}}}`)
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
