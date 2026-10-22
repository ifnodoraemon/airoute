package storage

// migratePostgres creates required tables and indexes for PostgreSQL deployments.
func (db *DB) migratePostgres() error {
	migrations := []Migration{
		{
			Version: 1,
			Name:    "initial_authoritative_schema_v1",
			Up: func(db *DB) error {
				schema := `
				CREATE TABLE IF NOT EXISTS channels (
					id BIGSERIAL PRIMARY KEY,
					name VARCHAR(255) UNIQUE NOT NULL,
					type VARCHAR(64) NOT NULL,
					base_url TEXT NOT NULL,
					api_key TEXT NOT NULL,
					models TEXT NOT NULL,
					model_mapping TEXT,
					protocols TEXT,
					priority INT DEFAULT 1,
					weight INT DEFAULT 10,
					timeout_seconds INT DEFAULT 60,
					status VARCHAR(32) DEFAULT 'active',
					created_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP,
					updated_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP
				);

				CREATE TABLE IF NOT EXISTS api_keys (
					id BIGSERIAL PRIMARY KEY,
					key VARCHAR(255) UNIQUE NOT NULL,
					tenant_id VARCHAR(128) NOT NULL,
					user_id BIGINT DEFAULT 0,
					group_name VARCHAR(64) DEFAULT 'default',
					allowed_models TEXT,
					rpm INT DEFAULT 60,
					tpm INT DEFAULT 100000,
					budget DOUBLE PRECISION DEFAULT 0,
					used_tokens BIGINT DEFAULT 0,
					used_cost DOUBLE PRECISION DEFAULT 0,
					status VARCHAR(32) DEFAULT 'active',
					format_validation VARCHAR(32) DEFAULT '',
					created_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP,
					updated_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP
				);

				CREATE TABLE IF NOT EXISTS usage_logs (
					id BIGSERIAL PRIMARY KEY,
					trace_id TEXT DEFAULT '',
					chat_id TEXT DEFAULT '',
					session_id TEXT DEFAULT '',
					api_key VARCHAR(255),
					tenant_id VARCHAR(128),
					model VARCHAR(128),
					channel VARCHAR(128),
					prompt_tokens INT DEFAULT 0,
					completion_tokens INT DEFAULT 0,
					cached_tokens INT DEFAULT 0,
					total_tokens INT DEFAULT 0,
					cost DOUBLE PRECISION DEFAULT 0,
					is_off_peak INT DEFAULT 0,
					off_peak_discount DOUBLE PRECISION DEFAULT 1.0,
					duration_ms BIGINT DEFAULT 0,
					ttft_ms BIGINT DEFAULT 0,
					status_code INT DEFAULT 200,
					created_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP
				);

				CREATE TABLE IF NOT EXISTS users (
					id BIGSERIAL PRIMARY KEY,
					username VARCHAR(128) UNIQUE NOT NULL,
					email VARCHAR(255) DEFAULT '',
					password_hash TEXT NOT NULL,
					role VARCHAR(32) DEFAULT 'admin',
					status VARCHAR(32) DEFAULT 'active',
					balance DOUBLE PRECISION DEFAULT 0.0,
					group_name VARCHAR(64) DEFAULT 'default',
					created_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP,
					updated_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP
				);

				CREATE TABLE IF NOT EXISTS model_fallbacks (
					model VARCHAR(128) PRIMARY KEY,
					fallback_model VARCHAR(128) NOT NULL,
					enabled INT DEFAULT 1,
					created_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP,
					updated_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP
				);

				CREATE TABLE IF NOT EXISTS model_prices (
					id BIGSERIAL PRIMARY KEY,
					model VARCHAR(128) NOT NULL,
					group_name VARCHAR(64) DEFAULT 'default',
					prompt_price DOUBLE PRECISION DEFAULT 0,
					completion_price DOUBLE PRECISION DEFAULT 0,
					cache_read_price DOUBLE PRECISION DEFAULT 0,
					fixed_price DOUBLE PRECISION DEFAULT 0,
					currency VARCHAR(16) DEFAULT 'CNY',
					off_peak_enabled INT DEFAULT 1,
					off_peak_start VARCHAR(32) DEFAULT '00:00',
					off_peak_end VARCHAR(32) DEFAULT '08:30',
					off_peak_discount DOUBLE PRECISION DEFAULT 0.5,
					off_peak_mode VARCHAR(32) DEFAULT 'custom',
					off_peak_slots TEXT DEFAULT '',
					weekend_all_day INT DEFAULT 1,
					created_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP,
					updated_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP,
					UNIQUE(model, group_name)
				);

				CREATE TABLE IF NOT EXISTS redemption_codes (
					id BIGSERIAL PRIMARY KEY,
					code VARCHAR(128) UNIQUE NOT NULL,
					name VARCHAR(255) DEFAULT '',
					amount DOUBLE PRECISION DEFAULT 0.0,
					status VARCHAR(32) DEFAULT 'active',
					used_by VARCHAR(128) DEFAULT '',
					used_at TIMESTAMPTZ,
					created_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP
				);

				CREATE TABLE IF NOT EXISTS recharge_orders (
					id BIGSERIAL PRIMARY KEY,
					order_no VARCHAR(128) UNIQUE NOT NULL,
					username VARCHAR(128) NOT NULL,
					amount DOUBLE PRECISION DEFAULT 0.0,
					currency VARCHAR(16) DEFAULT 'CNY',
					channel VARCHAR(64) DEFAULT 'stripe',
					stripe_session_id VARCHAR(255) DEFAULT '',
					status VARCHAR(32) DEFAULT 'pending',
					created_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP,
					updated_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP
				);

				CREATE TABLE IF NOT EXISTS verification_codes (
					id BIGSERIAL PRIMARY KEY,
					email VARCHAR(255) NOT NULL,
					code VARCHAR(32) NOT NULL,
					purpose VARCHAR(64) DEFAULT 'register',
					expires_at TIMESTAMPTZ NOT NULL,
					used INT DEFAULT 0,
					attempts INT DEFAULT 0,
					created_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP
				);

				CREATE TABLE IF NOT EXISTS system_skills (
					id VARCHAR(128) PRIMARY KEY,
					name VARCHAR(255) NOT NULL,
					description TEXT NOT NULL,
					category VARCHAR(64) NOT NULL,
					tools TEXT NOT NULL,
					loading_mode VARCHAR(32) DEFAULT 'lazy',
					manifest TEXT DEFAULT '',
					author VARCHAR(128) DEFAULT 'Airoute Official',
					version VARCHAR(32) DEFAULT '1.0.0',
					enabled INT DEFAULT 1,
					created_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP,
					updated_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP
				);

				CREATE TABLE IF NOT EXISTS system_mcp_servers (
					id VARCHAR(128) PRIMARY KEY,
					name VARCHAR(255) NOT NULL,
					description TEXT NOT NULL,
					category VARCHAR(64) NOT NULL,
					transport VARCHAR(32) NOT NULL,
					endpoint TEXT NOT NULL,
					status VARCHAR(32) DEFAULT 'online',
					author VARCHAR(128) DEFAULT 'Airoute Official',
					version VARCHAR(32) DEFAULT '1.0.0',
					tools TEXT NOT NULL,
					prompts TEXT DEFAULT '[]',
					resources TEXT DEFAULT '[]',
					env_vars TEXT DEFAULT '{}',
					enabled INT DEFAULT 1,
					created_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP,
					updated_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP
				);

				CREATE TABLE IF NOT EXISTS system_settings (
					key VARCHAR(128) PRIMARY KEY,
					value TEXT NOT NULL,
					updated_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP
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
				_, err := db.DB.Exec("INSERT INTO system_settings (key, value) VALUES ('mcp_enabled', 'true') ON CONFLICT (key) DO NOTHING;")
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
				if _, err := db.DB.Exec(`
				DO $$
				BEGIN
					IF EXISTS (SELECT FROM information_schema.tables WHERE table_name = 'virtual_keys' AND table_schema = current_schema()) THEN
						ALTER TABLE virtual_keys RENAME TO api_keys;
					END IF;
					IF EXISTS (SELECT FROM information_schema.columns WHERE table_name = 'usage_logs' AND column_name = 'virtual_key') THEN
						ALTER TABLE usage_logs RENAME COLUMN virtual_key TO api_key;
					END IF;
				END $$;
				`); err != nil {
					return err
				}
				return nil
			},
		},
		{
			Version: 5,
			Name:    "add_format_validation_to_api_keys",
			Up: func(db *DB) error {
				if _, err := db.DB.Exec(`
				DO $$
				BEGIN
					IF NOT EXISTS (SELECT FROM information_schema.columns WHERE table_name = 'api_keys' AND column_name = 'format_validation') THEN
						ALTER TABLE api_keys ADD COLUMN format_validation VARCHAR(32) DEFAULT '';
					END IF;
				END $$;
				`); err != nil {
					return err
				}
				return nil
			},
		},
		{
			Version: 6,
			Name:    "clean_legacy_orphan_api_keys",
			Up: func(db *DB) error {
				if _, err := db.DB.Exec(`
					UPDATE api_keys 
					SET user_id = COALESCE((SELECT id FROM users WHERE role = 'admin' ORDER BY id ASC LIMIT 1), 1)
					WHERE user_id <= 0 OR user_id NOT IN (SELECT id FROM users);
					UPDATE api_keys SET group_name = 'default' WHERE group_name IS NULL OR group_name = '';
					UPDATE api_keys SET status = 'active' WHERE status IS NULL OR status = '';
					UPDATE api_keys SET format_validation = '' WHERE format_validation IS NULL;
				`); err != nil {
					return err
				}
				return nil
			},
		},
		{
			Version: 7,
			Name:    "add_chat_id_to_usage_logs",
			Up: func(db *DB) error {
				if _, err := db.DB.Exec(`ALTER TABLE usage_logs ADD COLUMN IF NOT EXISTS chat_id TEXT DEFAULT '';`); err != nil {
					return err
				}
				_, err := db.DB.Exec(`CREATE INDEX IF NOT EXISTS idx_usage_chat ON usage_logs(chat_id);`)
				return err
			},
		},
		{
			Version: 8,
			Name:    "create_system_skills_and_mcp_servers",
			Up: func(db *DB) error {
				_, err := db.DB.Exec(`
				CREATE TABLE IF NOT EXISTS system_skills (
					id VARCHAR(128) PRIMARY KEY,
					name VARCHAR(255) NOT NULL,
					description TEXT NOT NULL,
					category VARCHAR(64) NOT NULL,
					tools TEXT NOT NULL,
					loading_mode VARCHAR(32) DEFAULT 'lazy',
					manifest TEXT DEFAULT '',
					author VARCHAR(128) DEFAULT 'Airoute Official',
					version VARCHAR(32) DEFAULT '1.0.0',
					enabled INT DEFAULT 1,
					created_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP,
					updated_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP
				);

				CREATE TABLE IF NOT EXISTS system_mcp_servers (
					id VARCHAR(128) PRIMARY KEY,
					name VARCHAR(255) NOT NULL,
					description TEXT NOT NULL,
					category VARCHAR(64) NOT NULL,
					transport VARCHAR(32) NOT NULL,
					endpoint TEXT NOT NULL,
					status VARCHAR(32) DEFAULT 'online',
					author VARCHAR(128) DEFAULT 'Airoute Official',
					version VARCHAR(32) DEFAULT '1.0.0',
					tools TEXT NOT NULL,
					prompts TEXT DEFAULT '[]',
					resources TEXT DEFAULT '[]',
					env_vars TEXT DEFAULT '{}',
					enabled INT DEFAULT 1,
					created_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP,
					updated_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP
				);
				`)
				return err
			},
		},
		{
			Version: 9,
			Name:    "add_attempts_to_verification_codes",
			Up: func(db *DB) error {
				if _, err := db.DB.Exec("ALTER TABLE verification_codes ADD COLUMN IF NOT EXISTS attempts INT DEFAULT 0;"); err != nil {
					return err
				}
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
				_, err := db.DB.Exec("ALTER TABLE usage_logs ADD COLUMN IF NOT EXISTS usage_source VARCHAR(16) DEFAULT 'upstream';")
				return err
			},
		},
		{
			Version: 12,
			Name:    "add_integer_money_columns",
			Up: func(db *DB) error {
				stmts := []string{
					"ALTER TABLE users ADD COLUMN IF NOT EXISTS balance_units BIGINT DEFAULT 0;",
					"ALTER TABLE api_keys ADD COLUMN IF NOT EXISTS used_cost_units BIGINT DEFAULT 0;",
					"ALTER TABLE api_keys ADD COLUMN IF NOT EXISTS budget_units BIGINT DEFAULT 0;",
					"ALTER TABLE usage_logs ADD COLUMN IF NOT EXISTS cost_units BIGINT DEFAULT 0;",
					"UPDATE users SET balance_units = ROUND(balance * 1000000000)::BIGINT WHERE balance_units = 0 AND balance <> 0;",
					"UPDATE api_keys SET used_cost_units = ROUND(used_cost * 1000000000)::BIGINT, budget_units = ROUND(budget * 1000000000)::BIGINT WHERE used_cost_units = 0 AND used_cost <> 0;",
				}
				for _, stmt := range stmts {
					if _, err := db.DB.Exec(stmt); err != nil {
						return err
					}
				}
				return nil
			},
		},
		{
			Version: 13,
			Name:    "create_request_traces",
			Up: func(db *DB) error {
				_, err := db.DB.Exec(`
					CREATE TABLE IF NOT EXISTS request_traces (
						id BIGSERIAL PRIMARY KEY,
						trace_id TEXT NOT NULL,
						chat_id TEXT DEFAULT '',
						session_id TEXT DEFAULT '',
						api_key TEXT DEFAULT '',
						tenant_id TEXT DEFAULT '',
						model TEXT DEFAULT '',
						channel TEXT DEFAULT '',
						status_code INT DEFAULT 0,
						error_message TEXT DEFAULT '',
						request_snippet TEXT DEFAULT '',
						response_snippet TEXT DEFAULT '',
						duration_ms BIGINT DEFAULT 0,
						created_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP
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
				_, err := db.DB.Exec("ALTER TABLE usage_logs ADD COLUMN IF NOT EXISTS error_message TEXT DEFAULT '';")
				return err
			},
		},
		{
			// Version 12 originally created the *_nano money columns and was
			// applied to existing deployments. It was later edited in place to
			// create *_units instead, so databases that had already recorded
			// version 12 kept the *_nano columns and never received *_units —
			// every api_keys read and usage_logs insert then failed with
			// `column "budget_units" does not exist`.
			//
			// This migration is the forward-only repair: it guarantees the
			// *_units columns exist and backfills them from the legacy *_nano
			// columns when those are still present. It is idempotent and safe on
			// fresh databases (which already have *_units from version 12).
			Version: 15,
			Name:    "repair_integer_money_units_columns",
			Up: func(db *DB) error {
				stmts := []string{
					"ALTER TABLE users ADD COLUMN IF NOT EXISTS balance_units BIGINT DEFAULT 0;",
					"ALTER TABLE api_keys ADD COLUMN IF NOT EXISTS used_cost_units BIGINT DEFAULT 0;",
					"ALTER TABLE api_keys ADD COLUMN IF NOT EXISTS budget_units BIGINT DEFAULT 0;",
					"ALTER TABLE usage_logs ADD COLUMN IF NOT EXISTS cost_units BIGINT DEFAULT 0;",
					// Backfill from the legacy columns only when they exist. The
					// float columns are the authoritative display source and are
					// mirrored here so ledger reads stop returning zero.
					`DO $$
					BEGIN
						IF EXISTS (SELECT 1 FROM information_schema.columns
						           WHERE table_schema = current_schema()
						             AND table_name = 'users' AND column_name = 'balance_nano') THEN
							UPDATE users SET balance_units = balance_nano
							 WHERE balance_units = 0 AND balance_nano <> 0;
						END IF;

						IF EXISTS (SELECT 1 FROM information_schema.columns
						           WHERE table_schema = current_schema()
						             AND table_name = 'api_keys' AND column_name = 'used_cost_nano') THEN
							UPDATE api_keys
							   SET used_cost_units = used_cost_nano
							 WHERE used_cost_units = 0 AND used_cost_nano <> 0;
						END IF;

						IF EXISTS (SELECT 1 FROM information_schema.columns
						           WHERE table_schema = current_schema()
						             AND table_name = 'api_keys' AND column_name = 'budget_nano') THEN
							UPDATE api_keys SET budget_units = budget_nano
							 WHERE budget_units = 0 AND budget_nano <> 0;
						END IF;

						IF EXISTS (SELECT 1 FROM information_schema.columns
						           WHERE table_schema = current_schema()
						             AND table_name = 'usage_logs' AND column_name = 'cost_nano') THEN
							UPDATE usage_logs SET cost_units = cost_nano
							 WHERE cost_units = 0 AND cost_nano <> 0;
						END IF;
					END $$;`,
					// Anything still at zero but with a non-zero float value is
					// legacy data that predates both mirrors.
					"UPDATE users SET balance_units = ROUND(balance * 1000000000)::BIGINT WHERE balance_units = 0 AND balance <> 0;",
					"UPDATE api_keys SET used_cost_units = ROUND(used_cost * 1000000000)::BIGINT WHERE used_cost_units = 0 AND used_cost <> 0;",
					"UPDATE api_keys SET budget_units = ROUND(budget * 1000000000)::BIGINT WHERE budget_units = 0 AND budget <> 0;",
				}
				for _, stmt := range stmts {
					if _, err := db.DB.Exec(stmt); err != nil {
						return err
					}
				}
				return nil
			},
		},
		{Version: 16, Name: "independent_model_routes", Up: createModelRoutesTable},
		{Version: 17, Name: "usage_pricing_snapshot", Up: addUsagePricingSnapshot},
	}
	return db.runMigrations(migrations)
}
