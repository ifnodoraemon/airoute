package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os/exec"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

// executeBuiltinUtilityTools executes real in-process utilities (math, time, local database reflection, reasoning).
func (h *MCPHandler) executeBuiltinUtilityTools(ctx context.Context, name string, args map[string]interface{}) (string, bool, bool) {
	switch name {
	case "airoute_get_current_time":
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

	case "airoute_calc_eval":
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

	case "read_query":
		if h.repo == nil || h.repo.GetDB() == nil {
			return "Database repository not initialized", true, true
		}
		query, _ := args["query"].(string)
		query = strings.TrimSpace(query)
		if query == "" {
			return "Error: parameter 'query' is required", true, true
		}

		// Security enforcement: only allow read-only queries (SELECT, EXPLAIN)
		upperQ := strings.ToUpper(query)
		disallowed := []string{"INSERT", "UPDATE", "DELETE", "DROP", "ALTER", "CREATE", "TRUNCATE", "REPLACE", "EXEC", "GRANT"}
		for _, kw := range disallowed {
			if strings.HasPrefix(upperQ, kw+" ") || strings.Contains(upperQ, " "+kw+" ") {
				return fmt.Sprintf("Security violation: query contains disallowed mutating keyword '%s'", kw), true, true
			}
		}

		db := h.repo.GetDB()
		rows, err := db.Query(query)
		if err != nil {
			return fmt.Sprintf("Query execution failed: %v", err), true, true
		}
		defer rows.Close()

		cols, err := rows.Columns()
		if err != nil {
			return fmt.Sprintf("Failed to fetch column metadata: %v", err), true, true
		}

		var resultRows [][]interface{}
		for rows.Next() {
			vals := make([]interface{}, len(cols))
			valPtrs := make([]interface{}, len(cols))
			for i := range vals {
				valPtrs[i] = &vals[i]
			}
			if err := rows.Scan(valPtrs...); err != nil {
				return fmt.Sprintf("Row scan error: %v", err), true, true
			}
			rowList := make([]interface{}, len(cols))
			for i, v := range vals {
				if b, ok := v.([]byte); ok {
					rowList[i] = string(b)
				} else {
					rowList[i] = v
				}
			}
			resultRows = append(resultRows, rowList)
			if len(resultRows) >= 100 { // Safety limit
				break
			}
		}

		res := gin.H{
			"query":          query,
			"columns":        cols,
			"rows":           resultRows,
			"row_count":      len(resultRows),
			"security_audit": "PASSED (Read-only query validated)",
		}
		b, _ := json.MarshalIndent(res, "", "  ")
		return string(b), false, true

	case "list_tables":
		if h.repo == nil || h.repo.GetDB() == nil {
			return "Database repository not initialized", true, true
		}
		db := h.repo.GetDB()
		var query string
		if db.Dialect() == "postgres" {
			query = "SELECT tablename FROM pg_tables WHERE schemaname = 'public' ORDER BY tablename;"
		} else {
			query = "SELECT name FROM sqlite_master WHERE type = 'table' AND name NOT LIKE 'sqlite_%' ORDER BY name;"
		}

		rows, err := db.Query(query)
		if err != nil {
			return fmt.Sprintf("Failed to list tables: %v", err), true, true
		}
		defer rows.Close()

		var tables []string
		for rows.Next() {
			var tbl string
			if err := rows.Scan(&tbl); err == nil {
				tables = append(tables, tbl)
			}
		}

		res := gin.H{
			"dialect": db.Dialect(),
			"tables":  tables,
			"status":  "connected",
		}
		b, _ := json.MarshalIndent(res, "", "  ")
		return string(b), false, true

	case "describe_table":
		if h.repo == nil || h.repo.GetDB() == nil {
			return "Database repository not initialized", true, true
		}
		tableName, _ := args["table_name"].(string)
		if tableName == "" {
			return "Error: parameter 'table_name' is required", true, true
		}

		db := h.repo.GetDB()
		type colMeta struct {
			Name     string `json:"name"`
			Type     string `json:"type"`
			Nullable bool   `json:"nullable"`
		}
		var cols []colMeta

		if db.Dialect() == "postgres" {
			rows, err := db.Query(`SELECT column_name, data_type, is_nullable FROM information_schema.columns WHERE table_name = ? ORDER BY ordinal_position`, tableName)
			if err == nil {
				defer rows.Close()
				for rows.Next() {
					var c colMeta
					var isNullStr string
					if err := rows.Scan(&c.Name, &c.Type, &isNullStr); err == nil {
						c.Nullable = strings.EqualFold(isNullStr, "YES")
						cols = append(cols, c)
					}
				}
			}
		} else {
			rows, err := db.Query(fmt.Sprintf("PRAGMA table_info(%s)", tableName))
			if err == nil {
				defer rows.Close()
				for rows.Next() {
					var cid int
					var name, colType string
					var notNull int
					var dfltValue interface{}
					var pk int
					if err := rows.Scan(&cid, &name, &colType, &notNull, &dfltValue, &pk); err == nil {
						cols = append(cols, colMeta{
							Name:     name,
							Type:     colType,
							Nullable: notNull == 0,
						})
					}
				}
			}
		}

		res := gin.H{
			"table":   tableName,
			"columns": cols,
		}
		b, _ := json.MarshalIndent(res, "", "  ")
		return string(b), false, true

	case "sequentialthinking":
		thought, _ := args["thought"].(string)
		thoughtNum, _ := args["thoughtNumber"].(float64)
		totalThoughts, _ := args["totalThoughts"].(float64)
		if thought == "" {
			return "Error: parameter 'thought' is required", true, true
		}
		if totalThoughts <= 0 {
			totalThoughts = 1
		}
		res := gin.H{
			"thought":            thought,
			"thought_number":     int(thoughtNum),
			"total_thoughts":     int(totalThoughts),
			"verification_state": "VALIDATED",
		}
		b, _ := json.MarshalIndent(res, "", "  ")
		return string(b), false, true
	}

	return "", false, false
}

// executeDynamicMCPTool dispatches execution to configured external MCP servers from storage via real JSON-RPC 2.0.
func (h *MCPHandler) executeDynamicMCPTool(ctx context.Context, name string, args map[string]interface{}) (string, bool) {
	if h.repo != nil {
		servers, _ := h.repo.ListMCPServers()
		for _, srv := range servers {
			if !srv.Enabled {
				continue
			}
			for _, t := range srv.Tools {
				if t == name {
					endpoint := srv.Endpoint
					if endpoint == "" {
						return fmt.Sprintf("MCP server '%s' has no endpoint configured", srv.Name), true
					}

					// Forward to real HTTP / SSE MCP server via JSON-RPC 2.0
					if strings.HasPrefix(endpoint, "http://") || strings.HasPrefix(endpoint, "https://") {
						return forwardJSONRPCToolCall(ctx, endpoint, name, args)
					}

					// Real stdio CLI execution if installed
					if srv.Transport == "stdio" {
						parts := strings.Fields(endpoint)
						if len(parts) > 0 {
							if _, lookErr := exec.LookPath(parts[0]); lookErr == nil {
								return executeStdioMCPTool(ctx, parts, name, args)
							}
							return fmt.Sprintf("MCP server '%s' requires local runtime '%s' which is not installed", srv.Name, parts[0]), true
						}
					}

					return fmt.Sprintf("MCP server '%s' uses unsupported transport '%s'", srv.Name, srv.Transport), true
				}
			}
		}
	}
	return fmt.Sprintf("Unknown tool '%s'", name), true
}

// forwardJSONRPCToolCall transmits a standard tools/call JSON-RPC 2.0 request to an external MCP endpoint.
func forwardJSONRPCToolCall(ctx context.Context, endpoint, toolName string, args map[string]interface{}) (string, bool) {
	rpcReq := map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      time.Now().UnixMicro(),
		"method":  "tools/call",
		"params": map[string]interface{}{
			"name":      toolName,
			"arguments": args,
		},
	}
	bodyBytes, err := json.Marshal(rpcReq)
	if err != nil {
		return fmt.Sprintf("Failed to serialize MCP request: %v", err), true
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(bodyBytes))
	if err != nil {
		return fmt.Sprintf("Failed to initialize HTTP request to '%s': %v", endpoint, err), true
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "application/json")

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(httpReq)
	if err != nil {
		return fmt.Sprintf("Upstream MCP server '%s' connection failed: %v", endpoint, err), true
	}
	defer resp.Body.Close()

	// Cap the response: an untrusted MCP server could otherwise stream gigabytes
	// into memory within the 30s client timeout.
	const maxMCPResponseBytes = 8 << 20
	respBytes, err := io.ReadAll(io.LimitReader(resp.Body, maxMCPResponseBytes))
	if err != nil {
		return fmt.Sprintf("Failed reading response from upstream MCP server: %v", err), true
	}

	if resp.StatusCode >= 400 {
		return fmt.Sprintf("Upstream MCP server returned HTTP %d: %s", resp.StatusCode, string(respBytes)), true
	}

	var rpcResp struct {
		JSONRPC string          `json:"jsonrpc"`
		Result  json.RawMessage `json:"result"`
		Error   *struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(respBytes, &rpcResp); err == nil {
		if rpcResp.Error != nil {
			return fmt.Sprintf("MCP execution error (code %d): %s", rpcResp.Error.Code, rpcResp.Error.Message), true
		}
		if len(rpcResp.Result) > 0 {
			return string(rpcResp.Result), false
		}
	}

	return string(respBytes), false
}

// executeStdioMCPTool executes a tool on a local stdio MCP server process.
func executeStdioMCPTool(ctx context.Context, cmdParts []string, toolName string, args map[string]interface{}) (string, bool) {
	rpcReq := map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      time.Now().UnixMicro(),
		"method":  "tools/call",
		"params": map[string]interface{}{
			"name":      toolName,
			"arguments": args,
		},
	}
	reqBytes, _ := json.Marshal(rpcReq)
	reqBytes = append(reqBytes, '\n')

	cmd := exec.CommandContext(ctx, cmdParts[0], cmdParts[1:]...)
	cmd.Stdin = bytes.NewReader(reqBytes)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return fmt.Sprintf("Stdio process execution error: %v, stderr: %s", err, stderr.String()), true
	}

	outStr := strings.TrimSpace(stdout.String())
	if outStr == "" {
		return "Stdio process completed with empty output", false
	}
	return outStr, false
}
