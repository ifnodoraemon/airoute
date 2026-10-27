package storage

import (
	"path/filepath"
	"testing"
)

// newResponseContextRepo opens a fresh SQLite database, running every migration
// (including the response_contexts one).
func newResponseContextRepo(t *testing.T) *Repository {
	t.Helper()
	db, err := OpenDB(filepath.Join(t.TempDir(), "response-contexts.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return NewRepository(db)
}

// The previous_response_id feature needs the table and its expiry index; without
// them every follow-up request would silently lose its context.
func TestResponseContextsMigrationCreatesSchema(t *testing.T) {
	repo := newResponseContextRepo(t)

	var tableCount int
	if err := repo.db.QueryRow(
		`SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = 'response_contexts'`,
	).Scan(&tableCount); err != nil {
		t.Fatalf("query sqlite_master: %v", err)
	}
	if tableCount != 1 {
		t.Fatal("migration did not create the response_contexts table")
	}
	if !indexExists(t, repo, "idx_response_contexts_expires") {
		t.Fatal("migration did not create the expires_at index")
	}
}
