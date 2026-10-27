package storage

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
)

// TestMoneyUnitsRepairMigration reproduces the schema drift that broke a live
// deployment: version 12 ("add_integer_money_columns") was edited in place from
// the legacy *_nano columns to *_units after it had already been applied, so
// upgraded databases kept *_nano and every api_keys read / usage_logs insert
// failed with `column "budget_units" does not exist`.
//
// The test builds a database that is pinned at version 14 with only the legacy
// columns, then runs the migrator and asserts that:
//   - the *_units columns now exist,
//   - they are backfilled from the legacy *_nano values,
//   - version 15 is recorded,
//   - running the migrator a second time is a no-op.
func TestMoneyUnitsRepairMigration(t *testing.T) {
	dsn := filepath.Join(t.TempDir(), "legacy.db")

	// 1. Create a legacy-shaped database and pin it at version 14.
	raw, err := sql.Open("sqlite", dsn)
	if err != nil {
		t.Fatalf("open raw sqlite: %v", err)
	}
	legacy := []string{
		`CREATE TABLE schema_migrations (version INTEGER PRIMARY KEY, name TEXT NOT NULL, applied_at DATETIME DEFAULT CURRENT_TIMESTAMP);`,
		`CREATE TABLE users (id INTEGER PRIMARY KEY AUTOINCREMENT, username TEXT UNIQUE NOT NULL, email TEXT DEFAULT '',
			password_hash TEXT NOT NULL, role TEXT DEFAULT 'user', status TEXT DEFAULT 'active',
			balance REAL DEFAULT 0, group_name TEXT DEFAULT 'default',
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP, updated_at DATETIME DEFAULT CURRENT_TIMESTAMP);`,
		`CREATE TABLE api_keys (id INTEGER PRIMARY KEY AUTOINCREMENT, key TEXT UNIQUE NOT NULL, tenant_id TEXT NOT NULL,
			user_id INTEGER DEFAULT 0, group_name TEXT DEFAULT 'default', allowed_models TEXT,
			rpm INTEGER DEFAULT 60, tpm INTEGER DEFAULT 100000, budget REAL DEFAULT 0,
			used_tokens INTEGER DEFAULT 0, used_cost REAL DEFAULT 0, status TEXT DEFAULT 'active',
			format_validation TEXT DEFAULT '', created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP);`,
		`CREATE TABLE usage_logs (id INTEGER PRIMARY KEY AUTOINCREMENT, trace_id TEXT DEFAULT '', chat_id TEXT DEFAULT '',
			session_id TEXT DEFAULT '', api_key TEXT, tenant_id TEXT, model TEXT, channel TEXT,
			prompt_tokens INTEGER DEFAULT 0, completion_tokens INTEGER DEFAULT 0, cached_tokens INTEGER DEFAULT 0,
			total_tokens INTEGER DEFAULT 0, cost REAL DEFAULT 0, is_off_peak INTEGER DEFAULT 0,
			off_peak_discount REAL DEFAULT 1.0, duration_ms INTEGER DEFAULT 0, ttft_ms INTEGER DEFAULT 0,
			status_code INTEGER DEFAULT 200, usage_source TEXT DEFAULT 'upstream',
			error_message TEXT DEFAULT '', created_at DATETIME DEFAULT CURRENT_TIMESTAMP);`,
		// The legacy mirrors the buggy version 12 produced.
		`ALTER TABLE users ADD COLUMN balance_nano INTEGER DEFAULT 0;`,
		`ALTER TABLE api_keys ADD COLUMN used_cost_nano INTEGER DEFAULT 0;`,
		`ALTER TABLE api_keys ADD COLUMN budget_nano INTEGER DEFAULT 0;`,
		`ALTER TABLE usage_logs ADD COLUMN cost_nano INTEGER DEFAULT 0;`,
		`INSERT INTO users (username, password_hash, role, balance, balance_nano) VALUES ('alice', 'x', 'user', 12.5, 12500000000);`,
		`INSERT INTO api_keys (key, tenant_id, budget, used_cost, budget_nano, used_cost_nano) VALUES ('sk-legacy', 't1', 5.0, 1.25, 5000000000, 1250000000);`,
		`INSERT INTO usage_logs (trace_id, api_key, tenant_id, model, cost, cost_nano) VALUES ('tr-1', 'sk-legacy', 't1', 'm1', 0.5, 500000000);`,
	}
	for _, stmt := range legacy {
		if _, err := raw.Exec(stmt); err != nil {
			t.Fatalf("legacy setup failed on %q: %v", stmt, err)
		}
	}
	for v := 1; v <= 14; v++ {
		if _, err := raw.Exec(`INSERT INTO schema_migrations (version, name) VALUES (?, 'legacy')`, v); err != nil {
			t.Fatalf("seed migration version %d: %v", v, err)
		}
	}
	raw.Close()

	// 2. Open through the real migrator, which must apply version 15.
	db, err := OpenDB(dsn)
	if err != nil {
		t.Fatalf("OpenDB after legacy setup: %v", err)
	}
	defer db.Close()

	for _, tc := range []struct{ table, col string }{
		{"users", "balance_units"},
		{"api_keys", "used_cost_units"},
		{"api_keys", "budget_units"},
		{"usage_logs", "cost_units"},
	} {
		var count int
		if err := db.DB.QueryRow(`SELECT COUNT(*) FROM pragma_table_info(?) WHERE name = ?`, tc.table, tc.col).Scan(&count); err != nil {
			t.Fatalf("pragma lookup %s.%s: %v", tc.table, tc.col, err)
		}
		if count == 0 {
			t.Fatalf("migration did not add %s.%s", tc.table, tc.col)
		}
	}

	var balanceUnits int64
	if err := db.DB.QueryRow(`SELECT balance_units FROM users WHERE username = 'alice'`).Scan(&balanceUnits); err != nil {
		t.Fatalf("read balance_units: %v", err)
	}
	if balanceUnits != 12500000000 {
		t.Fatalf("balance_units not backfilled from legacy nano: got %d", balanceUnits)
	}

	// api_keys.key holds the credential digest after migration 29, so the legacy
	// plaintext is no longer queryable (and must not be).
	var plaintextRows int
	if err := db.DB.QueryRow(`SELECT COUNT(*) FROM api_keys WHERE key = 'sk-legacy'`).Scan(&plaintextRows); err != nil {
		t.Fatalf("count legacy plaintext keys: %v", err)
	}
	if plaintextRows != 0 {
		t.Fatalf("legacy cleartext key survived migration 29")
	}

	var usedUnits, budgetUnits int64
	if err := db.DB.QueryRow(`SELECT used_cost_units, budget_units FROM api_keys WHERE key = ?`, HashAPIKey("sk-legacy")).Scan(&usedUnits, &budgetUnits); err != nil {
		t.Fatalf("read api_keys units: %v", err)
	}
	if usedUnits != 1250000000 || budgetUnits != 5000000000 {
		t.Fatalf("api_keys units not backfilled: used=%d budget=%d", usedUnits, budgetUnits)
	}

	var costUnits int64
	if err := db.DB.QueryRow(`SELECT cost_units FROM usage_logs WHERE trace_id = 'tr-1'`).Scan(&costUnits); err != nil {
		t.Fatalf("read cost_units: %v", err)
	}
	if costUnits != 500000000 {
		t.Fatalf("usage_logs cost_units not backfilled: got %d", costUnits)
	}

	var applied int
	if err := db.DB.QueryRow(`SELECT COUNT(*) FROM schema_migrations WHERE version = 15`).Scan(&applied); err != nil {
		t.Fatalf("check version 15: %v", err)
	}
	if applied != 1 {
		t.Fatalf("expected version 15 to be recorded, got %d", applied)
	}

	// 3. Re-opening must be a no-op (idempotent), not an error.
	if err := db.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	db2, err := OpenDB(dsn)
	if err != nil {
		t.Fatalf("second OpenDB (idempotency): %v", err)
	}
	defer db2.Close()

	// The data plane read path that originally failed must now work.
	rec, err := NewRepository(db2).GetAPIKeyBySecret(context.Background(), "sk-legacy")
	if err != nil {
		t.Fatalf("GetAPIKeyBySecret failed after repair: %v", err)
	}
	if rec.UsedCostUnits != 1250000000 {
		t.Fatalf("repository read wrong units: %d", rec.UsedCostUnits)
	}
}
