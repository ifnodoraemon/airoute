package storage

import (
	"context"
	"database/sql"
	"fmt"
	"net/url"
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
	// PostgreSQL is case-insensitive only with ILIKE. Rewrite the LIKE operator
	// explicitly (never inside string literals, which the scanner above already
	// left untouched because they contain no '?' placeholders).
	s = rewriteLikeToILike(s)
	if strings.Contains(s, "INSERT OR IGNORE INTO") {
		s = strings.ReplaceAll(s, "INSERT OR IGNORE INTO", "INSERT INTO")
		if !strings.Contains(strings.ToUpper(s), "ON CONFLICT") {
			s = s + " ON CONFLICT DO NOTHING"
		}
	}
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

// withReturningID appends "RETURNING id" to a statement, tolerating a trailing
// semicolon and skipping statements that already ask for a returning clause.
func withReturningID(query string) string {
	trimmed := strings.TrimRight(strings.TrimSpace(query), ";")
	if strings.Contains(strings.ToUpper(trimmed), " RETURNING ") {
		return trimmed
	}
	return trimmed + " RETURNING id"
}

// InsertGetID executes an INSERT query and returns the newly generated auto-increment ID
// in a dialect-agnostic way (RETURNING id on Postgres, LastInsertId on SQLite).
func (db *DB) InsertGetID(query string, args ...any) (int64, error) {
	if db.Dialect() == "postgres" {
		queryWithReturning := withReturningID(query)
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
		queryWithReturning := withReturningID(query)
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

// BeginTx begins a transaction bound to ctx. Callers that must not block
// indefinitely (background writers) should use this so a stalled database
// releases the goroutine instead of pinning it until the process restarts.
func (db *DB) BeginTx(ctx context.Context) (*Tx, error) {
	tx, err := db.DB.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	return &Tx{Tx: tx, db: db}, nil
}

// PostgresStatementTimeoutMS bounds any single server-side statement.
//
// The repository layer is mostly context-free (a full ExecContext/QueryContext
// migration is a much larger refactor), so a runaway query on the control plane
// would otherwise pin a pooled connection indefinitely. Setting the limit in the
// connection string makes the *server* abort it, which covers every call site
// including ones that never had a deadline.
const PostgresStatementTimeoutMS = 30000

// ApplyPostgresStatementTimeouts injects statement_timeout (and an idle
// transaction timeout) into a libpq DSN unless the operator already configured
// `options` themselves. Both the URL form and the keyword/value form are
// supported; anything unrecognised is returned unchanged.
func ApplyPostgresStatementTimeouts(dsn string) string {
	if strings.TrimSpace(dsn) == "" {
		return dsn
	}
	opts := fmt.Sprintf("-c statement_timeout=%d -c idle_in_transaction_session_timeout=%d",
		PostgresStatementTimeoutMS, PostgresStatementTimeoutMS)

	lower := strings.ToLower(dsn)
	if strings.HasPrefix(lower, "postgres://") || strings.HasPrefix(lower, "postgresql://") {
		u, err := url.Parse(dsn)
		if err != nil {
			return dsn
		}
		q := u.Query()
		if q.Get("options") != "" {
			return dsn // operator-provided options win
		}
		q.Set("options", opts)
		u.RawQuery = q.Encode()
		return u.String()
	}

	// Keyword/value DSN: host=... dbname=... [options='...']
	if strings.Contains(lower, "options=") {
		return dsn
	}
	return dsn + " options='" + opts + "'"
}

// OpenDB opens either a SQLite or PostgreSQL database depending on dataSourceName.
func OpenDB(dataSourceName string) (*DB, error) {
	if dataSourceName == "" {
		dataSourceName = "data/gateway.db"
	}

	// Detect PostgreSQL from the URL form or a libpq keyword/value DSN
	// (host=... dbname=...). A path that merely contains "sslmode=" used to be
	// misdetected as PostgreSQL and vice versa, so keyword DSNs need both keys.
	lowerDSN := strings.ToLower(dataSourceName)
	isPostgres := strings.HasPrefix(lowerDSN, "postgres://") ||
		strings.HasPrefix(lowerDSN, "postgresql://") ||
		(strings.Contains(lowerDSN, "host=") && strings.Contains(lowerDSN, "dbname="))

	if isPostgres {
		dataSourceName = ApplyPostgresStatementTimeouts(dataSourceName)
		telemetry.Logger.Info("initializing distributed PostgreSQL connection", "source", strings.Split(dataSourceName, "@")[len(strings.Split(dataSourceName, "@"))-1])
		db, err := sql.Open("postgres", dataSourceName)
		if err != nil {
			return nil, fmt.Errorf("open postgres db error: %w", err)
		}

		db.SetMaxOpenConns(50)
		db.SetMaxIdleConns(25)
		db.SetConnMaxLifetime(10 * time.Minute)
		// Recycle idle connections too: without this, connections kept open by
		// the pool can outlive a server-side idle timeout and then fail the first
		// query issued on them.
		db.SetConnMaxIdleTime(5 * time.Minute)

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

	// Connection-level PRAGMAs must live in the DSN: database/sql recycles
	// connections, and a recycled connection would silently lose busy_timeout /
	// synchronous (the classic "works for an hour then randomly fails" bug).
	dsn := dataSourceName
	if dataSourceName != ":memory:" && !strings.Contains(dataSourceName, "_pragma=") {
		sep := "?"
		if strings.Contains(dataSourceName, "?") {
			sep = "&"
		}
		dsn = dataSourceName + sep +
			"_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=synchronous(NORMAL)"
	}

	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open sqlite db error: %w", err)
	}

	// Configure connection pool for SQLite with WAL mode for serialized concurrency.
	// A single connection makes every query (including the per-request auth
	// lookup) strictly serialized, which is why those lookups carry deadlines.
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	db.SetConnMaxLifetime(time.Hour)
	db.SetConnMaxIdleTime(30 * time.Minute)

	if dataSourceName != ":memory:" {
		// Verify the pragmas actually applied; a silent failure here means lock
		// contention would surface as intermittent SQLITE_BUSY at runtime.
		var journalMode string
		if err := db.QueryRow("PRAGMA journal_mode;").Scan(&journalMode); err != nil {
			db.Close()
			return nil, fmt.Errorf("sqlite pragma check failed: %w", err)
		}
		if !strings.EqualFold(journalMode, "wal") {
			db.Close()
			return nil, fmt.Errorf("sqlite could not enable WAL mode (got %q); use a filesystem that supports it", journalMode)
		}
	}

	wrapper := &DB{DB: db, dialect: "sqlite"}
	if err := wrapper.migrate(); err != nil {
		db.Close()
		return nil, fmt.Errorf("migrate sqlite db schema error: %w", err)
	}

	return wrapper, nil
}

// rewriteLikeToILike converts the SQL LIKE operator to PostgreSQL's ILIKE while
// leaving string literals intact.
func rewriteLikeToILike(query string) string {
	var out strings.Builder
	out.Grow(len(query) + 8)
	inString := false
	for i := 0; i < len(query); i++ {
		ch := query[i]
		if ch == '\'' {
			inString = !inString
			out.WriteByte(ch)
			continue
		}
		if !inString && (ch == 'L' || ch == 'l') && i+4 <= len(query) {
			candidate := query[i : i+4]
			if strings.EqualFold(candidate, "LIKE") {
				prevBoundary := i == 0 || isSQLBoundary(query[i-1])
				nextBoundary := i+4 == len(query) || isSQLBoundary(query[i+4])
				if prevBoundary && nextBoundary {
					out.WriteString("ILIKE")
					i += 3
					continue
				}
			}
		}
		out.WriteByte(ch)
	}
	return out.String()
}

func isSQLBoundary(b byte) bool {
	switch b {
	case ' ', '\t', '\n', '\r', '(', ')', ',', '=', '<', '>', '!', '+', '-', '*', '/', ';':
		return true
	}
	return false
}
