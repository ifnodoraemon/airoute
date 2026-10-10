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

// InsertGetID executes an INSERT query and returns the newly generated auto-increment ID
// in a dialect-agnostic way (RETURNING id on Postgres, LastInsertId on SQLite).
func (db *DB) InsertGetID(query string, args ...any) (int64, error) {
	if db.Dialect() == "postgres" {
		queryWithReturning := query + " RETURNING id"
		var id int64
		err := db.QueryRow(queryWithReturning, args...).Scan(&id)
		return id, err
	}
	res, err := db.Exec(query, args...)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// InsertGetID executes an INSERT query within a transaction and returns the newly generated auto-increment ID.
func (tx *Tx) InsertGetID(query string, args ...any) (int64, error) {
	if tx.db != nil && tx.db.Dialect() == "postgres" {
		queryWithReturning := query + " RETURNING id"
		var id int64
		err := tx.QueryRow(queryWithReturning, args...).Scan(&id)
		return id, err
	}
	res, err := tx.Exec(query, args...)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
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


