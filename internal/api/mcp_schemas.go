package api

import (
	"github.com/gin-gonic/gin"
)

// prop is the one-field JSON-schema property shape the declarative tool
// schemas below use. Keeping it in a helper stops the 38-property table from
// repeating the same two keys and drifting apart.
func prop(typ, description string) gin.H {
	return gin.H{"type": typ, "description": description}
}

// tool is one declarative MCP tool: a name, a description and a JSON-schema
// object built from the property helpers above. required lists the property
// names that must be present (nil when the tool takes no arguments).
func tool(name, description string, properties gin.H, required []string) gin.H {
	schema := gin.H{"type": "object", "properties": properties}
	if len(required) > 0 {
		schema["required"] = required
	}
	return gin.H{"name": name, "description": description, "inputSchema": schema}
}

// getBuiltinMCPTools returns the declarative tool schemas supported natively by Airoute gateway.
func getBuiltinMCPTools() []gin.H {
	return []gin.H{
		tool("airoute_search_skills",
			"Progressive Stage 1 (Search & Discovery): Search available Agent skills by keywords or category to find relevant capabilities without context bloat",
			gin.H{
				"query":    prop("string", "Keyword to search across skill names, descriptions, or tool names (e.g. '治理', '脱敏', 'SQL', 'search')"),
				"category": prop("string", "Optional category filter: ops, search, security, dev, prompt, agent"),
			},
			nil),
		tool("airoute_inspect_skill",
			"Progressive Stage 2 (Confirmation & Inspection): Inspect a single skill's triggers, prerequisites, and tool signatures before loading full instructions",
			gin.H{
				"skill_id": prop("string", "Skill identifier, e.g. gateway_ops, deep_research, security_compliance, sql_code_guard, prompt_optimizer, model_arbiter"),
			},
			[]string{"skill_id"}),
		tool("airoute_get_skill_manifest",
			"Progressive Stage 3 (Full Manifest): Pull the complete SKILL.md specification with operational procedures, full schemas, and examples on-demand",
			gin.H{
				"skill_id": prop("string", "Skill identifier, e.g. gateway_ops, deep_research, security_compliance, sql_code_guard, prompt_optimizer, model_arbiter"),
			},
			[]string{"skill_id"}),
		tool("airoute_cluster_status",
			"Check real-time gateway cluster health, SLA metrics, channel circuit breakers, and upstream availability",
			gin.H{},
			nil),
		tool("airoute_model_topology",
			"Inspect unified model routing topology, multi-channel failover hierarchy, and modality matrix",
			gin.H{
				"modality": prop("string", "Optional modality filter: chat, images, audio_speech, audio_transcription, videos, embeddings, rerank"),
			},
			nil),
		tool("airoute_deep_search",
			"Perform multi-source real-time web deep search and structured knowledge extraction with citations",
			gin.H{
				"query":       prop("string", "Search query or research question to search across the web"),
				"max_results": prop("integer", "Max number of citations to retrieve (default 5)"),
			},
			[]string{"query"}),
		tool("airoute_data_redact",
			"Detect and mask sensitive data (mobile phones, national ID, bank cards, API keys, emails, secrets) for enterprise compliance",
			gin.H{
				"text":      prop("string", "Input text to scan and redact"),
				"mask_char": prop("string", "Masking character, default '*'"),
			},
			[]string{"text"}),
		tool("airoute_sql_security_check",
			"Audit SQL statements for security risks, full-table scans, missing WHERE clauses, and injection vulnerabilities",
			gin.H{
				"sql": prop("string", "SQL statement to audit and analyze"),
			},
			[]string{"sql"}),
		tool("airoute_optimize_prompt",
			"Engineer and reconstruct raw prompts into structured, battle-tested system prompts with few-shot constraints",
			gin.H{
				"prompt":    prop("string", "Original raw or conversational prompt to optimize"),
				"task_type": prop("string", "Optional task category: coding, analysis, creative, roleplay, extractor"),
			},
			[]string{"prompt"}),
		tool("airoute_recommend_model",
			"Intelligently recommend optimal models and routing channel based on task nature, latency, cost, and modality",
			gin.H{
				"task_description": prop("string", "Detailed description of the AI task to be performed"),
				"priority":         prop("string", "Optimization priority: quality (deep reasoning), speed (low latency), cost (budget friendly)"),
			},
			[]string{"task_description"}),
		tool("airoute_query_logs",
			"Query request audit logs, token consumption, and session history from Airoute",
			gin.H{
				"session_id": prop("string", "Filter by conversation session ID"),
				"limit":      prop("integer", "Max number of logs to return (default 10, max 50)"),
			},
			nil),
		tool("airoute_chat",
			"Execute an LLM chat completion through Airoute with automatic zero-touch session affinity and multi-provider load balancing",
			gin.H{
				"model":      prop("string", "Model identifier, e.g. deepseek-chat, gpt-4o, claude-3-5-sonnet"),
				"message":    prop("string", "User message / prompt"),
				"session_id": prop("string", "Optional session ID for multi-turn conversational memory and prefix cache affinity"),
			},
			[]string{"model", "message"}),
		tool("puppeteer_navigate",
			"Navigate and render web page using headless Puppeteer sandbox browser",
			gin.H{
				"url": prop("string", "Target web page URL to load"),
			},
			[]string{"url"}),
		tool("puppeteer_screenshot",
			"Capture full-viewport or element screenshot via Puppeteer",
			gin.H{
				"name": prop("string", "Screenshot name or image identifier"),
			},
			nil),
		tool("puppeteer_click",
			"Trigger simulated DOM element click in Puppeteer headless browser",
			gin.H{
				"selector": prop("string", "CSS selector to click"),
			},
			[]string{"selector"}),
		tool("puppeteer_evaluate",
			"Evaluate JavaScript expression safely inside page context",
			gin.H{
				"script": prop("string", "JavaScript code snippet or expression to evaluate"),
			},
			[]string{"script"}),
		tool("read_query",
			"Execute a read-only SQL SELECT/EXPLAIN query over a small allowlisted set of non-sensitive gateway tables (usage_logs, model_prices, model_fallbacks). Administrator API key required; mutating statements, sensitive tables and multiple statements are rejected.",
			gin.H{
				"query": prop("string", "Single read-only SELECT/WITH/EXPLAIN statement over an allowlisted table"),
			},
			[]string{"query"}),
		tool("list_tables",
			"Enumerate the non-sensitive gateway tables exposed to MCP SQL tools (administrator API key required)",
			gin.H{},
			nil),
		tool("describe_table",
			"Inspect the column schema of an allowlisted, non-sensitive gateway table (administrator API key required)",
			gin.H{
				"table_name": prop("string", "Allowlisted table name (usage_logs, model_prices, model_fallbacks)"),
			},
			[]string{"table_name"}),
		tool("sequentialthinking",
			"Deep step-by-step reasoning and dynamic reflective thinking with hypothesis verification",
			gin.H{
				"thought":       prop("string", "Current thinking step or analysis"),
				"thoughtNumber": prop("integer", "Current thought index (1-based)"),
				"totalThoughts": prop("integer", "Estimated total thoughts"),
			},
			[]string{"thought"}),
		tool("search_repositories",
			"Search GitHub repositories by query, stars, or language",
			gin.H{
				"query": prop("string", "Search query keywords"),
			},
			[]string{"query"}),
		tool("create_issue",
			"Create a new issue in a GitHub repository",
			gin.H{
				"repo":  prop("string", "Repository identifier (owner/repo)"),
				"title": prop("string", "Issue title"),
				"body":  prop("string", "Issue markdown body"),
			},
			[]string{"repo", "title"}),
		tool("get_file_contents",
			"Retrieve source code or document contents from a GitHub repository",
			gin.H{
				"repo": prop("string", "Repository identifier (owner/repo)"),
				"path": prop("string", "File path in repo"),
			},
			[]string{"repo", "path"}),
		tool("create_pull_request",
			"Create a pull request for branch changes in a GitHub repository",
			gin.H{
				"repo":  prop("string", "Repository identifier (owner/repo)"),
				"title": prop("string", "Pull request title"),
				"head":  prop("string", "Head branch"),
				"base":  prop("string", "Base branch"),
			},
			[]string{"repo", "title", "head", "base"}),
	}
}
