package storage

import (
	"encoding/json"
	"os"
)

// SeedDefaultMCPServers ensures default curated ModelScope-style MCP servers exist.
func (r *Repository) SeedDefaultMCPServers() error {
	defaults := []MCPServerRecord{
		{
			ID:          "airoute-gateway",
			Name:        "Airoute 网关原生核心服务",
			Description: "企业级 AI 网关核心管控与路由服务，暴露集群熔断、模型拓扑、数据脱敏与智能仲裁",
			Category:    "ops",
			Transport:   "sse",
			Endpoint:    "/mcp/sse",
			Status:      "online",
			Author:      "Airoute Official",
			Version:     "1.2.0",
			Tools:       []string{"airoute_cluster_status", "airoute_model_topology", "airoute_data_redact", "airoute_recommend_model", "airoute_query_logs"},
			Prompts:     []string{"cluster_health_report", "route_optimization_guide"},
			Resources:   []string{"airoute://topology/matrix", "airoute://metrics/sli"},
			Enabled:     true,
		},
		{
			ID:          "modelscope-search",
			Name:        "ModelScope 联网检索与正文提取",
			Description: "魔搭社区与开源生态精选多源 Web 检索、动态抓取与结构化 Markdown 提炼",
			Category:    "search",
			Transport:   "sse",
			Endpoint:    "https://mcp.modelscope.cn/servers/search/sse",
			Status:      "online",
			Author:      "ModelScope",
			Version:     "2.1.0",
			Tools:       []string{"airoute_deep_search", "web_fetch_markdown", "academic_paper_search"},
			Prompts:     []string{"deep_research_brief"},
			Resources:   []string{"search://history"},
			Enabled:     true,
		},
		{
			ID:          "github-mcp",
			Name:        "GitHub 研发协作协议服务",
			Description: "仓库代码探查、Pull Request 代码审查、Issue 追踪与 GitHub Actions 工作流联动",
			Category:    "dev",
			Transport:   "stdio",
			Endpoint:    "npx -y @modelcontextprotocol/server-github",
			Status:      "active",
			Author:      "GitHub / Anthropic",
			Version:     "1.0.4",
			Tools:       []string{"search_repositories", "create_issue", "get_file_contents", "create_pull_request"},
			Prompts:     []string{"pull_request_review_summary"},
			Resources:   []string{"github://repos/recent"},
			Enabled:     true,
		},
		{
			ID:          "postgres-mcp",
			Name:        "PostgreSQL 企业数据安全审计服务",
			Description: "企业级关系型数据库 Schema 自动探测、只读隔离查询与慢 SQL 诊断",
			Category:    "db",
			Transport:   "stdio",
			Endpoint: func() string {
				if dbURL := os.Getenv("DATABASE_URL"); dbURL != "" {
					return "npx -y @modelcontextprotocol/server-postgres " + dbURL
				}
				return "npx -y @modelcontextprotocol/server-postgres postgresql://${POSTGRES_USER:-airoute}:${POSTGRES_PASSWORD:-password}@${POSTGRES_HOST:-postgres}:5432/${POSTGRES_DB:-airoute}"
			}(),
			Status:      "active",
			Author:      "ModelContextProtocol",
			Version:     "0.9.2",
			Tools:       []string{"read_query", "list_tables", "describe_table", "airoute_sql_security_check"},
			Prompts:     []string{"explain_slow_query"},
			Resources:   []string{"db://schema/public"},
			Enabled:     true,
		},
		{
			ID:          "browser-fetch-mcp",
			Name:        "Puppeteer 无头浏览器渲染服务",
			Description: "动态 JS 页面无头渲染、网页全景截图与 SPA 应用深度爬取",
			Category:    "search",
			Transport:   "stdio",
			Endpoint:    "npx -y @modelcontextprotocol/server-puppeteer",
			Status:      "active",
			Author:      "Puppeteer Team",
			Version:     "1.0.2",
			Tools:       []string{"puppeteer_navigate", "puppeteer_screenshot", "puppeteer_click", "puppeteer_fill", "puppeteer_evaluate"},
			Prompts:     []string{"capture_page_state"},
			Resources:   []string{"browser://tabs/active"},
			Enabled:     true,
		},
		{
			ID:          "filesystem-mcp",
			Name:        "Filesystem 沙箱文件安全交互",
			Description: "官方受限目录安全文件操作服务，支持安全读写、目录树构建、递归搜索与多文件聚合",
			Category:    "dev",
			Transport:   "stdio",
			Endpoint:    "npx -y @modelcontextprotocol/server-filesystem /workspace",
			Status:      "active",
			Author:      "Anthropic Official",
			Version:     "1.2.1",
			Tools:       []string{"read_file", "read_multiple_files", "write_file", "list_directory", "directory_tree", "search_files"},
			Prompts:     []string{"review_file_diff"},
			Resources:   []string{"file://workspace"},
			Enabled:     true,
		},
		{
			ID:          "fetch-mcp",
			Name:        "Fetch 快速网页提取与 Markdown 转换",
			Description: "官方超轻量网页内容抓取器，零浏览器开销将任意 HTTP/HTTPS 页面转换为紧凑 LLM 结构化 Markdown",
			Category:    "search",
			Transport:   "stdio",
			Endpoint:    "npx -y @modelcontextprotocol/server-fetch",
			Status:      "active",
			Author:      "ModelContextProtocol",
			Version:     "1.0.1",
			Tools:       []string{"fetch_markdown", "fetch_raw"},
			Prompts:     []string{"summarize_webpage"},
			Resources:   []string{"web://recent_fetches"},
			Enabled:     true,
		},
		{
			ID:          "sqlite-mcp",
			Name:        "SQLite 轻量嵌入式分析数据库",
			Description: "官方嵌入式 SQLite 数据库交互服务，支持本地数据快速探索、表结构审计与只读 SQL 执行",
			Category:    "db",
			Transport:   "stdio",
			Endpoint:    "npx -y @modelcontextprotocol/server-sqlite --db-path ./data/gateway.db",
			Status:      "active",
			Author:      "ModelContextProtocol",
			Version:     "1.0.0",
			Tools:       []string{"read_query", "list_tables", "describe_table"},
			Prompts:     []string{"sqlite_schema_analysis"},
			Resources:   []string{"sqlite://schema/main"},
			Enabled:     true,
		},
		{
			ID:          "sentry-mcp",
			Name:        "Sentry 生产异常监控与错误追踪",
			Description: "Sentry 官方错误监控服务，允许智能体直接查询生产链路崩溃、堆栈信息与问题事件定位",
			Category:    "ops",
			Transport:   "stdio",
			Endpoint:    "npx -y @modelcontextprotocol/server-sentry",
			Status:      "standby",
			Author:      "Sentry Official",
			Version:     "1.0.0",
			Tools:       []string{"get_issue", "search_issues", "get_event_stack_trace"},
			Prompts:     []string{"diagnose_issue_root_cause"},
			Resources:   []string{"sentry://issues/unresolved"},
			Enabled:     false,
		},
	}

	for _, s := range defaults {
		toolsJSON, _ := json.Marshal(s.Tools)
		promptsJSON, _ := json.Marshal(s.Prompts)
		resourcesJSON, _ := json.Marshal(s.Resources)
		enabledInt := 0
		if s.Enabled {
			enabledInt = 1
		}
		_, _ = r.db.Exec(`
			INSERT INTO system_mcp_servers (id, name, description, category, transport, endpoint, status, author, version, tools, prompts, resources, env_vars, enabled, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
			ON CONFLICT(id) DO UPDATE SET
				name = excluded.name,
				description = excluded.description,
				category = excluded.category,
				transport = excluded.transport,
				endpoint = excluded.endpoint,
				status = excluded.status,
				author = excluded.author,
				version = excluded.version,
				tools = excluded.tools,
				prompts = excluded.prompts,
				resources = excluded.resources
		`, s.ID, s.Name, s.Description, s.Category, s.Transport, s.Endpoint, s.Status, s.Author, s.Version, string(toolsJSON), string(promptsJSON), string(resourcesJSON), s.EnvVars, enabledInt)
	}
	return nil
}
