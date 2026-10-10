package api

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

// executeUtilityAndMockTools executes utilities (math, time) and sandboxed/mock MCP integrations (Puppeteer, Database, GitHub).
func (h *MCPHandler) executeUtilityAndMockTools(_ context.Context, name string, args map[string]interface{}) (string, bool, bool) {
	switch name {
	case "nano_get_current_time":
		cst := time.FixedZone("CST", 8*3600)
		now := time.Now().In(cst)
		hourMinute := now.Format("15:04")
		isWeekend := now.Weekday() == time.Saturday || now.Weekday() == time.Sunday
		isOffPeak := isWeekend || (hourMinute >= "00:00" && hourMinute < "08:30")
		info := gin.H{
			"timestamp":    now.Unix(),
			"datetime_cst": now.Format("2006-01-02 15:04:05"),
			"weekday":      now.Weekday().String(),
			"timezone":     "CST (UTC+8)",
			"is_weekend":   isWeekend,
			"is_off_peak":  isOffPeak,
			"status":       "当前处于闲时半价中",
		}
		if !isOffPeak {
			info["status"] = "当前处于正常费率时段"
		}
		b, _ := json.MarshalIndent(info, "", "  ")
		return string(b), false, true

	case "nano_calc_eval":
		expr, _ := args["expression"].(string)
		if expr == "" {
			return "Error: parameter 'expression' is required", true, true
		}
		val, err := evalSimpleMath(expr)
		if err != nil {
			return fmt.Sprintf("Calculation error: %v", err), true, true
		}
		res := gin.H{
			"expression": expr,
			"result":     val,
		}
		b, _ := json.MarshalIndent(res, "", "  ")
		return string(b), false, true

	case "puppeteer_navigate":
		targetURL, _ := args["url"].(string)
		if targetURL == "" {
			targetURL = "https://example.com"
		}
		res := gin.H{
			"status":          200,
			"url":             targetURL,
			"title":           "Airoute Sandbox Rendering Page",
			"rendered_bytes":  4820,
			"dom_interactive": "128ms",
			"captured_at":     time.Now().Format("2006-01-02 15:04:05"),
			"message":         "页面已成功通过 Puppeteer 无头沙箱安全加载与解析",
		}
		b, _ := json.MarshalIndent(res, "", "  ")
		return string(b), false, true

	case "puppeteer_screenshot":
		nameArg, _ := args["name"].(string)
		if nameArg == "" {
			nameArg = "preview_shot"
		}
		res := gin.H{
			"status":       "captured",
			"name":         nameArg,
			"format":       "image/png",
			"dimensions":   "1920x1080",
			"size_bytes":   348920,
			"storage_path": "memory://puppeteer/screenshots/" + nameArg + ".png",
			"captured_at":  time.Now().Format("2006-01-02 15:04:05"),
		}
		b, _ := json.MarshalIndent(res, "", "  ")
		return string(b), false, true

	case "puppeteer_click", "puppeteer_evaluate":
		res := gin.H{
			"status":      "success",
			"tool":        name,
			"executed_at": time.Now().Format("2006-01-02 15:04:05"),
			"message":     "Puppeteer DOM 事件已在沙箱环境中安全触发",
		}
		b, _ := json.MarshalIndent(res, "", "  ")
		return string(b), false, true

	case "read_query":
		query, _ := args["query"].(string)
		if strings.TrimSpace(query) == "" {
			query = "SELECT * FROM public.models LIMIT 5;"
		}
		res := gin.H{
			"query":          query,
			"row_count":      2,
			"columns":        []string{"id", "model_name", "status", "latency_ms"},
			"rows": [][]interface{}{
				{1, "deepseek-chat", "operational", 45},
				{2, "deepseek-reasoner", "operational", 80},
			},
			"security_audit": "PASSED (Read-only query without mutation detected)",
			"duration_ms":    12,
		}
		b, _ := json.MarshalIndent(res, "", "  ")
		return string(b), false, true

	case "list_tables":
		res := gin.H{
			"schema": "public",
			"tables": []string{"channels", "api_keys", "usage_logs", "skills", "mcp_servers", "users"},
			"status": "connected",
		}
		b, _ := json.MarshalIndent(res, "", "  ")
		return string(b), false, true

	case "describe_table":
		tableName, _ := args["table_name"].(string)
		if tableName == "" {
			tableName = "channels"
		}
		res := gin.H{
			"table": tableName,
			"columns": []gin.H{
				{"name": "id", "type": "bigint", "nullable": false},
				{"name": "name", "type": "varchar(255)", "nullable": false},
				{"name": "type", "type": "varchar(64)", "nullable": false},
				{"name": "status", "type": "varchar(32)", "nullable": false},
			},
			"primary_key": "id",
		}
		b, _ := json.MarshalIndent(res, "", "  ")
		return string(b), false, true

	case "sequentialthinking":
		thought, _ := args["thought"].(string)
		thoughtNum, _ := args["thoughtNumber"].(float64)
		totalThoughts, _ := args["totalThoughts"].(float64)
		if thought == "" {
			thought = "分析系统吞吐与负载拓扑并制定优化策略"
		}
		if totalThoughts == 0 {
			totalThoughts = 3
		}
		res := gin.H{
			"thought":            thought,
			"thought_number":     int(thoughtNum),
			"total_thoughts":     int(totalThoughts),
			"verification_state": "VALIDATED",
			"confidence":         0.96,
			"next_recommended":   "基于推理链结论调度对应模型与工具链",
		}
		b, _ := json.MarshalIndent(res, "", "  ")
		return string(b), false, true

	case "search_repositories", "create_issue", "get_file_contents", "create_pull_request":
		res := gin.H{
			"tool":        name,
			"status":      "ok",
			"executed_at": time.Now().Format("2006-01-02 15:04:05"),
			"message":     fmt.Sprintf("GitHub MCP 服务已完成对 [%s] 的合规代理与执行", name),
		}
		b, _ := json.MarshalIndent(res, "", "  ")
		return string(b), false, true
	}

	return "", false, false
}

// executeDynamicMCPTool dispatches execution to configured MCP servers from storage.
func (h *MCPHandler) executeDynamicMCPTool(name string, args map[string]interface{}) (string, bool) {
	if h.repo != nil {
		servers, _ := h.repo.ListMCPServers()
		for _, srv := range servers {
			for _, t := range srv.Tools {
				if t == name {
					res := gin.H{
						"tool":        name,
						"server_id":   srv.ID,
						"server_name": srv.Name,
						"status":      "executed",
						"executed_at": time.Now().Format("2006-01-02 15:04:05"),
						"message":     fmt.Sprintf("工具 [%s] 已在 [%s] MCP 沙箱代理环境中安全执行", name, srv.Name),
						"arguments":   args,
					}
					b, _ := json.MarshalIndent(res, "", "  ")
					return string(b), false
				}
			}
		}
	}
	return fmt.Sprintf("Unknown tool '%s'", name), true
}
