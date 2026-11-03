package storage

import (
	"path/filepath"
	"strings"
	"testing"
)

// TestAPIKeyPublicIDMigrationBackfillsIdempotently covers the upgrade path for
// rows that predate migration 39: the backfill names them, a replay keeps the
// identifier stable (the migration runner replays after a crash between the DDL
// and the version record), and the unique index rejects a duplicate.
func TestAPIKeyPublicIDMigrationBackfillsIdempotently(t *testing.T) {
	db, err := OpenDB(filepath.Join(t.TempDir(), "public_ids.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	repository := NewRepository(db)

	rec := &APIKeyRecord{Key: "sk-airoute-legacy-row-0123456789", TenantID: "tenant", Status: "active"}
	if err := repository.CreateAPIKey(rec); err != nil {
		t.Fatalf("create key: %v", err)
	}

	// Simulate a row written before the identifier existed.
	if _, err := db.DB.Exec(`UPDATE api_keys SET public_id = '' WHERE id = ?`, rec.ID); err != nil {
		t.Fatalf("clear public id: %v", err)
	}
	if err := addAPIKeyPublicIDs(db); err != nil {
		t.Fatalf("backfill migration: %v", err)
	}
	var backfilled string
	if err := db.DB.QueryRow(`SELECT public_id FROM api_keys WHERE id = ?`, rec.ID).Scan(&backfilled); err != nil {
		t.Fatalf("read backfilled public id: %v", err)
	}
	if !strings.HasPrefix(backfilled, apiKeyPublicIDPrefix) || len(backfilled) != len(apiKeyPublicIDPrefix)+apiKeyPublicIDBytes*2 {
		t.Fatalf("backfilled public id = %q, want an %q identifier", backfilled, apiKeyPublicIDPrefix)
	}

	// A replay must not rename existing keys.
	if err := addAPIKeyPublicIDs(db); err != nil {
		t.Fatalf("replay migration: %v", err)
	}
	var replayed string
	if err := db.DB.QueryRow(`SELECT public_id FROM api_keys WHERE id = ?`, rec.ID).Scan(&replayed); err != nil {
		t.Fatalf("read replayed public id: %v", err)
	}
	if replayed != backfilled {
		t.Fatalf("public id changed across a replay: %q -> %q", backfilled, replayed)
	}

	// The partial unique index must reject a second row claiming the same id.
	if _, err := db.DB.Exec(`INSERT INTO api_keys (key, tenant_id, status, public_id) VALUES (?, ?, 'active', ?)`,
		HashAPIKey("sk-airoute-another-0123456789"), "tenant-2", backfilled); err == nil {
		t.Fatal("a duplicate public id was accepted; the unique index is missing")
	}
}

// TestAPIKeyPublicIDMigrationFillsEveryExistingRow pins the case the console
// actually hit: several legacy rows, all without an identifier.
func TestAPIKeyPublicIDMigrationFillsEveryExistingRow(t *testing.T) {
	db, err := OpenDB(filepath.Join(t.TempDir(), "public_ids_many.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	repository := NewRepository(db)

	for i := 0; i < 5; i++ {
		rec := &APIKeyRecord{Key: "sk-airoute-legacy-" + string(rune('a'+i)) + "-0123456789", TenantID: "tenant", Status: "active"}
		if err := repository.CreateAPIKey(rec); err != nil {
			t.Fatalf("create key %d: %v", i, err)
		}
	}
	if _, err := db.DB.Exec(`UPDATE api_keys SET public_id = ''`); err != nil {
		t.Fatalf("clear public ids: %v", err)
	}
	if err := addAPIKeyPublicIDs(db); err != nil {
		t.Fatalf("backfill migration: %v", err)
	}

	rows, err := db.DB.Query(`SELECT public_id FROM api_keys ORDER BY id`)
	if err != nil {
		t.Fatalf("read public ids: %v", err)
	}
	defer rows.Close()
	seen := map[string]bool{}
	for rows.Next() {
		var publicID string
		if err := rows.Scan(&publicID); err != nil {
			t.Fatalf("scan public id: %v", err)
		}
		if !strings.HasPrefix(publicID, apiKeyPublicIDPrefix) {
			t.Fatalf("row still has no public id: %q", publicID)
		}
		if seen[publicID] {
			t.Fatalf("duplicate public id after backfill: %q", publicID)
		}
		seen[publicID] = true
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate public ids: %v", err)
	}
	if len(seen) != 5 {
		t.Fatalf("backfilled %d identifiers, want 5", len(seen))
	}
}
