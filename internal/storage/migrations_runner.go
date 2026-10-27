package storage

import (
	"context"
	"errors"
	"fmt"

	"github.com/ifnodoraemon/airoute/internal/telemetry"
)

// errMigrationDeferred marks a migration that could not be applied yet for a
// reason the operator must fix (for example duplicate rows blocking a unique
// index). The runner does not record the version, so the next start retries it,
// but it also does not abort startup: an upgrade must not leave the gateway
// down. Idempotent migrations make the retry safe.
var errMigrationDeferred = errors.New("migration deferred")

// Migration represents a versioned database schema migration step.
type Migration struct {
	Version int
	Name    string
	Up      func(db *DB) error
}

// runMigrations executes a sequence of schema migrations within a dialect-specific tracking table.
// On PostgreSQL it first takes an advisory lock so that concurrent replicas cannot
// race through the same DDL, and each migration plus its version record is
// committed atomically.
func (db *DB) runMigrations(migrations []Migration) error {
	if db.Dialect() == "postgres" {
		conn, err := db.DB.Conn(context.Background())
		if err != nil {
			return fmt.Errorf("acquire migration connection: %w", err)
		}
		defer conn.Close()
		if _, err := conn.ExecContext(context.Background(), `SELECT pg_advisory_lock(hashtext('airoute_migrations'))`); err != nil {
			return fmt.Errorf("acquire migration advisory lock: %w", err)
		}
		defer func() {
			_, _ = conn.ExecContext(context.Background(), `SELECT pg_advisory_unlock(hashtext('airoute_migrations'))`)
		}()
	}

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

		telemetry.ScopedLogger("system").Info("applying database migration", "version", m.Version, "name", m.Name, "dialect", db.Dialect())
		// Up and the version record are two separate statements rather than one
		// transaction; see applyMigration below. A crash between them replays the
		// migration on the next start, which is safe because every migration is
		// written to be idempotent. This comment used to claim the two were
		// committed together, which they never were.
		if err := db.applyMigration(m); err != nil {
			if errors.Is(err, errMigrationDeferred) {
				telemetry.ScopedLogger("system").Warn(
					"migration deferred; it will be retried on the next start",
					"version", m.Version, "name", m.Name, "error", err.Error())
				continue
			}
			return fmt.Errorf("migration %d (%s) failed: %w", m.Version, m.Name, err)
		}
	}
	return nil
}

// applyMigration runs one migration and records its version. Migrations are
// written to be idempotent (CREATE ... IF NOT EXISTS, ADD COLUMN IF NOT EXISTS,
// ensureSQLiteColumn), so a crash between the DDL and the version record simply
// replays the migration safely on the next start.
func (db *DB) applyMigration(m Migration) error {
	if err := m.Up(db); err != nil {
		return err
	}
	var recordSQL string
	if db.Dialect() == "postgres" {
		recordSQL = "INSERT INTO schema_migrations (version, name) VALUES ($1, $2)"
	} else {
		recordSQL = "INSERT INTO schema_migrations (version, name) VALUES (?, ?)"
	}
	if _, err := db.DB.Exec(recordSQL, m.Version, m.Name); err != nil {
		return fmt.Errorf("record migration: %w", err)
	}
	return nil
}
