package api

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

// mcpSafeTables is the allowlist of gateway tables exposed by the MCP SQL
// tools. The gateway database also stores upstream provider credentials
// (channels.api_key), issued API keys, user password hashes and redemption
// codes; none of those may be reachable from an agent-facing tool, so this is
// an allowlist rather than a denylist.
var mcpSafeTables = map[string]bool{
	"usage_logs":      true,
	"model_prices":    true,
	"model_fallbacks": true,
}

var (
	mcpSQLIdentRe         = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)
	mcpSQLFromJoinRe      = regexp.MustCompile(`(?i)\b(?:from|join)\s+([A-Za-z_][A-Za-z0-9_."]*)`)
	mcpSQLCTENameRe       = regexp.MustCompile(`(?i)(?:\bwith\b|,)\s+(?:recursive\s+)?([a-z_][a-z0-9_]*)\s*(?:\([^)]*\))?\s+as\s*\(`)
	mcpSQLLineCommentRe   = regexp.MustCompile(`--[^\n]*`)
	mcpSQLBlockCommentRe  = regexp.MustCompile(`(?s)/\*.*?\*/`)
	mcpSQLSensitiveTables = regexp.MustCompile(`(?i)\b(channels|api_keys|users|redemption_codes|verification_codes|recharge_orders|system_skills|system_mcp_servers|system_settings|sqlite_master|sqlite_schema|pg_tables|pg_catalog|information_schema|pg_shadow|pg_user|pg_authid)\b`)
)

// mcpSQLDeniedKeywords are rejected anywhere in a statement, not only at the
// start, so `WITH x AS (DELETE ...)` or a tab/newline separator cannot slip a
// mutating statement past a prefix-only check.
var mcpSQLDeniedKeywordRes = func() []*regexp.Regexp {
	keywords := []string{
		"INSERT", "UPDATE", "DELETE", "DROP", "ALTER", "CREATE", "TRUNCATE",
		"REPLACE", "ATTACH", "DETACH", "PRAGMA", "VACUUM", "REINDEX", "GRANT",
		"REVOKE", "EXEC", "EXECUTE", "CALL", "COPY", "LOAD_EXTENSION",
	}
	res := make([]*regexp.Regexp, 0, len(keywords))
	for _, kw := range keywords {
		res = append(res, regexp.MustCompile(`(?i)\b`+kw+`\b`))
	}
	return res
}()

// callerIsAdmin reports whether the MCP caller is a gateway administrator.
//
// Every admin-gated tool uses this one function. Two separate predicates used to
// answer the same question differently: the infrastructure tools accepted the
// stdio adapter's api_key argument as a fallback credential while the SQL
// reflection tools only read the transport-bound context key, so an
// administrator driving the stdio adapter could call cluster_status but never
// read_query. The order below is deliberate: the caller the transport already
// resolved and authorised wins (it cannot be spoofed by tool arguments), and the
// historical key sources are consulted only for in-process invocations that have
// no HTTP request to carry a credential.
func (h *MCPHandler) callerIsAdmin(ctx context.Context, args map[string]interface{}) bool {
	if h.repo == nil {
		return false
	}
	// Open development mode: no API key has ever been registered, so no tenant
	// boundary exists to protect and the MCP transport admits anonymous callers.
	// Once a key exists the transport requires one, and this gate then requires it
	// to belong to an administrator.
	if h.repo.CountAPIKeys() == 0 {
		return true
	}
	if caller := callerFromContext(ctx); caller != nil {
		if caller.Record != nil {
			return strings.EqualFold(caller.Record.UserRole, "admin")
		}
		return false
	}
	key := ""
	if ctx != nil {
		key, _ = ctx.Value(contextKeyCallerKey).(string)
	}
	if strings.TrimSpace(key) == "" && args != nil {
		key, _ = args["api_key"].(string)
	}
	return h.callerIsAdminForKey(key)
}

// adminOnlyMCPTools are the agent-facing tools whose payload is gateway
// infrastructure rather than tenant data: channel inventory (names, types,
// breaker state, priority, weights) and the model→provider topology. Exposing
// them to every API key let any tenant enumerate the upstream fleet and probe it
// (A16).
var adminOnlyMCPTools = map[string]bool{
	"airoute_cluster_status": true,
	"airoute_model_topology": true,
}

// isAdminOnlyMCPTool reports whether a tool requires gateway-admin authority.
func isAdminOnlyMCPTool(name string) bool {
	return adminOnlyMCPTools[name]
}

// callerIsAdminForKey resolves admin authority for a raw API key, which is how
// out-of-band callers (the stdio adapter, an explicit tool argument) present
// their credential.
func (h *MCPHandler) callerIsAdminForKey(key string) bool {
	if h.repo == nil || strings.TrimSpace(key) == "" {
		return false
	}
	rec, err := h.repo.GetAPIKeyByKey(key)
	if err != nil || rec == nil || strings.TrimSpace(rec.UserRole) == "" {
		return false
	}
	// The transport already joined the owner row; only re-read when the role is
	// missing so the common path costs no extra query.
	if strings.EqualFold(rec.UserRole, "admin") {
		return true
	}
	user, err := h.repo.GetUserByID(rec.UserID)
	if err != nil || user == nil {
		return false
	}
	return strings.EqualFold(user.Role, "admin")
}

// validateMCPReadQuery enforces the read-only, allowlisted subset of SQL the
// MCP tools may execute. It fails closed: anything it cannot prove is a single
// read-only statement over an allowlisted table is rejected.
func validateMCPReadQuery(query string) (string, error) {
	clean := strings.TrimSpace(query)
	if clean == "" {
		return "", fmt.Errorf("parameter 'query' is required")
	}
	if len(clean) > 8192 {
		return "", fmt.Errorf("query is too long")
	}

	// Strip comments before validating so `DELETE/**/FROM` and a newline-hidden
	// `-- \n DROP` cannot evade the keyword scan.
	stripped := mcpSQLBlockCommentRe.ReplaceAllString(clean, " ")
	stripped = mcpSQLLineCommentRe.ReplaceAllString(stripped, " ")
	stripped = strings.TrimSpace(stripped)

	// Exactly one statement. A single trailing semicolon is tolerated.
	statement := strings.TrimSpace(strings.TrimSuffix(stripped, ";"))
	if strings.Contains(statement, ";") {
		return "", fmt.Errorf("multiple SQL statements are not allowed")
	}
	if statement == "" {
		return "", fmt.Errorf("parameter 'query' is required")
	}

	upper := strings.ToUpper(statement)
	readOnlyPrefix := false
	for _, prefix := range []string{"SELECT", "WITH", "EXPLAIN"} {
		if strings.HasPrefix(upper, prefix) {
			readOnlyPrefix = true
			break
		}
	}
	if !readOnlyPrefix {
		return "", fmt.Errorf("only SELECT, WITH and EXPLAIN queries are allowed")
	}

	for _, re := range mcpSQLDeniedKeywordRes {
		if re.MatchString(statement) {
			return "", fmt.Errorf("query contains disallowed keyword")
		}
	}
	if name := mcpSQLSensitiveTables.FindString(statement); name != "" {
		return "", fmt.Errorf("table '%s' is not available to MCP SQL tools", strings.ToLower(name))
	}

	refs := mcpSQLFromJoinRe.FindAllStringSubmatch(statement, -1)
	if len(refs) == 0 {
		return "", fmt.Errorf("query must read from an allowlisted table")
	}
	// Common table expressions defined by the query itself are allowed as read
	// sources, but every table their bodies read is still checked against the
	// allowlist below (and sensitive names are rejected anywhere up front).
	cteNames := map[string]bool{}
	for _, m := range mcpSQLCTENameRe.FindAllStringSubmatch(statement, -1) {
		cteNames[strings.ToLower(m[1])] = true
	}
	for _, m := range refs {
		name := strings.ToLower(strings.Trim(m[1], `".`))
		if idx := strings.LastIndex(name, "."); idx >= 0 {
			name = name[idx+1:]
		}
		if !mcpSafeTables[name] && !cteNames[name] {
			return "", fmt.Errorf("table '%s' is not available to MCP SQL tools", name)
		}
	}

	return clean, nil
}

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
