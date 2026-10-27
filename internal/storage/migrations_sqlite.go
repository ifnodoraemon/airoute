package storage

import (
	"database/sql"
	"fmt"
)

func ensureSQLiteColumn(db *sql.DB, table, col, colType string) {
	var count int
	_ = db.QueryRow(fmt.Sprintf("SELECT COUNT(*) FROM pragma_table_info('%s') WHERE name='%s';", table, col)).Scan(&count)
	if count == 0 {
		_, _ = db.Exec(fmt.Sprintf("ALTER TABLE %s ADD COLUMN %s %s;", table, col, colType))
	}
}

// sqliteColumnExists reports whether a column is present on a table.
func sqliteColumnExists(db *sql.DB, table, col string) bool {
	var count int
	_ = db.QueryRow(fmt.Sprintf("SELECT COUNT(*) FROM pragma_table_info('%s') WHERE name='%s';", table, col)).Scan(&count)
	return count > 0
}

// migrate creates required tables if they don't exist for SQLite.
func (db *DB) migrate() error {
	migrations := []Migration{
		{
			Version: 1,
			Name:    "initial_authoritative_schema_v1",
			Up: func(db *DB) error {
				// Handle seamless upgrade from pre-migration legacy databases
				var ulCount int
				_ = db.DB.QueryRow("SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='usage_logs';").Scan(&ulCount)
				if ulCount > 0 {
					ensureSQLiteColumn(db.DB, "usage_logs", "trace_id", "TEXT DEFAULT ''")
					ensureSQLiteColumn(db.DB, "usage_logs", "session_id", "TEXT DEFAULT ''")
					ensureSQLiteColumn(db.DB, "usage_logs", "api_key", "TEXT DEFAULT ''")
					ensureSQLiteColumn(db.DB, "usage_logs", "cached_tokens", "INTEGER DEFAULT 0")
					ensureSQLiteColumn(db.DB, "usage_logs", "cost", "REAL DEFAULT 0")
					ensureSQLiteColumn(db.DB, "usage_logs", "is_off_peak", "INTEGER DEFAULT 0")
					ensureSQLiteColumn(db.DB, "usage_logs", "off_peak_discount", "REAL DEFAULT 1.0")
					ensureSQLiteColumn(db.DB, "usage_logs", "chat_id", "TEXT DEFAULT ''")
				}
				var vkCount int
				_ = db.DB.QueryRow("SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='virtual_keys';").Scan(&vkCount)
				if vkCount > 0 {
					var akCount int
					_ = db.DB.QueryRow("SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='api_keys';").Scan(&akCount)
					if akCount == 0 {
						_, _ = db.DB.Exec("ALTER TABLE virtual_keys RENAME TO api_keys;")
					}
				}
				var akExists int
				_ = db.DB.QueryRow("SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='api_keys';").Scan(&akExists)
				if akExists > 0 {
					ensureSQLiteColumn(db.DB, "api_keys", "used_cost", "REAL DEFAULT 0")
					ensureSQLiteColumn(db.DB, "api_keys", "group_name", "TEXT DEFAULT 'default'")
					ensureSQLiteColumn(db.DB, "api_keys", "user_id", "INTEGER DEFAULT 0")
					ensureSQLiteColumn(db.DB, "api_keys", "format_validation", "TEXT DEFAULT ''")
				}

				schema := `
				CREATE TABLE IF NOT EXISTS channels (
					id INTEGER PRIMARY KEY AUTOINCREMENT,
					name TEXT UNIQUE NOT NULL,
					type TEXT NOT NULL,
					base_url TEXT NOT NULL,
					api_key TEXT NOT NULL,
					models TEXT NOT NULL,
					model_mapping TEXT,
					protocols TEXT,
					priority INTEGER DEFAULT 1,
					weight INTEGER DEFAULT 10,
					timeout_seconds INTEGER DEFAULT 60,
					status TEXT DEFAULT 'active',
					created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
					updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
				);

				CREATE TABLE IF NOT EXISTS api_keys (
					id INTEGER PRIMARY KEY AUTOINCREMENT,
					key TEXT UNIQUE NOT NULL,
					tenant_id TEXT NOT NULL,
					user_id INTEGER DEFAULT 0,
					group_name TEXT DEFAULT 'default',
					allowed_models TEXT,
					rpm INTEGER DEFAULT 60,
					tpm INTEGER DEFAULT 100000,
					budget REAL DEFAULT 0,
					used_tokens INTEGER DEFAULT 0,
					used_cost REAL DEFAULT 0,
					status TEXT DEFAULT 'active',
					format_validation TEXT DEFAULT '',
					created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
					updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
				);

				CREATE TABLE IF NOT EXISTS usage_logs (
					id INTEGER PRIMARY KEY AUTOINCREMENT,
					trace_id TEXT DEFAULT '',
					chat_id TEXT DEFAULT '',
					session_id TEXT DEFAULT '',
					api_key TEXT,
					tenant_id TEXT,
					model TEXT,
					channel TEXT,
					prompt_tokens INTEGER DEFAULT 0,
					completion_tokens INTEGER DEFAULT 0,
					cached_tokens INTEGER DEFAULT 0,
					total_tokens INTEGER DEFAULT 0,
					cost REAL DEFAULT 0,
					is_off_peak INTEGER DEFAULT 0,
					off_peak_discount REAL DEFAULT 1.0,
					duration_ms INTEGER DEFAULT 0,
					ttft_ms INTEGER DEFAULT 0,
					status_code INTEGER DEFAULT 200,
					created_at DATETIME DEFAULT CURRENT_TIMESTAMP
				);

				CREATE TABLE IF NOT EXISTS users (
					id INTEGER PRIMARY KEY AUTOINCREMENT,
					username TEXT UNIQUE NOT NULL,
					email TEXT DEFAULT '',
					password_hash TEXT NOT NULL,
					role TEXT DEFAULT 'admin',
					status TEXT DEFAULT 'active',
					balance REAL DEFAULT 0.0,
					group_name TEXT DEFAULT 'default',
					created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
					updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
				);

				CREATE TABLE IF NOT EXISTS model_fallbacks (
					model TEXT PRIMARY KEY,
					fallback_model TEXT NOT NULL,
					enabled INTEGER DEFAULT 1,
					created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
					updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
				);

				CREATE TABLE IF NOT EXISTS model_prices (
					id INTEGER PRIMARY KEY AUTOINCREMENT,
					model TEXT NOT NULL,
					group_name TEXT DEFAULT 'default',
					prompt_price REAL DEFAULT 0,
					completion_price REAL DEFAULT 0,
					cache_read_price REAL DEFAULT 0,
					fixed_price REAL DEFAULT 0,
					currency TEXT DEFAULT 'CNY',
					off_peak_enabled INTEGER DEFAULT 1,
					off_peak_start TEXT DEFAULT '00:00',
					off_peak_end TEXT DEFAULT '08:30',
					off_peak_discount REAL DEFAULT 0.5,
					off_peak_mode TEXT DEFAULT 'custom',
					off_peak_slots TEXT DEFAULT '',
					weekend_all_day INTEGER DEFAULT 1,
					created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
					updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
					UNIQUE(model, group_name)
				);

				CREATE TABLE IF NOT EXISTS redemption_codes (
					id INTEGER PRIMARY KEY AUTOINCREMENT,
					code TEXT UNIQUE NOT NULL,
					name TEXT DEFAULT '',
					amount REAL DEFAULT 0.0,
					status TEXT DEFAULT 'active',
					used_by TEXT DEFAULT '',
					used_at DATETIME,
					created_at DATETIME DEFAULT CURRENT_TIMESTAMP
				);

				CREATE TABLE IF NOT EXISTS recharge_orders (
					id INTEGER PRIMARY KEY AUTOINCREMENT,
					order_no TEXT UNIQUE NOT NULL,
					username TEXT NOT NULL,
					amount REAL DEFAULT 0.0,
					currency TEXT DEFAULT 'CNY',
					channel TEXT DEFAULT 'stripe',
					stripe_session_id TEXT DEFAULT '',
					status TEXT DEFAULT 'pending',
					created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
					updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
				);

				CREATE TABLE IF NOT EXISTS verification_codes (
					id INTEGER PRIMARY KEY AUTOINCREMENT,
					email TEXT NOT NULL,
					code TEXT NOT NULL,
					purpose TEXT DEFAULT 'register',
					expires_at DATETIME NOT NULL,
					used INTEGER DEFAULT 0,
					attempts INTEGER DEFAULT 0,
					created_at DATETIME DEFAULT CURRENT_TIMESTAMP
				);

				CREATE TABLE IF NOT EXISTS system_skills (
					id TEXT PRIMARY KEY,
					name TEXT NOT NULL,
					description TEXT NOT NULL,
					category TEXT NOT NULL,
					tools TEXT NOT NULL,
					loading_mode TEXT DEFAULT 'lazy',
					manifest TEXT DEFAULT '',
					author TEXT DEFAULT 'Airoute Official',
					version TEXT DEFAULT '1.0.0',
					enabled INTEGER DEFAULT 1,
					created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
					updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
				);

				CREATE TABLE IF NOT EXISTS system_mcp_servers (
					id TEXT PRIMARY KEY,
					name TEXT NOT NULL,
					description TEXT NOT NULL,
					category TEXT NOT NULL,
					transport TEXT NOT NULL,
					endpoint TEXT NOT NULL,
					status TEXT DEFAULT 'online',
					author TEXT DEFAULT 'Airoute Official',
					version TEXT DEFAULT '1.0.0',
					tools TEXT NOT NULL,
					prompts TEXT DEFAULT '[]',
					resources TEXT DEFAULT '[]',
					env_vars TEXT DEFAULT '{}',
					enabled INTEGER DEFAULT 1,
					created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
					updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
				);

				CREATE TABLE IF NOT EXISTS system_settings (
					key TEXT PRIMARY KEY,
					value TEXT NOT NULL,
					updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
				);

				CREATE INDEX IF NOT EXISTS idx_usage_created ON usage_logs(created_at);
				CREATE INDEX IF NOT EXISTS idx_usage_trace ON usage_logs(trace_id);
				CREATE INDEX IF NOT EXISTS idx_usage_chat ON usage_logs(chat_id);
				CREATE INDEX IF NOT EXISTS idx_usage_session ON usage_logs(session_id);
				CREATE INDEX IF NOT EXISTS idx_usage_key ON usage_logs(api_key);
				CREATE INDEX IF NOT EXISTS idx_usage_tenant ON usage_logs(tenant_id);
				CREATE INDEX IF NOT EXISTS idx_usage_model ON usage_logs(model);
				CREATE INDEX IF NOT EXISTS idx_usage_channel ON usage_logs(channel);
				CREATE INDEX IF NOT EXISTS idx_usage_channel_created ON usage_logs(channel, created_at);
				CREATE INDEX IF NOT EXISTS idx_key_user_id ON api_keys(user_id);
				CREATE INDEX IF NOT EXISTS idx_key_tenant_id ON api_keys(tenant_id);
				CREATE INDEX IF NOT EXISTS idx_model_price_group ON model_prices(model, group_name);
				CREATE INDEX IF NOT EXISTS idx_users_email ON users(email);
				CREATE INDEX IF NOT EXISTS idx_redemption_code ON redemption_codes(code);
				CREATE INDEX IF NOT EXISTS idx_recharge_user ON recharge_orders(username);
				CREATE INDEX IF NOT EXISTS idx_recharge_order ON recharge_orders(order_no);
				CREATE INDEX IF NOT EXISTS idx_verify_email ON verification_codes(email, code);
				`
				_, err := db.DB.Exec(schema)
				return err
			},
		},
		{
			Version: 2,
			Name:    "seed_system_defaults",
			Up: func(db *DB) error {
				_, err := db.DB.Exec("INSERT OR IGNORE INTO system_settings (key, value) VALUES ('mcp_enabled', 'true');")
				return err
			},
		},
		{
			Version: 3,
			Name:    "purge_legacy_deepseek_mode",
			Up: func(db *DB) error {
				_, err := db.DB.Exec("UPDATE model_prices SET off_peak_mode = 'custom' WHERE off_peak_mode = 'deepseek' OR off_peak_mode = '';")
				return err
			},
		},
		{
			Version: 4,
			Name:    "rename_virtual_keys_to_api_keys",
			Up: func(db *DB) error {
				var count int
				_ = db.DB.QueryRow("SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='virtual_keys';").Scan(&count)
				if count > 0 {
					_, _ = db.DB.Exec("ALTER TABLE virtual_keys RENAME TO api_keys;")
				}
				_, _ = db.DB.Exec("ALTER TABLE usage_logs RENAME COLUMN virtual_key TO api_key;")
				return nil
			},
		},
		{
			Version: 5,
			Name:    "add_format_validation_to_api_keys",
			Up: func(db *DB) error {
				var colCount int
				_ = db.DB.QueryRow("SELECT COUNT(*) FROM pragma_table_info('api_keys') WHERE name='format_validation';").Scan(&colCount)
				if colCount == 0 {
					_, _ = db.DB.Exec("ALTER TABLE api_keys ADD COLUMN format_validation TEXT DEFAULT '';")
				}
				return nil
			},
		},
		{
			Version: 6,
			Name:    "clean_legacy_orphan_api_keys",
			Up: func(db *DB) error {
				_, _ = db.DB.Exec(`
					UPDATE api_keys 
					SET user_id = COALESCE((SELECT id FROM users WHERE role = 'admin' ORDER BY id ASC LIMIT 1), 1)
					WHERE user_id <= 0 OR user_id NOT IN (SELECT id FROM users);
					UPDATE api_keys SET group_name = 'default' WHERE group_name IS NULL OR group_name = '';
					UPDATE api_keys SET status = 'active' WHERE status IS NULL OR status = '';
					UPDATE api_keys SET format_validation = '' WHERE format_validation IS NULL;
				`)
				return nil
			},
		},
		{
			Version: 7,
			Name:    "add_chat_id_to_usage_logs",
			Up: func(db *DB) error {
				ensureSQLiteColumn(db.DB, "usage_logs", "chat_id", "TEXT DEFAULT ''")
				if _, err := db.DB.Exec(`CREATE INDEX IF NOT EXISTS idx_usage_chat ON usage_logs(chat_id);`); err != nil {
					return err
				}
				return nil
			},
		},
		{
			Version: 8,
			Name:    "create_system_skills_and_mcp_servers",
			Up: func(db *DB) error {
				_, err := db.DB.Exec(`
				CREATE TABLE IF NOT EXISTS system_skills (
					id TEXT PRIMARY KEY,
					name TEXT NOT NULL,
					description TEXT NOT NULL,
					category TEXT NOT NULL,
					tools TEXT NOT NULL,
					loading_mode TEXT DEFAULT 'lazy',
					manifest TEXT DEFAULT '',
					author TEXT DEFAULT 'Airoute Official',
					version TEXT DEFAULT '1.0.0',
					enabled INTEGER DEFAULT 1,
					created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
					updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
				);

				CREATE TABLE IF NOT EXISTS system_mcp_servers (
					id TEXT PRIMARY KEY,
					name TEXT NOT NULL,
					description TEXT NOT NULL,
					category TEXT NOT NULL,
					transport TEXT NOT NULL,
					endpoint TEXT NOT NULL,
					status TEXT DEFAULT 'online',
					author TEXT DEFAULT 'Airoute Official',
					version TEXT DEFAULT '1.0.0',
					tools TEXT NOT NULL,
					prompts TEXT DEFAULT '[]',
					resources TEXT DEFAULT '[]',
					env_vars TEXT DEFAULT '{}',
					enabled INTEGER DEFAULT 1,
					created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
					updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
				);
				`)
				return err
			},
		},
		{
			Version: 9,
			Name:    "add_attempts_to_verification_codes",
			Up: func(db *DB) error {
				ensureSQLiteColumn(db.DB, "verification_codes", "attempts", "INTEGER DEFAULT 0")
				return nil
			},
		},
		{
			Version: 10,
			Name:    "add_usage_logs_channel_indexes",
			Up: func(db *DB) error {
				_, err := db.DB.Exec(`
					CREATE INDEX IF NOT EXISTS idx_usage_channel ON usage_logs(channel);
					CREATE INDEX IF NOT EXISTS idx_usage_channel_created ON usage_logs(channel, created_at);
				`)
				return err
			},
		},
		{
			Version: 11,
			Name:    "add_usage_source_to_usage_logs",
			Up: func(db *DB) error {
				ensureSQLiteColumn(db.DB, "usage_logs", "usage_source", "TEXT DEFAULT 'upstream'")
				return nil
			},
		},
		{
			Version: 12,
			Name:    "add_integer_money_columns",
			Up: func(db *DB) error {
				// Money is mirrored as integer 1e-9 CNY units so that
				// accumulation never drifts. The legacy REAL columns remain the
				// display source while both are written in the same transaction.
				ensureSQLiteColumn(db.DB, "users", "balance_units", "INTEGER DEFAULT 0")
				ensureSQLiteColumn(db.DB, "api_keys", "used_cost_units", "INTEGER DEFAULT 0")
				ensureSQLiteColumn(db.DB, "api_keys", "budget_units", "INTEGER DEFAULT 0")
				ensureSQLiteColumn(db.DB, "usage_logs", "cost_units", "INTEGER DEFAULT 0")
				_, _ = db.DB.Exec(`UPDATE users SET balance_units = CAST(ROUND(balance * 1000000000) AS INTEGER) WHERE balance_units = 0 AND balance <> 0;`)
				_, _ = db.DB.Exec(`UPDATE api_keys SET used_cost_units = CAST(ROUND(used_cost * 1000000000) AS INTEGER), budget_units = CAST(ROUND(budget * 1000000000) AS INTEGER) WHERE used_cost_units = 0 AND used_cost <> 0;`)
				return nil
			},
		},
		{
			Version: 13,
			Name:    "create_request_traces",
			Up: func(db *DB) error {
				_, err := db.DB.Exec(`
					CREATE TABLE IF NOT EXISTS request_traces (
						id INTEGER PRIMARY KEY AUTOINCREMENT,
						trace_id TEXT NOT NULL,
						chat_id TEXT DEFAULT '',
						session_id TEXT DEFAULT '',
						api_key TEXT DEFAULT '',
						tenant_id TEXT DEFAULT '',
						model TEXT DEFAULT '',
						channel TEXT DEFAULT '',
						status_code INTEGER DEFAULT 0,
						error_message TEXT DEFAULT '',
						request_snippet TEXT DEFAULT '',
						response_snippet TEXT DEFAULT '',
						duration_ms INTEGER DEFAULT 0,
						created_at DATETIME DEFAULT CURRENT_TIMESTAMP
					);
					CREATE INDEX IF NOT EXISTS idx_trace_trace_id ON request_traces(trace_id);
					CREATE INDEX IF NOT EXISTS idx_trace_chat_id ON request_traces(chat_id);
					CREATE INDEX IF NOT EXISTS idx_trace_session ON request_traces(session_id);
					CREATE INDEX IF NOT EXISTS idx_trace_created ON request_traces(created_at);
				`)
				return err
			},
		},
		{
			Version: 14,
			Name:    "add_error_message_to_usage_logs",
			Up: func(db *DB) error {
				ensureSQLiteColumn(db.DB, "usage_logs", "error_message", "TEXT DEFAULT ''")
				return nil
			},
		},
		{
			// Forward-only repair for the same defect as the PostgreSQL version 15
			// migration: version 12 was edited in place from *_nano to *_units
			// after it had already been applied, so upgraded databases kept the
			// legacy columns and never received *_units.
			Version: 15,
			Name:    "repair_integer_money_units_columns",
			Up: func(db *DB) error {
				ensureSQLiteColumn(db.DB, "users", "balance_units", "INTEGER DEFAULT 0")
				ensureSQLiteColumn(db.DB, "api_keys", "used_cost_units", "INTEGER DEFAULT 0")
				ensureSQLiteColumn(db.DB, "api_keys", "budget_units", "INTEGER DEFAULT 0")
				ensureSQLiteColumn(db.DB, "usage_logs", "cost_units", "INTEGER DEFAULT 0")

				// Backfill from the legacy *_nano mirrors when present.
				if sqliteColumnExists(db.DB, "users", "balance_nano") {
					_, _ = db.DB.Exec(`UPDATE users SET balance_units = balance_nano WHERE balance_units = 0 AND balance_nano <> 0;`)
				}
				if sqliteColumnExists(db.DB, "api_keys", "used_cost_nano") {
					_, _ = db.DB.Exec(`UPDATE api_keys SET used_cost_units = used_cost_nano WHERE used_cost_units = 0 AND used_cost_nano <> 0;`)
				}
				if sqliteColumnExists(db.DB, "api_keys", "budget_nano") {
					_, _ = db.DB.Exec(`UPDATE api_keys SET budget_units = budget_nano WHERE budget_units = 0 AND budget_nano <> 0;`)
				}
				if sqliteColumnExists(db.DB, "usage_logs", "cost_nano") {
					_, _ = db.DB.Exec(`UPDATE usage_logs SET cost_units = cost_nano WHERE cost_units = 0 AND cost_nano <> 0;`)
				}

				// Remaining legacy rows that predate both mirrors.
				_, _ = db.DB.Exec(`UPDATE users SET balance_units = CAST(ROUND(balance * 1000000000) AS INTEGER) WHERE balance_units = 0 AND balance <> 0;`)
				_, _ = db.DB.Exec(`UPDATE api_keys SET used_cost_units = CAST(ROUND(used_cost * 1000000000) AS INTEGER) WHERE used_cost_units = 0 AND used_cost <> 0;`)
				_, _ = db.DB.Exec(`UPDATE api_keys SET budget_units = CAST(ROUND(budget * 1000000000) AS INTEGER) WHERE budget_units = 0 AND budget <> 0;`)
				return nil
			},
		},
		{Version: 16, Name: "independent_model_routes", Up: createModelRoutesTable},
		{Version: 17, Name: "usage_pricing_snapshot", Up: addUsagePricingSnapshot},
		{Version: 18, Name: "usage_log_query_indexes", Up: addUsageLogQueryIndexes},
		{Version: 19, Name: "usage_log_search_indexes", Up: addUsageLogSearchIndexes},
		{Version: 20, Name: "usage_log_sqlite_fts", Up: addUsageLogFTSIndex},
		{Version: 21, Name: "usage_log_billing_id", Up: addUsageLogBillingID},
		{Version: 22, Name: "usage_log_cost_units_backfill", Up: backfillUsageLogCostUnits},
		{Version: 23, Name: "billing_settlements", Up: addBillingSettlements},
		{Version: 24, Name: "reclassify_client_canceled", Up: reclassifyClientCanceledUsageLogs},
		{Version: 25, Name: "oauth_identities", Up: createOAuthIdentities},
		{Version: 26, Name: "video_tasks", Up: createVideoTasks},
		{Version: 27, Name: "users_email_unique", Up: createUsersEmailUniqueIndex},
		{Version: 28, Name: "operational_indexes", Up: addOperationalIndexes},
		{Version: 29, Name: "api_key_digests", Up: migrateAPIKeysToDigests},
		{Version: 30, Name: "org_governance", Up: addOrgGovernance},
		{Version: 31, Name: "response_contexts", Up: createResponseContexts},
	}
	return db.runMigrations(migrations)
}
