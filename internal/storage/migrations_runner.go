package storage

import (
	"fmt"

	"github.com/ifnodoraemon/airoute/internal/telemetry"
)

// Migration represents a versioned database schema migration step.
type Migration struct {
	Version int
	Name    string
	Up      func(db *DB) error
}

// runMigrations executes a sequence of schema migrations within a dialect-specific tracking table.
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
