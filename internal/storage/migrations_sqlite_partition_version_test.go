package storage

import (
	"path/filepath"
	"testing"
)

// TestSQLiteRecordsThePartitioningVersion pins the no-op counterpart of the
// PostgreSQL-only migration 43.
//
// The partitioning migration is PG-only, but the version is recorded on SQLite
// too (see migrations_sqlite.go) so that a reader comparing the two dialect
// histories does not have to work out whether 43 was skipped by accident. This
// test fails if someone "tidies up" the no-op away, and it also proves that a
// fresh SQLite database still runs the full set without applying any
// partitioning DDL: usage_logs stays a single table.
func TestSQLiteRecordsThePartitioningVersion(t *testing.T) {
	db, err := OpenDB(filepath.Join(t.TempDir(), "partitioning-version.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	var count int
	if err := db.DB.QueryRow(`SELECT COUNT(*) FROM schema_migrations WHERE version = 43`).Scan(&count); err != nil {
		t.Fatalf("query schema_migrations: %v", err)
	}
	if count != 1 {
		t.Fatalf("schema_migrations rows for version 43 = %d, want 1", count)
	}

	// The no-op must not have created the PostgreSQL shadow table or a partitioned
	// shape: SQLite keeps the single-table + UNIQUE(billing_id) path.
	if tableExists, err := tableExists(db, "usage_logs_partitioned"); err != nil {
		t.Fatalf("check usage_logs_partitioned: %v", err)
	} else if tableExists {
		t.Fatal("SQLite must not create usage_logs_partitioned")
	}
}
