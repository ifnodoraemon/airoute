package storage

import (
	"context"
	"database/sql"
	"sync"

	_ "github.com/lib/pq"
	_ "modernc.org/sqlite"
)

// DB wraps standard sql.DB connection with dialect-aware query adaptation.
type DB struct {
	*sql.DB
	dialect string // "sqlite" or "postgres"

	// readDB is a second pool for hot read-only queries in SQLite mode. The write
	// pool is pinned to a single connection (SQLite serialises writers anyway), so
	// without this the per-request auth lookup and the pricing reload queue behind
	// the settlement writer. WAL lets readers run concurrently with the single
	// writer, so a separate read pool is safe; reads only ever see committed data.
	// PostgreSQL does not need it: its main pool is already multi-connection.
	// nil means "use the main pool" (always the case for PostgreSQL and :memory:).
	// It is written only during OpenDB and never reassigned, so Reader() can read it
	// without a lock even while Close runs.
	readDB *sql.DB

	// closeOnce keeps Close idempotent without mutating readDB, so a concurrent
	// Reader() never races with shutdown.
	closeOnce sync.Once
	closeErr  error
}

// Dialect returns the active database engine dialect ("sqlite" or "postgres").
func (db *DB) Dialect() string {
	if db == nil || db.dialect == "" {
		return "sqlite"
	}
	return db.dialect
}

// Reader returns the pool that read-only queries should use: the SQLite read
// pool when one was opened, otherwise the main pool (PostgreSQL, :memory: and
// zero-value test DBs). Callers must still pass the query through Rebind: the
// read pool is a raw *sql.DB, so on PostgreSQL the '?' placeholders would
// otherwise reach lib/pq verbatim.
func (db *DB) Reader() *sql.DB {
	if db == nil {
		return nil
	}
	if db.readDB != nil {
		return db.readDB
	}
	return db.DB
}

// Close closes the read pool (when present) and the main pool. The embedded
// *sql.DB would otherwise only close the writer, leaving the SQLite read
// connections and their WAL read marks alive until the process exits. It is
// idempotent and safe to call concurrently with reads.
func (db *DB) Close() error {
	if db == nil {
		return nil
	}
	db.closeOnce.Do(func() {
		if db.readDB != nil {
			db.closeErr = db.readDB.Close()
		}
		if db.DB != nil {
			if err := db.DB.Close(); err != nil && db.closeErr == nil {
				db.closeErr = err
			}
		}
	})
	return db.closeErr
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

// ExecContext is Exec bound to a context, with dialect rebind.
//
// Without this override the promoted *sql.DB method sends '?' placeholders to
// PostgreSQL verbatim, which is a syntax error. That silently disabled the two
// Context-bound repository queries that used them: the auth middleware's
// authoritative api-key lookup (whose error was discarded, so every data-plane
// request quietly fell back to the 10s in-memory snapshot) and
// GetUserByIDContext.
func (db *DB) ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error) {
	return db.DB.ExecContext(ctx, db.Rebind(query), args...)
}

// QueryContext is Query bound to a context, with dialect rebind.
func (db *DB) QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error) {
	return db.DB.QueryContext(ctx, db.Rebind(query), args...)
}

// QueryRowContext is QueryRow bound to a context, with dialect rebind.
func (db *DB) QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row {
	return db.DB.QueryRowContext(ctx, db.Rebind(query), args...)
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

// ExecContext is Exec bound to a context, with dialect rebind.
func (tx *Tx) ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error) {
	return tx.Tx.ExecContext(ctx, tx.db.Rebind(query), args...)
}

// QueryContext is Query bound to a context, with dialect rebind.
func (tx *Tx) QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error) {
	return tx.Tx.QueryContext(ctx, tx.db.Rebind(query), args...)
}

// QueryRowContext is QueryRow bound to a context, with dialect rebind.
func (tx *Tx) QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row {
	return tx.Tx.QueryRowContext(ctx, tx.db.Rebind(query), args...)
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

// tableExists reports whether a table is present in the current schema. Migrations
// that touch optional/legacy tables use it so an upgrade from an older schema does
// not fail on a table a later migration creates.
func (db *DB) tableExists(table string) (bool, error) {
	if db == nil || db.DB == nil {
		return false, nil
	}
	var query string
	if db.Dialect() == "postgres" {
		query = `SELECT COUNT(*) FROM information_schema.tables WHERE table_schema = current_schema() AND table_name = ?`
	} else {
		query = `SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = ?`
	}
	var count int
	if err := db.QueryRow(query, table).Scan(&count); err != nil {
		return false, err
	}
	return count > 0, nil
}
