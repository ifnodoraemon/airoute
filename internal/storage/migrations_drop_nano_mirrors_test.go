package storage

import "testing"

// TestDropLegacyNanoMoneyColumns covers the dual-mirror cleanup: a database that
// still carries the migration-12 *_nano columns must end up with them gone, with
// every amount preserved in its *_units column, and a replay must be a no-op.
func TestDropLegacyNanoMoneyColumns(t *testing.T) {
	db, err := OpenDB(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	// A database created from scratch must never grow the mirrors.
	for _, m := range legacyNanoMirrors {
		if tableHasColumn(db, m.table, m.nano) {
			t.Fatalf("fresh database already has %s.%s", m.table, m.nano)
		}
	}

	// Simulate a database upgraded from migration 12: the mirrors exist, one row
	// per table holds an amount only in the mirror, and another row has a stale
	// mirror next to a correct units value.
	for _, stmt := range []string{
		`ALTER TABLE users ADD COLUMN balance_nano INTEGER DEFAULT 0`,
		`ALTER TABLE api_keys ADD COLUMN used_cost_nano INTEGER DEFAULT 0`,
		`ALTER TABLE api_keys ADD COLUMN budget_nano INTEGER DEFAULT 0`,
		`ALTER TABLE usage_logs ADD COLUMN cost_nano INTEGER DEFAULT 0`,
		`INSERT INTO users (username, password_hash, balance, balance_units, balance_nano) VALUES ('nano-only', 'h', 0, 0, 1250000000)`,
		`INSERT INTO users (username, password_hash, balance, balance_units, balance_nano) VALUES ('units-win', 'h', 2, 2000000000, 999)`,
		`INSERT INTO api_keys (key, tenant_id, used_cost, used_cost_units, used_cost_nano, budget_units, budget_nano) VALUES ('sk-nano', 't', 0, 0, 500000000, 0, 10000000000)`,
		`INSERT INTO api_keys (key, tenant_id, used_cost, used_cost_units, used_cost_nano, budget_units, budget_nano) VALUES ('sk-units', 't', 3, 3000000000, 7, 4000000000, 8)`,
		`INSERT INTO usage_logs (trace_id, model, cost, cost_units, cost_nano) VALUES ('tr-nano', 'm', 0, 0, 700000000)`,
		`INSERT INTO usage_logs (trace_id, model, cost, cost_units, cost_nano) VALUES ('tr-units', 'm', 0.8, 800000000, 5)`,
	} {
		if _, err := db.Exec(stmt); err != nil {
			t.Fatalf("fixture %q: %v", stmt, err)
		}
	}

	for pass := 0; pass < 2; pass++ {
		if err := dropLegacyNanoMoneyColumns(db); err != nil {
			t.Fatalf("pass %d: %v", pass, err)
		}
	}

	for _, m := range legacyNanoMirrors {
		if tableHasColumn(db, m.table, m.nano) {
			t.Fatalf("%s.%s survived the migration", m.table, m.nano)
		}
	}

	for _, tc := range []struct {
		query string
		want  int64
	}{
		{`SELECT balance_units FROM users WHERE username = 'nano-only'`, 1250000000},
		{`SELECT balance_units FROM users WHERE username = 'units-win'`, 2000000000},
		{`SELECT used_cost_units FROM api_keys WHERE key = 'sk-nano'`, 500000000},
		{`SELECT budget_units FROM api_keys WHERE key = 'sk-nano'`, 10000000000},
		{`SELECT used_cost_units FROM api_keys WHERE key = 'sk-units'`, 3000000000},
		{`SELECT budget_units FROM api_keys WHERE key = 'sk-units'`, 4000000000},
		{`SELECT cost_units FROM usage_logs WHERE trace_id = 'tr-nano'`, 700000000},
		{`SELECT cost_units FROM usage_logs WHERE trace_id = 'tr-units'`, 800000000},
	} {
		var got int64
		if err := db.QueryRow(tc.query).Scan(&got); err != nil {
			t.Fatalf("%s: %v", tc.query, err)
		}
		if got != tc.want {
			t.Fatalf("%s = %d, want %d", tc.query, got, tc.want)
		}
	}
}
