package storage

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/ifnodoraemon/airoute/internal/telemetry"
	_ "github.com/lib/pq"
	_ "modernc.org/sqlite"
)

// DB wraps standard sql.DB connection with dialect-aware query adaptation.
type DB struct {
	*sql.DB
	dialect string // "sqlite" or "postgres"
}

// Dialect returns the active database engine dialect ("sqlite" or "postgres").
func (db *DB) Dialect() string {
	if db == nil || db.dialect == "" {
		return "sqlite"
	}
	return db.dialect
}

// Rebind adapts ANSI SQL queries containing '?' parameters to target engine syntax ($1, $2 for Postgres).
func (db *DB) Rebind(query string) string {
	if db == nil || db.Dialect() != "postgres" {
		return query
	}

	var buf strings.Builder
	argIdx := 1
	inString := false

	for i := 0; i < len(query); i++ {
		c := query[i]
		if c == '\'' {
			inString = !inString
			buf.WriteByte(c)
		} else if c == '?' && !inString {
			buf.WriteString(fmt.Sprintf("$%d", argIdx))
			argIdx++
		} else {
			buf.WriteByte(c)
		}
	}
	s := buf.String()
	s = strings.ReplaceAll(s, "datetime(created_at) >=", "created_at >=")
	s = strings.ReplaceAll(s, "datetime(created_at) <=", "created_at <=")
	s = strings.ReplaceAll(s, " LIKE ", " ILIKE ")
	s = strings.ReplaceAll(s, "INSERT OR IGNORE INTO", "INSERT INTO")
	return s
}

// Exec executes a prepared query using dialect rebind.
func (db *DB) Exec(query string, args ...any) (sql.Result, error) {
	return db.DB.Exec(db.Rebind(query), args...)
}

// Query executes a query returning rows with dialect rebind.
func (db *DB) Query(query string, args ...any) (*sql.Rows, error) {
	return db.DB.Query(db.Rebind(query), args...)
}

// QueryRow executes a query returning a single row with dialect rebind.
func (db *DB) QueryRow(query string, args ...any) *sql.Row {
	return db.DB.QueryRow(db.Rebind(query), args...)
}

// Prepare creates a prepared statement using dialect rebind.
func (db *DB) Prepare(query string) (*sql.Stmt, error) {
	return db.DB.Prepare(db.Rebind(query))
}

// Tx wraps sql.Tx with dialect-aware query adaptation.
type Tx struct {
	*sql.Tx
	db *DB
}

// Exec executes a query within transaction using dialect rebind.
func (tx *Tx) Exec(query string, args ...any) (sql.Result, error) {
	return tx.Tx.Exec(tx.db.Rebind(query), args...)
}

// Prepare creates a prepared statement within transaction using dialect rebind.
func (tx *Tx) Prepare(query string) (*sql.Stmt, error) {
	return tx.Tx.Prepare(tx.db.Rebind(query))
}

// Query executes a query returning rows within transaction using dialect rebind.
func (tx *Tx) Query(query string, args ...any) (*sql.Rows, error) {
	return tx.Tx.Query(tx.db.Rebind(query), args...)
}

// QueryRow executes a query returning a single row within transaction using dialect rebind.
func (tx *Tx) QueryRow(query string, args ...any) *sql.Row {
	return tx.Tx.QueryRow(tx.db.Rebind(query), args...)
}

// Begin begins a new transaction wrapped with dialect rebind support.
func (db *DB) Begin() (*Tx, error) {
	tx, err := db.DB.Begin()
	if err != nil {
		return nil, err
	}
	return &Tx{Tx: tx, db: db}, nil
}

// OpenDB opens either a SQLite or PostgreSQL database depending on dataSourceName.
func OpenDB(dataSourceName string) (*DB, error) {
	if dataSourceName == "" {
		dataSourceName = "data/gateway.db"
	}

	isPostgres := strings.HasPrefix(dataSourceName, "postgres://") ||
		strings.HasPrefix(dataSourceName, "postgresql://") ||
		strings.Contains(dataSourceName, "sslmode=")

	if isPostgres {
		telemetry.Logger.Info("initializing distributed PostgreSQL connection", "source", strings.Split(dataSourceName, "@")[len(strings.Split(dataSourceName, "@"))-1])
		db, err := sql.Open("postgres", dataSourceName)
		if err != nil {
			return nil, fmt.Errorf("open postgres db error: %w", err)
		}

		db.SetMaxOpenConns(50)
		db.SetMaxIdleConns(25)
		db.SetConnMaxLifetime(10 * time.Minute)

		if err := db.Ping(); err != nil {
			telemetry.Logger.Warn("postgres ping failed, checking connection", "error", err.Error())
		}

		wrapper := &DB{DB: db, dialect: "postgres"}
		if err := wrapper.migratePostgres(); err != nil {
			db.Close()
			return nil, fmt.Errorf("migrate postgres db schema error: %w", err)
		}
		telemetry.Logger.Info("PostgreSQL distributed storage initialized successfully")
		return wrapper, nil
	}

	// Local SQLite mode
	if dataSourceName != ":memory:" {
		dir := filepath.Dir(dataSourceName)
		if err := os.MkdirAll(dir, 0755); err != nil {
			return nil, fmt.Errorf("create db directory error: %w", err)
		}
	}

	db, err := sql.Open("sqlite", dataSourceName)
	if err != nil {
		return nil, fmt.Errorf("open sqlite db error: %w", err)
	}

	// Configure connection pool for SQLite with WAL mode for serialized concurrency
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	db.SetConnMaxLifetime(time.Hour)

	if dataSourceName != ":memory:" {
		_, _ = db.Exec("PRAGMA journal_mode=WAL;")
		_, _ = db.Exec("PRAGMA synchronous=NORMAL;")
		_, _ = db.Exec("PRAGMA busy_timeout=5000;")
	}

	wrapper := &DB{DB: db, dialect: "sqlite"}
	if err := wrapper.migrate(); err != nil {
		db.Close()
		return nil, fmt.Errorf("migrate sqlite db schema error: %w", err)
	}

	return wrapper, nil
}

// Migration represents a versioned database schema migration step.
type Migration struct {
	Version int
	Name    string
	Up      func(db *DB) error
}

func (db *DB) runMigrations(migrations []Migration) error {
	var createMigrationsTableSQL string
	if db.Dialect() == "postgres" {
		createMigrationsTableSQL = `CREATE TABLE IF NOT EXISTS schema_migrations (
			version INT PRIMARY KEY,
			name VARCHAR(128) NOT NULL,
			applied_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP
		);`
	} else {
		createMigrationsTableSQL = `CREATE TABLE IF NOT EXISTS schema_migrations (
			version INTEGER PRIMARY KEY,
			name TEXT NOT NULL,
			applied_at DATETIME DEFAULT CURRENT_TIMESTAMP
		);`
	}

	if _, err := db.DB.Exec(createMigrationsTableSQL); err != nil {
		return fmt.Errorf("create schema_migrations table error: %w", err)
	}

	for _, m := range migrations {
		var count int
		var checkSQL string
		if db.Dialect() == "postgres" {
			checkSQL = "SELECT COUNT(*) FROM schema_migrations WHERE version = $1"
		} else {
			checkSQL = "SELECT COUNT(*) FROM schema_migrations WHERE version = ?"
		}
		if err := db.DB.QueryRow(checkSQL, m.Version).Scan(&count); err != nil {
			return fmt.Errorf("check migration version %d error: %w", m.Version, err)
		}
		if count > 0 {
			continue
		}

		telemetry.Logger.Info("applying database migration", "version", m.Version, "name", m.Name, "dialect", db.Dialect())
		if err := m.Up(db); err != nil {
			return fmt.Errorf("migration %d (%s) failed: %w", m.Version, m.Name, err)
		}

		var recordSQL string
		if db.Dialect() == "postgres" {
			recordSQL = "INSERT INTO schema_migrations (version, name) VALUES ($1, $2)"
		} else {
			recordSQL = "INSERT INTO schema_migrations (version, name) VALUES (?, ?)"
		}
		if _, err := db.DB.Exec(recordSQL, m.Version, m.Name); err != nil {
			return fmt.Errorf("record migration %d error: %w", m.Version, err)
		}
	}
	return nil
}

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
					author TEXT DEFAULT 'Nano Official',
					version TEXT DEFAULT '1.0.0',
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
	}
	return db.runMigrations(migrations)
}

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
					author VARCHAR(128) DEFAULT 'Nano Official',
					version VARCHAR(32) DEFAULT '1.0.0',
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
				_, _ = db.DB.Exec(`
				DO $$
				BEGIN
					IF EXISTS (SELECT FROM information_schema.tables WHERE table_name = 'virtual_keys') THEN
						ALTER TABLE virtual_keys RENAME TO api_keys;
					END IF;
					IF EXISTS (SELECT FROM information_schema.columns WHERE table_name = 'usage_logs' AND column_name = 'virtual_key') THEN
						ALTER TABLE usage_logs RENAME COLUMN virtual_key TO api_key;
					END IF;
				END $$;
				`)
				return nil
			},
		},
		{
			Version: 5,
			Name:    "add_format_validation_to_api_keys",
			Up: func(db *DB) error {
				_, _ = db.DB.Exec(`
				DO $$
				BEGIN
					IF NOT EXISTS (SELECT FROM information_schema.columns WHERE table_name = 'api_keys' AND column_name = 'format_validation') THEN
						ALTER TABLE api_keys ADD COLUMN format_validation VARCHAR(32) DEFAULT '';
					END IF;
				END $$;
				`)
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
	}
	return db.runMigrations(migrations)
}
