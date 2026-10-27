package storage

import (
	"fmt"
	"strings"
	"testing"
)

// TestOrgMigrationCreatesRootAndColumns pins the additive contract of v30: the
// root unit exists, every attribution column is present, and replaying the
// migration is safe.
func TestOrgMigrationCreatesRootAndColumns(t *testing.T) {
	db, err := OpenDB(":memory:")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer db.Close()

	var rootID int64
	var rootPath string
	if err := db.QueryRow(`SELECT id, path FROM orgs WHERE code = 'default'`).Scan(&rootID, &rootPath); err != nil {
		t.Fatalf("default organization: %v", err)
	}
	if rootID <= 0 {
		t.Fatalf("default organization id = %d", rootID)
	}
	if want := fmt.Sprintf("/%d/", rootID); rootPath != want {
		t.Fatalf("default path = %q, want %q", rootPath, want)
	}

	for _, table := range []string{"users", "api_keys", "usage_logs", "billing_settlements"} {
		if !sqliteColumnExists(db.DB, table, "org_id") {
			t.Fatalf("%s.org_id is missing", table)
		}
	}

	// A replay (crash between DDL and version record) must not duplicate the root
	// or fail on the already-present columns.
	if err := addOrgGovernance(db); err != nil {
		t.Fatalf("replay migration: %v", err)
	}
	var roots int
	if err := db.QueryRow(`SELECT COUNT(*) FROM orgs WHERE code = 'default'`).Scan(&roots); err != nil {
		t.Fatal(err)
	}
	if roots != 1 {
		t.Fatalf("default organizations = %d, want 1", roots)
	}
}

// TestOrgBackfillAttributesLegacyRows inserts the shape of pre-v30 data (NULL
// org_id everywhere) and verifies the backfill walks user -> key -> audit /
// settlement, with the root as the final fallback.
func TestOrgBackfillAttributesLegacyRows(t *testing.T) {
	db, err := OpenDB(":memory:")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer db.Close()

	var rootID int64
	if err := db.QueryRow(`SELECT id FROM orgs WHERE code = 'default'`).Scan(&rootID); err != nil {
		t.Fatal(err)
	}

	userID, err := db.InsertGetID(`INSERT INTO users (username, password_hash, role, status, balance_units)
		VALUES (?, ?, ?, ?, 0)`, "legacy-org-user", "x", "user", "active")
	if err != nil {
		t.Fatalf("insert legacy user: %v", err)
	}
	keyHash := strings.Repeat("a", 64)
	keyID, err := db.InsertGetID(`INSERT INTO api_keys (key, tenant_id, user_id, status, budget_units, used_cost_units)
		VALUES (?, ?, ?, ?, 0, 0)`, keyHash, "legacy-org-user", userID, "active")
	if err != nil {
		t.Fatalf("insert legacy key: %v", err)
	}
	if _, err := db.DB.Exec(`INSERT INTO usage_logs (trace_id, api_key, model, channel, total_tokens, cost, cost_units, status_code)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`, "tr-legacy-org", keyHash, "m", "c", 10, 0.01, 10000000, 200); err != nil {
		t.Fatalf("insert legacy usage: %v", err)
	}
	if _, err := db.DB.Exec(`INSERT INTO billing_settlements (billing_id, api_key, tenant_id, cost_units, total_tokens)
		VALUES (?, ?, ?, ?, ?)`, "bill-legacy-org", keyHash, "legacy-org-user", 10000000, 10); err != nil {
		t.Fatalf("insert legacy settlement: %v", err)
	}

	if err := backfillOrgAttribution(db); err != nil {
		t.Fatalf("backfill: %v", err)
	}

	checks := []struct {
		name  string
		query string
		arg   int64
	}{
		{"user", `SELECT org_id FROM users WHERE id = ?`, userID},
		{"key", `SELECT org_id FROM api_keys WHERE id = ?`, keyID},
		{"usage", `SELECT org_id FROM usage_logs WHERE trace_id = ?`, 0},
		{"settlement", `SELECT org_id FROM billing_settlements WHERE billing_id = ?`, 0},
	}
	for _, check := range checks {
		var orgID int64
		var err error
		if check.name == "usage" {
			err = db.QueryRow(check.query, "tr-legacy-org").Scan(&orgID)
		} else if check.name == "settlement" {
			err = db.QueryRow(check.query, "bill-legacy-org").Scan(&orgID)
		} else {
			err = db.QueryRow(check.query, check.arg).Scan(&orgID)
		}
		if err != nil {
			t.Fatalf("read %s org_id: %v", check.name, err)
		}
		if orgID != rootID {
			t.Fatalf("%s org_id = %d, want default %d", check.name, orgID, rootID)
		}
	}

	// Idempotent: a second pass changes nothing and does not error.
	if err := backfillOrgAttribution(db); err != nil {
		t.Fatalf("second backfill: %v", err)
	}
}
