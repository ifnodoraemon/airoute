package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// handleKeys manages virtual API keys via CLI.
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
