package storage

import (
	"database/sql"
	"path/filepath"
	"testing"
)

// Regression (A12): the migration must rewrite cleartext credentials in
// api_keys.key, usage_logs.api_key, billing_settlements.api_key and
// request_traces.api_key to their SHA-256 digests, keep the ledger joins intact,
// and be idempotent.
func TestAPIKeyDigestMigrationRewritesHistory(t *testing.T) {
	dsn := filepath.Join(t.TempDir(), "legacy-keys.db")

	// 1. Build a pre-migration database: cleartext credential in every referencing
	// column, as a 28-era deployment would have it.
	raw, err := sql.Open("sqlite", dsn)
	if err != nil {
		t.Fatalf("open raw db: %v", err)
	}
	seed := []string{
		`CREATE TABLE api_keys (
			id INTEGER PRIMARY KEY AUTOINCREMENT, key TEXT UNIQUE NOT NULL, tenant_id TEXT NOT NULL,
			user_id INTEGER DEFAULT 0, group_name TEXT DEFAULT 'default', allowed_models TEXT,
			rpm INTEGER DEFAULT 60, tpm INTEGER DEFAULT 100000, budget REAL DEFAULT 0,
			used_tokens INTEGER DEFAULT 0, used_cost REAL DEFAULT 0, status TEXT DEFAULT 'active',
			format_validation TEXT DEFAULT '', created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			used_cost_units INTEGER DEFAULT 0, budget_units INTEGER DEFAULT 0)`,
		`CREATE TABLE usage_logs (
			id INTEGER PRIMARY KEY AUTOINCREMENT, trace_id TEXT DEFAULT '', api_key TEXT,
			tenant_id TEXT, model TEXT, cost REAL DEFAULT 0, status_code INTEGER DEFAULT 200,
			cost_units INTEGER DEFAULT 0, created_at DATETIME DEFAULT CURRENT_TIMESTAMP)`,
		`CREATE TABLE billing_settlements (
			id INTEGER PRIMARY KEY AUTOINCREMENT, billing_id TEXT NOT NULL, api_key VARCHAR(255) DEFAULT '',
			tenant_id VARCHAR(128) DEFAULT '', cost_units INTEGER NOT NULL DEFAULT 0,
			total_tokens INTEGER NOT NULL DEFAULT 0, created_at DATETIME DEFAULT CURRENT_TIMESTAMP)`,
		`CREATE TABLE request_traces (
			id INTEGER PRIMARY KEY AUTOINCREMENT, trace_id TEXT NOT NULL, api_key TEXT DEFAULT '',
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP)`,
		`CREATE TABLE schema_migrations (version INTEGER PRIMARY KEY, name TEXT NOT NULL, applied_at DATETIME DEFAULT CURRENT_TIMESTAMP)`,
		`INSERT INTO api_keys (key, tenant_id, used_cost, used_cost_units, budget, budget_units)
			VALUES ('sk-airoute-legacycredential000000000000', 'tenant-a', 2.5, 2500000000, 10, 10000000000)`,
		`INSERT INTO usage_logs (trace_id, api_key, tenant_id, model, cost, cost_units)
			VALUES ('tr-1', 'sk-airoute-legacycredential000000000000', 'tenant-a', 'm', 2.5, 2500000000)`,
		`INSERT INTO billing_settlements (billing_id, api_key, tenant_id, cost_units, total_tokens)
			VALUES ('bill-1', 'sk-airoute-legacycredential000000000000', 'tenant-a', 2500000000, 100)`,
		`INSERT INTO request_traces (trace_id, api_key) VALUES ('tr-1', 'sk-airoute-legacycredential000000000000')`,
	}
	for _, statement := range seed {
		if _, err := raw.Exec(statement); err != nil {
			raw.Close()
			t.Fatalf("legacy seed failed: %v (%s)", err, statement)
		}
	}
	for version := 1; version <= 28; version++ {
		if _, err := raw.Exec(`INSERT INTO schema_migrations (version, name) VALUES (?, 'legacy')`, version); err != nil {
			raw.Close()
			t.Fatalf("seed migration version %d: %v", version, err)
		}
	}
	raw.Close()

	const credential = "sk-airoute-legacycredential000000000000"
	wantDigest := HashAPIKey(credential)

	// 2. Open through the real migrator: version 29 rewrites the credential.
	db, err := OpenDB(dsn)
	if err != nil {
		t.Fatalf("OpenDB after legacy setup: %v", err)
	}
	defer db.Close()

	for _, tc := range []struct{ table, query string }{
		{"api_keys", `SELECT key FROM api_keys`},
		{"usage_logs", `SELECT api_key FROM usage_logs`},
		{"billing_settlements", `SELECT api_key FROM billing_settlements`},
		{"request_traces", `SELECT api_key FROM request_traces`},
	} {
		rows, err := db.DB.Query(tc.query)
		if err != nil {
			t.Fatalf("read %s: %v", tc.table, err)
		}
		for rows.Next() {
			var value string
			if err := rows.Scan(&value); err != nil {
				rows.Close()
				t.Fatalf("scan %s: %v", tc.table, err)
			}
			if value == credential {
				rows.Close()
				t.Fatalf("%s still contains the cleartext credential", tc.table)
			}
			if value != "" && value != wantDigest {
				rows.Close()
				t.Fatalf("%s.api_key = %q, want the key digest", tc.table, value)
			}
		}
		rows.Close()
	}

	// 3. The ledger join must still work: the key's counters are readable by digest
	// and the audit/settlement rows still reference it.
	var usedUnits int64
	if err := db.DB.QueryRow(`SELECT used_cost_units FROM api_keys WHERE key = ?`, wantDigest).Scan(&usedUnits); err != nil {
		t.Fatalf("read key by digest: %v", err)
	}
	if usedUnits != 2500000000 {
		t.Fatalf("used_cost_units = %d, want 2500000000", usedUnits)
	}

	repo := NewRepository(db)
	// Reconciliation joins billing_settlements.api_key to api_keys.key: both sides
	// are digests now, so the ledger must be balanced (drift 0) after the rewrite.
	reconciler := &BillingReconciler{repo: repo}
	drift, driftedKeys, err := reconciler.ledgerDrift()
	if err != nil {
		t.Fatalf("ledger drift query: %v", err)
	}
	if drift != 0 || driftedKeys != 0 {
		t.Fatalf("ledger drift after migration = %d units across %d keys, want 0", drift, driftedKeys)
	}

	// The audit row is still attributed to the key digest.
	var attributed int64
	if err := db.DB.QueryRow(`SELECT COALESCE(SUM(cost_units), 0) FROM usage_logs WHERE api_key = ?`, wantDigest).Scan(&attributed); err != nil {
		t.Fatalf("read attributed usage: %v", err)
	}
	if attributed != 2500000000 {
		t.Fatalf("usage attributed to the key digest = %d, want 2500000000", attributed)
	}

	// The authentication path hashes the presented credential and matches the
	// stored value: hash(presented) must still find the row. (A plain query is used
	// here because this minimal legacy schema has no users table to join.)
	var resolvedID int64
	if err := db.DB.QueryRow(`SELECT id FROM api_keys WHERE key = ?`, HashAPIKey(credential)).Scan(&resolvedID); err != nil {
		t.Fatalf("credential stopped authenticating after the migration: %v", err)
	}
	if resolvedID != 1 {
		t.Fatalf("resolved key id = %d, want 1", resolvedID)
	}

	// 4. Idempotency: re-running the migration must be a no-op.
	if err := migrateAPIKeysToDigests(db); err != nil {
		t.Fatalf("re-running the digest migration: %v", err)
	}
	var stored string
	if err := db.DB.QueryRow(`SELECT key FROM api_keys WHERE id = 1`).Scan(&stored); err != nil {
		t.Fatalf("read key after replay: %v", err)
	}
	if stored != wantDigest {
		t.Fatalf("replay changed the digest: %q", stored)
	}

	// 5. A second OpenDB (fresh migrator) must also be a no-op.
	db2, err := OpenDB(dsn)
	if err != nil {
		t.Fatalf("second OpenDB: %v", err)
	}
	defer db2.Close()
	if err := db2.DB.QueryRow(`SELECT key FROM api_keys WHERE id = 1`).Scan(&stored); err != nil {
		t.Fatalf("read key after reopen: %v", err)
	}
	if stored != wantDigest {
		t.Fatalf("reopen changed the digest: %q", stored)
	}
}
