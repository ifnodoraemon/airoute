package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// handleUsers manages system user accounts and roles via CLI.
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
