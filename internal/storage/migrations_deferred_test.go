package storage

import (
	"errors"
	"testing"
)

// TestDeferredMigrationIsRetriedUntilItSucceeds pins the contract introduced
// for migration 27: a deferred migration must not abort startup, must not be
// recorded, and must run again on the next start.
func TestDeferredMigrationIsRetriedUntilItSucceeds(t *testing.T) {
	db, err := OpenDB(":memory:")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer db.Close()

	attempts := 0
	deferred := Migration{
		Version: 9001,
		Name:    "deferred_probe",
		Up: func(*DB) error {
			attempts++
			return errMigrationDeferred
		},
	}

	if err := db.runMigrations([]Migration{deferred}); err != nil {
		t.Fatalf("a deferred migration must not fail startup: %v", err)
	}
	if attempts != 1 {
		t.Fatalf("attempts = %d, want 1", attempts)
	}
	var recorded int
	if err := db.DB.QueryRow(`SELECT COUNT(*) FROM schema_migrations WHERE version = 9001`).Scan(&recorded); err != nil {
		t.Fatal(err)
	}
	if recorded != 0 {
		t.Fatalf("deferred migration was recorded %d time(s); it would never retry", recorded)
	}

	// Second start: still failing -> runs again, still not recorded.
	if err := db.runMigrations([]Migration{deferred}); err != nil {
		t.Fatalf("second deferred run: %v", err)
	}
	if attempts != 2 {
		t.Fatalf("attempts = %d, want 2", attempts)
	}

	// Once the operator fixes the data, the same version applies and is recorded.
	recovered := deferred
	recovered.Up = func(*DB) error {
		attempts++
		return nil
	}
	if err := db.runMigrations([]Migration{recovered}); err != nil {
		t.Fatalf("recovered run: %v", err)
	}
	if attempts != 3 {
		t.Fatalf("attempts = %d, want 3", attempts)
	}
	if err := db.DB.QueryRow(`SELECT COUNT(*) FROM schema_migrations WHERE version = 9001`).Scan(&recorded); err != nil {
		t.Fatal(err)
	}
	if recorded != 1 {
		t.Fatalf("recovered migration recorded %d time(s), want 1", recorded)
	}
}

// A normal (non-deferred) failure must still abort startup: silence is only for
// conditions the operator can fix without losing the gateway.
func TestNonDeferredMigrationFailureStillAborts(t *testing.T) {
	db, err := OpenDB(":memory:")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer db.Close()

	boom := errors.New("boom")
	err = db.runMigrations([]Migration{{
		Version: 9002,
		Name:    "explode",
		Up:      func(*DB) error { return boom },
	}})
	if err == nil || !errors.Is(err, boom) {
		t.Fatalf("expected the migration failure to abort startup, got %v", err)
	}
}
