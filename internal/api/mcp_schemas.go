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

// getBuiltinMCPTools returns the declarative tool schemas supported natively by Airoute gateway.
func getBuiltinMCPTools() []gin.H {
	return []gin.H{
		{
			"name":        "airoute_search_skills",
			"description": "Progressive Stage 1 (Search & Discovery): Search available Agent skills by keywords or category to find relevant capabilities without context bloat",
			"inputSchema": gin.H{
				"type": "object",
				"properties": gin.H{
					"query":    prop("string", "Keyword to search across skill names, descriptions, or tool names (e.g. '治理', '脱敏', 'SQL', 'search')"),
					"category": prop("string", "Optional category filter: ops, search, security, dev, prompt, agent"),
				},
			},
		},
		{
			"name":        "airoute_inspect_skill",
			"description": "Progressive Stage 2 (Confirmation & Inspection): Inspect a single skill's triggers, prerequisites, and tool signatures before loading full instructions",
			"inputSchema": gin.H{
				"type": "object",
				"properties": gin.H{
					"skill_id": prop("string", "Skill identifier, e.g. gateway_ops, deep_research, security_compliance, sql_code_guard, prompt_optimizer, model_arbiter"),
				},
				"required": []string{"skill_id"},
			},
		},
		{
			"name":        "airoute_get_skill_manifest",
			"description": "Progressive Stage 3 (Full Manifest): Pull the complete SKILL.md specification with operational procedures, full schemas, and examples on-demand",
			"inputSchema": gin.H{
				"type": "object",
				"properties": gin.H{
					"skill_id": prop("string", "Skill identifier, e.g. gateway_ops, deep_research, security_compliance, sql_code_guard, prompt_optimizer, model_arbiter"),
				},
				"required": []string{"skill_id"},
			},
		},
		{
			"name":        "airoute_cluster_status",
			"description": "Check real-time gateway cluster health, SLA metrics, channel circuit breakers, and upstream availability",
			"inputSchema": gin.H{
				"type":       "object",
				"properties": gin.H{},
			},
		},
		{
			"name":        "airoute_model_topology",
			"description": "Inspect unified model routing topology, multi-channel failover hierarchy, and modality matrix",
			"inputSchema": gin.H{
				"type": "object",
				"properties": gin.H{
					"modality": prop("string", "Optional modality filter: chat, images, audio_speech, audio_transcription, videos, embeddings, rerank"),
				},
			},
		},
		{
			"name":        "airoute_deep_search",
			"description": "Perform multi-source real-time web deep search and structured knowledge extraction with citations",
			"inputSchema": gin.H{
				"type": "object",
				"properties": gin.H{
					"query":       prop("string", "Search query or research question to search across the web"),
					"max_results": prop("integer", "Max number of citations to retrieve (default 5)"),
				},
				"required": []string{"query"},
			},
		},
		{
			"name":        "airoute_data_redact",
			"description": "Detect and mask sensitive data (mobile phones, national ID, bank cards, API keys, emails, secrets) for enterprise compliance",
			"inputSchema": gin.H{
				"type": "object",
				"properties": gin.H{
					"text":      prop("string", "Input text to scan and redact"),
					"mask_char": prop("string", "Masking character, default '*'"),
				},
				"required": []string{"text"},
			},
		},
		{
			"name":        "airoute_sql_security_check",
			"description": "Audit SQL statements for security risks, full-table scans, missing WHERE clauses, and injection vulnerabilities",
			"inputSchema": gin.H{
				"type": "object",
				"properties": gin.H{
					"sql": prop("string", "SQL statement to audit and analyze"),
				},
				"required": []string{"sql"},
			},
		},
		{
			"name":        "airoute_optimize_prompt",
			"description": "Engineer and reconstruct raw prompts into structured, battle-tested system prompts with few-shot constraints",
			"inputSchema": gin.H{
				"type": "object",
				"properties": gin.H{
					"prompt":    prop("string", "Original raw or conversational prompt to optimize"),
					"task_type": prop("string", "Optional task category: coding, analysis, creative, roleplay, extractor"),
				},
				"required": []string{"prompt"},
			},
		},
		{
			"name":        "airoute_recommend_model",
			"description": "Intelligently recommend optimal models and routing channel based on task nature, latency, cost, and modality",
			"inputSchema": gin.H{
				"type": "object",
				"properties": gin.H{
					"task_description": prop("string", "Detailed description of the AI task to be performed"),
					"priority":         prop("string", "Optimization priority: quality (deep reasoning), speed (low latency), cost (budget friendly)"),
				},
				"required": []string{"task_description"},
			},
		},
		{
			"name":        "airoute_query_logs",
			"description": "Query request audit logs, token consumption, and session history from Airoute",
			"inputSchema": gin.H{
				"type": "object",
				"properties": gin.H{
					"session_id": prop("string", "Filter by conversation session ID"),
					"limit":      prop("integer", "Max number of logs to return (default 10, max 50)"),
				},
			},
		},
		{
			"name":        "airoute_chat",
			"description": "Execute an LLM chat completion through Airoute with automatic zero-touch session affinity and multi-provider load balancing",
			"inputSchema": gin.H{
				"type": "object",
				"properties": gin.H{
					"model":      prop("string", "Model identifier, e.g. deepseek-chat, gpt-4o, claude-3-5-sonnet"),
					"message":    prop("string", "User message / prompt"),
					"session_id": prop("string", "Optional session ID for multi-turn conversational memory and prefix cache affinity"),
				},
				"required": []string{"model", "message"},
			},
		},
		{
			"name":        "puppeteer_navigate",
			"description": "Navigate and render web page using headless Puppeteer sandbox browser",
			"inputSchema": gin.H{
				"type": "object",
				"properties": gin.H{
					"url": prop("string", "Target web page URL to load"),
				},
				"required": []string{"url"},
			},
		},
		{
			"name":        "puppeteer_screenshot",
			"description": "Capture full-viewport or element screenshot via Puppeteer",
			"inputSchema": gin.H{
				"type": "object",
				"properties": gin.H{
					"name": prop("string", "Screenshot name or image identifier"),
				},
			},
		},
		{
			"name":        "puppeteer_click",
			"description": "Trigger simulated DOM element click in Puppeteer headless browser",
			"inputSchema": gin.H{
				"type": "object",
				"properties": gin.H{
					"selector": prop("string", "CSS selector to click"),
				},
				"required": []string{"selector"},
			},
		},
		{
			"name":        "puppeteer_evaluate",
			"description": "Evaluate JavaScript expression safely inside page context",
			"inputSchema": gin.H{
				"type": "object",
				"properties": gin.H{
					"script": prop("string", "JavaScript code snippet or expression to evaluate"),
				},
				"required": []string{"script"},
			},
		},
		{
			"name":        "read_query",
			"description": "Execute a read-only SQL SELECT/EXPLAIN query over a small allowlisted set of non-sensitive gateway tables (usage_logs, model_prices, model_fallbacks). Administrator API key required; mutating statements, sensitive tables and multiple statements are rejected.",
			"inputSchema": gin.H{
				"type": "object",
				"properties": gin.H{
					"query": prop("string", "Single read-only SELECT/WITH/EXPLAIN statement over an allowlisted table"),
				},
				"required": []string{"query"},
			},
		},
		{
			"name":        "list_tables",
			"description": "Enumerate the non-sensitive gateway tables exposed to MCP SQL tools (administrator API key required)",
			"inputSchema": gin.H{
				"type":       "object",
				"properties": gin.H{},
			},
		},
		{
			"name":        "describe_table",
			"description": "Inspect the column schema of an allowlisted, non-sensitive gateway table (administrator API key required)",
			"inputSchema": gin.H{
				"type": "object",
				"properties": gin.H{
					"table_name": prop("string", "Allowlisted table name (usage_logs, model_prices, model_fallbacks)"),
				},
				"required": []string{"table_name"},
			},
		},
		{
			"name":        "sequentialthinking",
			"description": "Deep step-by-step reasoning and dynamic reflective thinking with hypothesis verification",
			"inputSchema": gin.H{
				"type": "object",
				"properties": gin.H{
					"thought":       prop("string", "Current thinking step or analysis"),
					"thoughtNumber": prop("integer", "Current thought index (1-based)"),
					"totalThoughts": prop("integer", "Estimated total thoughts"),
				},
				"required": []string{"thought"},
			},
		},
		{
			"name":        "search_repositories",
			"description": "Search GitHub repositories by query, stars, or language",
			"inputSchema": gin.H{
				"type": "object",
				"properties": gin.H{
					"query": prop("string", "Search query keywords"),
				},
				"required": []string{"query"},
			},
		},
		{
			"name":        "create_issue",
			"description": "Create a new issue in a GitHub repository",
			"inputSchema": gin.H{
				"type": "object",
				"properties": gin.H{
					"repo":  prop("string", "Repository identifier (owner/repo)"),
					"title": prop("string", "Issue title"),
					"body":  prop("string", "Issue markdown body"),
				},
				"required": []string{"repo", "title"},
			},
		},
		{
			"name":        "get_file_contents",
			"description": "Retrieve source code or document contents from a GitHub repository",
			"inputSchema": gin.H{
				"type": "object",
				"properties": gin.H{
					"repo": prop("string", "Repository identifier (owner/repo)"),
					"path": prop("string", "File path in repo"),
				},
				"required": []string{"repo", "path"},
			},
		},
		{
			"name":        "create_pull_request",
			"description": "Create a pull request for branch changes in a GitHub repository",
			"inputSchema": gin.H{
				"type": "object",
				"properties": gin.H{
					"repo":  prop("string", "Repository identifier (owner/repo)"),
					"title": prop("string", "Pull request title"),
					"head":  prop("string", "Head branch"),
					"base":  prop("string", "Base branch"),
				},
				"required": []string{"repo", "title", "head", "base"},
			},
		},
	}
}
