package api

import (
	"context"
	"fmt"
	"regexp"
	"strings"
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
		"REVOKE", "EXEC", "EXECUTE", "CALL", "COPY", "LOAD_EXTENSION", "INTO",
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
