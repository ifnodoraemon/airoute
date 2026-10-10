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
	}
	return db.runMigrations(migrations)
}
