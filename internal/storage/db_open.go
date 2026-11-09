package storage

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/ifnodoraemon/airoute/internal/telemetry"
)

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
		return openPostgresDB(dataSourceName)
	}
	return openSQLiteDB(dataSourceName)
}

func openPostgresDB(dataSourceName string) (*DB, error) {
	dataSourceName = ApplyPostgresStatementTimeouts(dataSourceName)
	telemetry.ScopedLogger("system").Info("initializing distributed PostgreSQL connection", "source", redactDSN(dataSourceName))
	db, err := sql.Open("postgres", dataSourceName)
	if err != nil {
		return nil, fmt.Errorf("open postgres db error: %w", err)
	}

	configurePostgresPool(db)

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

func openSQLiteDB(dataSourceName string) (*DB, error) {
	// Local SQLite mode
	if dataSourceName != ":memory:" {
		dir := filepath.Dir(dataSourceName)
		if err := os.MkdirAll(dir, 0755); err != nil {
			return nil, fmt.Errorf("create db directory error: %w", err)
		}
	}

	dsn := sqliteConnectionDSN(dataSourceName)

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

	if err := verifySQLiteWAL(db, dataSourceName); err != nil {
		db.Close()
		return nil, err
	}

	wrapper := &DB{DB: db, dialect: "sqlite"}
	if err := wrapper.migrate(); err != nil {
		db.Close()
		return nil, fmt.Errorf("migrate sqlite db schema error: %w", err)
	}

	if sqliteReadPoolEligible(dataSourceName) {
		openSQLiteReadPool(wrapper, dsn)
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

func configurePostgresPool(db *sql.DB) {
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
func sqliteConnectionDSN(dataSourceName string) string {
	dsn := dataSourceName
	if dataSourceName != ":memory:" && !strings.Contains(dataSourceName, "_pragma=") {
		sep := "?"
		if strings.Contains(dataSourceName, "?") {
			sep = "&"
		}
		dsn = dataSourceName + sep +
			"_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=synchronous(NORMAL)&_pragma=wal_autocheckpoint(0)"
	}

	return dsn
}

func verifySQLiteWAL(db *sql.DB, dataSourceName string) error {
	if dataSourceName != ":memory:" {
		// Verify the pragmas actually applied; a silent failure here means lock
		// contention would surface as intermittent SQLITE_BUSY at runtime.
		var journalMode string
		if err := db.QueryRow("PRAGMA journal_mode;").Scan(&journalMode); err != nil {
			return fmt.Errorf("sqlite pragma check failed: %w", err)
		}
		if !strings.EqualFold(journalMode, "wal") {
			return fmt.Errorf("sqlite could not enable WAL mode (got %q); use a filesystem that supports it", journalMode)
		}
	}

	return nil
}

// Open readers after migrations so idle readers cannot hold WAL marks during DDL.
func openSQLiteReadPool(wrapper *DB, dsn string) {
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
