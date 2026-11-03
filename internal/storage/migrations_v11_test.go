package storage

import "testing"

// TestSupplierCostsMigration verifies the one v1.1 object that survived the
// 2026-10-28 scope cut. The org/budget/wallet-ledger/daily-rollup tables are no
// longer created for fresh databases, and replaying the migration list must stay
// idempotent for databases that already have them.
func TestSupplierCostsMigration(t *testing.T) {
	db, err := OpenDB(":memory:")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer db.Close()

	var name string
	if err := db.QueryRow(`SELECT name FROM sqlite_master WHERE type = 'table' AND name = ?`, "supplier_costs").Scan(&name); err != nil {
		t.Fatalf("supplier_costs table missing: %v", err)
	}
	for _, dropped := range []string{"budgets", "budget_events", "wallet_transactions", "usage_daily", "orgs", "org_memberships"} {
		var found string
		err := db.QueryRow(`SELECT name FROM sqlite_master WHERE type = 'table' AND name = ?`, dropped).Scan(&found)
		if err == nil {
			t.Fatalf("table %s must not be created after the scope cut", dropped)
		}
	}
	if sqliteColumnExists(db.DB, "usage_logs", "cost_basis_units") {
		t.Fatal("usage_logs.cost_basis_units must not be created after the scope cut")
	}
	if sqliteColumnExists(db.DB, "usage_logs", "channel_id") {
		t.Fatal("usage_logs.channel_id must not be created after the scope cut")
	}

	var version int
	if err := db.QueryRow(`SELECT COALESCE(MAX(version), 0) FROM schema_migrations`).Scan(&version); err != nil {
		t.Fatalf("read schema version: %v", err)
	}
	// v40 drops the objects the cut slice used to add, so an upgraded database
	// ends at least at that version.
	if version < 40 {
		t.Fatalf("schema version = %d, want >= 40", version)
	}

	// Replaying the migration list must not error or duplicate anything.
	if err := db.migrate(); err != nil {
		t.Fatalf("replay migrations: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO supplier_costs (channel, model, input_cost_units_per_million, output_cost_units_per_million, status)
		VALUES ('ch', 'm', 1000, 2000, 'active')`); err != nil {
		t.Fatalf("insert after replay: %v", err)
	}
	var id int64
	if err := db.QueryRow(`SELECT id FROM supplier_costs WHERE channel = 'ch' AND model = 'm'`).Scan(&id); err != nil || id <= 0 {
		t.Fatalf("read inserted price: id=%d err=%v", id, err)
	}
}
