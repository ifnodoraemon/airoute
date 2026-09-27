package storage

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/ifnodoraemon/nano-gateway/internal/telemetry"
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

// migrate creates required tables if they don't exist.
func (db *DB) migrate() error {
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

	CREATE TABLE IF NOT EXISTS virtual_keys (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		key TEXT UNIQUE NOT NULL,
		tenant_id TEXT NOT NULL,
		allowed_models TEXT,
		rpm INTEGER DEFAULT 60,
		tpm INTEGER DEFAULT 100000,
		budget REAL DEFAULT 0,
		used_tokens INTEGER DEFAULT 0,
		used_cost REAL DEFAULT 0,
		status TEXT DEFAULT 'active',
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);

	CREATE TABLE IF NOT EXISTS usage_logs (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		trace_id TEXT DEFAULT '',
		chat_id TEXT DEFAULT '',
		session_id TEXT DEFAULT '',
		virtual_key TEXT,
		tenant_id TEXT,
		model TEXT,
		channel TEXT,
		prompt_tokens INTEGER DEFAULT 0,
		completion_tokens INTEGER DEFAULT 0,
		cached_tokens INTEGER DEFAULT 0,
		total_tokens INTEGER DEFAULT 0,
		cost REAL DEFAULT 0,
		duration_ms INTEGER DEFAULT 0,
		ttft_ms INTEGER DEFAULT 0,
		status_code INTEGER DEFAULT 200,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);

	CREATE TABLE IF NOT EXISTS users (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		username TEXT UNIQUE NOT NULL,
		password_hash TEXT NOT NULL,
		role TEXT DEFAULT 'admin',
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
		model TEXT UNIQUE NOT NULL,
		prompt_price REAL DEFAULT 0,
		completion_price REAL DEFAULT 0,
		cache_read_price REAL DEFAULT 0,
		fixed_price REAL DEFAULT 0,
		currency TEXT DEFAULT 'CNY',
		off_peak_enabled INTEGER DEFAULT 1,
		off_peak_start TEXT DEFAULT '00:00',
		off_peak_end TEXT DEFAULT '08:30',
		off_peak_discount REAL DEFAULT 0.5,
		off_peak_mode TEXT DEFAULT 'deepseek',
		off_peak_slots TEXT DEFAULT '',
		weekend_all_day INTEGER DEFAULT 1,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
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
	`
	if _, err := db.Exec(schema); err != nil {
		return err
	}

	// Idempotent migrations for existing deployments
	_, _ = db.Exec("ALTER TABLE channels ADD COLUMN protocols TEXT;")
	_, _ = db.Exec("ALTER TABLE usage_logs ADD COLUMN trace_id TEXT DEFAULT '';")
	_, _ = db.Exec("CREATE INDEX IF NOT EXISTS idx_usage_trace ON usage_logs(trace_id);")
	_, _ = db.Exec("UPDATE usage_logs SET trace_id = chat_id WHERE (trace_id = '' OR trace_id IS NULL) AND chat_id != '';")
	_, _ = db.Exec("ALTER TABLE usage_logs ADD COLUMN chat_id TEXT DEFAULT '';")
	_, _ = db.Exec("CREATE INDEX IF NOT EXISTS idx_usage_chat ON usage_logs(chat_id);")
	_, _ = db.Exec("UPDATE usage_logs SET chat_id = session_id WHERE (chat_id = '' OR chat_id IS NULL) AND session_id != '';")
	_, _ = db.Exec("ALTER TABLE usage_logs ADD COLUMN session_id TEXT DEFAULT '';")
	_, _ = db.Exec("CREATE INDEX IF NOT EXISTS idx_usage_session ON usage_logs(session_id);")
	_, _ = db.Exec("ALTER TABLE usage_logs ADD COLUMN cached_tokens INTEGER DEFAULT 0;")
	_, _ = db.Exec("ALTER TABLE usage_logs ADD COLUMN cost REAL DEFAULT 0;")
	_, _ = db.Exec("ALTER TABLE usage_logs ADD COLUMN is_off_peak INTEGER DEFAULT 0;")
	_, _ = db.Exec("ALTER TABLE usage_logs ADD COLUMN off_peak_discount REAL DEFAULT 1.0;")
	_, _ = db.Exec("ALTER TABLE virtual_keys ADD COLUMN used_cost REAL DEFAULT 0;")
	_, _ = db.Exec("ALTER TABLE model_prices ADD COLUMN off_peak_enabled INTEGER DEFAULT 1;")
	_, _ = db.Exec("ALTER TABLE model_prices ADD COLUMN off_peak_start TEXT DEFAULT '00:00';")
	_, _ = db.Exec("ALTER TABLE model_prices ADD COLUMN off_peak_end TEXT DEFAULT '08:30';")
	_, _ = db.Exec("ALTER TABLE model_prices ADD COLUMN off_peak_discount REAL DEFAULT 0.5;")
	_, _ = db.Exec("ALTER TABLE model_prices ADD COLUMN off_peak_mode TEXT DEFAULT 'deepseek';")
	_, _ = db.Exec("ALTER TABLE model_prices ADD COLUMN off_peak_slots TEXT DEFAULT '';")
	_, _ = db.Exec("ALTER TABLE model_prices ADD COLUMN weekend_all_day INTEGER DEFAULT 1;")
	_, _ = db.Exec("ALTER TABLE users ADD COLUMN role TEXT DEFAULT 'admin';")
	_, _ = db.Exec("ALTER TABLE users ADD COLUMN created_at DATETIME DEFAULT CURRENT_TIMESTAMP;")
	_, _ = db.Exec("ALTER TABLE users ADD COLUMN updated_at DATETIME DEFAULT CURRENT_TIMESTAMP;")
	_, _ = db.Exec("ALTER TABLE system_skills ADD COLUMN loading_mode TEXT DEFAULT 'lazy';")
	_, _ = db.Exec("ALTER TABLE system_skills ADD COLUMN manifest TEXT DEFAULT '';")
	_, _ = db.Exec("ALTER TABLE system_skills ADD COLUMN author TEXT DEFAULT 'Nano Official';")
	_, _ = db.Exec("ALTER TABLE system_skills ADD COLUMN version TEXT DEFAULT '1.0.0';")
	_, _ = db.Exec("INSERT OR IGNORE INTO system_settings (key, value) VALUES ('mcp_enabled', 'true');")
	return nil
}

// migratePostgres creates required tables and indexes for PostgreSQL deployments.
func (db *DB) migratePostgres() error {
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

	CREATE TABLE IF NOT EXISTS virtual_keys (
		id BIGSERIAL PRIMARY KEY,
		key VARCHAR(255) UNIQUE NOT NULL,
		tenant_id VARCHAR(128) NOT NULL,
		allowed_models TEXT,
		rpm INT DEFAULT 60,
		tpm INT DEFAULT 100000,
		budget DOUBLE PRECISION DEFAULT 0,
		used_tokens BIGINT DEFAULT 0,
		used_cost DOUBLE PRECISION DEFAULT 0,
		status VARCHAR(32) DEFAULT 'active',
		created_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP,
		updated_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP
	);

	CREATE TABLE IF NOT EXISTS usage_logs (
		id BIGSERIAL PRIMARY KEY,
		trace_id TEXT DEFAULT '',
		chat_id TEXT DEFAULT '',
		session_id TEXT DEFAULT '',
		virtual_key VARCHAR(255),
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
		password_hash TEXT NOT NULL,
		role VARCHAR(32) DEFAULT 'admin',
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
		model VARCHAR(128) UNIQUE NOT NULL,
		prompt_price DOUBLE PRECISION DEFAULT 0,
		completion_price DOUBLE PRECISION DEFAULT 0,
		cache_read_price DOUBLE PRECISION DEFAULT 0,
		fixed_price DOUBLE PRECISION DEFAULT 0,
		currency VARCHAR(16) DEFAULT 'CNY',
		off_peak_enabled INT DEFAULT 1,
		off_peak_start VARCHAR(32) DEFAULT '00:00',
		off_peak_end VARCHAR(32) DEFAULT '08:30',
		off_peak_discount DOUBLE PRECISION DEFAULT 0.5,
		off_peak_mode VARCHAR(32) DEFAULT 'deepseek',
		off_peak_slots TEXT DEFAULT '',
		weekend_all_day INT DEFAULT 1,
		created_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP,
		updated_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP
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
	CREATE INDEX IF NOT EXISTS idx_usage_chat ON usage_logs(chat_id);
	CREATE INDEX IF NOT EXISTS idx_usage_session ON usage_logs(session_id);
	`
	if _, err := db.DB.Exec(schema); err != nil {
		return err
	}

	_, _ = db.DB.Exec("INSERT INTO system_settings (key, value) VALUES ('mcp_enabled', 'true') ON CONFLICT (key) DO NOTHING;")
	return nil
}
