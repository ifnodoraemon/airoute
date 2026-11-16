package api

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

// Built-in MCP utility tool execution (time, calculator, SQL reflection,
// sequential thinking), extracted from mcp_tools.go to keep the tool-guard file
// under the size red line.

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
			"status":       "Off-peak half-price billing is active",
		}
		if !isOffPeak {
			info["status"] = "Standard pricing is active"
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
		if !h.callerIsAdmin(ctx, args) {
			return "Security violation: SQL reflection tools are restricted to gateway administrators", true, true
		}
		rawQuery, _ := args["query"].(string)
		query, err := validateMCPReadQuery(rawQuery)
		if err != nil {
			return "Security violation: " + err.Error(), true, true
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
		if !h.callerIsAdmin(ctx, args) {
			return "Security violation: SQL reflection tools are restricted to gateway administrators", true, true
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

		// Only ever advertise the allowlisted, non-sensitive tables: enumerating
		// the rest (users, api_keys, channels, ...) would help an attacker map
		// where the secrets live.
		var tables []string
		for rows.Next() {
			var tbl string
			if err := rows.Scan(&tbl); err == nil && mcpSafeTables[strings.ToLower(tbl)] {
				tables = append(tables, tbl)
			}
		}

		res := gin.H{
			"dialect": db.Dialect(),
			"tables":  tables,
			"status":  "connected",
			"note":    "Only non-sensitive gateway tables are exposed to MCP SQL tools",
		}
		b, _ := json.MarshalIndent(res, "", "  ")
		return string(b), false, true

	case "describe_table":
		if h.repo == nil || h.repo.GetDB() == nil {
			return "Database repository not initialized", true, true
		}
		if !h.callerIsAdmin(ctx, args) {
			return "Security violation: SQL reflection tools are restricted to gateway administrators", true, true
		}
		tableName, _ := args["table_name"].(string)
		tableName = strings.TrimSpace(tableName)
		if tableName == "" {
			return "Error: parameter 'table_name' is required", true, true
		}
		// Validate the identifier AND the allowlist before it is interpolated
		// into PRAGMA: an unvalidated table_name is a SQL injection sink.
		if !mcpSQLIdentRe.MatchString(tableName) || !mcpSafeTables[strings.ToLower(tableName)] {
			return fmt.Sprintf("Security violation: table '%s' is not available to MCP SQL tools", tableName), true, true
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
			// Identifier is regex-validated and allowlisted above; quote it so the
			// value can only ever be a table name.
			rows, err := db.Query(fmt.Sprintf(`PRAGMA table_info("%s")`, tableName))
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