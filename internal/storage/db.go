package storage

import (
	"context"
	"database/sql"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/ifnodoraemon/airoute/internal/telemetry"
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

// PostgresLockTimeoutMS bounds how long a statement may wait for a row/table
// lock before the server aborts it with 55P03 (lock_not_available).
//
// Settlement is a short write transaction on a hot wallet row: when two flushes
// contend, waiting the full statement_timeout pins the goroutine (and its
// connection) for 30s while the request-side wait expires anyway. Failing fast
// at 500ms instead turns a serialisation delay into a retryable error, which
// settleCharges retries with jitter.
const PostgresLockTimeoutMS = 500

// ApplyPostgresStatementTimeouts injects statement_timeout (and an idle
// transaction timeout) into a libpq DSN unless the operator already configured
// `options` themselves. Both the URL form and the keyword/value form are
// supported; anything unrecognised is returned unchanged.
func ApplyPostgresStatementTimeouts(dsn string) string {
	if strings.TrimSpace(dsn) == "" {
		return dsn
	}
	opts := fmt.Sprintf("-c statement_timeout=%d -c lock_timeout=%d -c idle_in_transaction_session_timeout=%d",
		PostgresStatementTimeoutMS, PostgresLockTimeoutMS, PostgresStatementTimeoutMS)

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
		telemetry.ScopedLogger("system").Info("initializing distributed PostgreSQL connection", "source", redactDSN(dataSourceName))
		db, err := sql.Open("postgres", dataSourceName)
		if err != nil {
			return nil, fmt.Errorf("open postgres db error: %w", err)
		}

		// Pool budget: every replica multiplies this, so it has to fit the
		// server's max_connections together with maintenance sessions. The old
		// hardcoded 50 x 2 replicas equalled the whole 100-connection budget, so
		// a third replica could never connect and idle connections squatted on
		// slots other replicas needed.
		maxOpen := envPositiveInt("GATEWAY_DB_MAX_OPEN_CONNS", 25)
		maxIdle := envPositiveInt("GATEWAY_DB_MAX_IDLE_CONNS", 8)
		if maxIdle > maxOpen {
			maxIdle = maxOpen
		}
		db.SetMaxOpenConns(maxOpen)
		db.SetMaxIdleConns(maxIdle)
		db.SetConnMaxLifetime(10 * time.Minute)
		// Recycle idle connections too: without this, connections kept open by
		// the pool can outlive a server-side idle timeout and then fail the first
		// query issued on them.
		db.SetConnMaxIdleTime(5 * time.Minute)
		telemetry.ScopedLogger("system").Info("postgres connection pool configured",
			"max_open", maxOpen, "max_idle", maxIdle)

		if err := db.Ping(); err != nil {
			telemetry.ScopedLogger("system").Warn("postgres ping failed, checking connection", "error", err.Error())
		}

		wrapper := &DB{DB: db, dialect: "postgres"}
		if err := wrapper.migratePostgres(); err != nil {
			db.Close()
			return nil, fmt.Errorf("migrate postgres db schema error: %w", err)
		}
		telemetry.ScopedLogger("system").Info("PostgreSQL distributed storage initialized successfully")
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
	//
	// wal_autocheckpoint(0) disables the automatic checkpoint that otherwise fires
	// on the writer every ~1000 pages: on a group-commit settlement path it is a
	// hidden fsync inside a request's transaction. A low-frequency background
	// checkpointer (StartSQLiteCheckpointer) does the work instead, outside the
	// write critical section.
	dsn := dataSourceName
	if dataSourceName != ":memory:" && !strings.Contains(dataSourceName, "_pragma=") {
		sep := "?"
		if strings.Contains(dataSourceName, "?") {
			sep = "&"
		}
		dsn = dataSourceName + sep +
			"_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=synchronous(NORMAL)&_pragma=wal_autocheckpoint(0)"
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

	// Open the read pool after the migrations: they run on the write connection and
	// would otherwise contend with idle readers holding WAL read marks during DDL.
	// A read pool is impossible for :memory: (a second connection would open a
	// *different* empty database), so those callers fall back to the write pool.
	if sqliteReadPoolEligible(dataSourceName) {
		readDB, rerr := sql.Open("sqlite", dsn)
		if rerr != nil {
			telemetry.ScopedLogger("system").Warn(
				"sqlite read pool could not be opened; reads stay on the single write connection",
				"error", rerr.Error())
		} else {
			readDB.SetMaxOpenConns(4)
			readDB.SetMaxIdleConns(4)
			readDB.SetConnMaxLifetime(time.Hour)
			readDB.SetConnMaxIdleTime(30 * time.Minute)
			if perr := readDB.Ping(); perr != nil {
				_ = readDB.Close()
				telemetry.ScopedLogger("system").Warn(
					"sqlite read pool ping failed; reads stay on the single write connection",
					"error", perr.Error())
			} else {
				wrapper.readDB = readDB
				telemetry.ScopedLogger("system").Info("sqlite read pool configured", "max_open", 4)
			}
		}
	}

	return wrapper, nil
}

// sqliteReadPoolEligible reports whether a second pool can read the same SQLite
// database. In-memory databases cannot: every connection to ":memory:" (or a
// "file:...?mode=memory" DSN) creates its own private database, so a read pool
// would see an empty schema.
func sqliteReadPoolEligible(dataSourceName string) bool {
	lower := strings.ToLower(dataSourceName)
	return !strings.Contains(lower, ":memory:") && !strings.Contains(lower, "mode=memory")
}

// defaultSQLiteCheckpointInterval is how often StartSQLiteCheckpointer truncates
// the WAL when the caller asks for the default cadence.
const defaultSQLiteCheckpointInterval = 2 * time.Minute

// StartSQLiteCheckpointer starts the low-frequency WAL checkpoint that replaces
// SQLite's per-connection auto-checkpoint (disabled via wal_autocheckpoint(0)).
//
// The auto-checkpoint used to run inside whichever write transaction happened to
// push the WAL past its page threshold, charging an fsync to an unlucky settle
// call. A background ticker keeps the WAL bounded without putting that work on
// the settlement critical path. The returned stop function is idempotent and the
// caller (server.go) should invoke it on shutdown.
//
// It is a no-op for PostgreSQL and for a zero-value DB (tests), so callers do not
// need to branch on the dialect.
func (db *DB) StartSQLiteCheckpointer(interval time.Duration) (stop func()) {
	if interval <= 0 {
		interval = defaultSQLiteCheckpointInterval
	}
	if db == nil || db.DB == nil || db.Dialect() != "sqlite" {
		return func() {}
	}

	done := make(chan struct{})
	var stopOnce sync.Once
	stop = func() {
		stopOnce.Do(func() { close(done) })
	}

	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-done:
				return
			case <-ticker.C:
				// TRUNCATE resets the WAL to zero bytes; it blocks until readers
				// release their marks, which is acceptable at this cadence.
				if _, err := db.DB.Exec(`PRAGMA wal_checkpoint(TRUNCATE)`); err != nil {
					telemetry.WarnLogger(telemetry.ScopedLogger("system"), "sqlite-wal-checkpoint-failed",
						time.Minute, "background WAL checkpoint failed; the WAL will keep growing until it succeeds",
						"error", err.Error())
				}
			}
		}
	}()
	return stop
}

// keywordDSNPasswordPattern matches the credential keys of a libpq keyword/value
// DSN, in both the `password=secret` and `password='secret with spaces'` forms.
var keywordDSNPasswordPattern = regexp.MustCompile(`(?i)\b((?:ssl)?password\s*=\s*)(?:'[^']*'|"[^"]*"|[^\s]+)`)

// redactDSN removes credentials from a database DSN before it is logged.
//
// It handles both supported forms. The previous implementation kept only the
// text after the last "@", which works for postgres://user:pass@host/db but
// leaves the whole string intact for a libpq keyword/value DSN — that form has no
// "@", so "source=host=… password=…" logged the password verbatim.
func redactDSN(dsn string) string {
	if dsn == "" {
		return dsn
	}
	lower := strings.ToLower(dsn)
	if strings.HasPrefix(lower, "postgres://") || strings.HasPrefix(lower, "postgresql://") {
		u, err := url.Parse(dsn)
		if err != nil {
			return keywordDSNPasswordPattern.ReplaceAllString(dsn, "${1}REDACTED")
		}
		if u.User != nil {
			if _, hasPassword := u.User.Password(); hasPassword {
				u.User = url.UserPassword(u.User.Username(), "REDACTED")
			}
		}
		if q := u.Query(); q.Get("password") != "" {
			q.Set("password", "REDACTED")
			u.RawQuery = q.Encode()
		}
		return u.String()
	}
	return keywordDSNPasswordPattern.ReplaceAllString(dsn, "${1}REDACTED")
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

// envPositiveInt reads a positive integer override from the environment,
// falling back to the caller's default when unset, unparsable or non-positive.
func envPositiveInt(name string, fallback int) int {
	raw := strings.TrimSpace(os.Getenv(name))
	if raw == "" {
		return fallback
	}
	value, err := strconv.Atoi(raw)
	if err != nil || value <= 0 {
		return fallback
	}
	return value
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
