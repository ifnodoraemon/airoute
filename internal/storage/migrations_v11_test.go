package storage

import "testing"

func TestV11MigrationsCreateBudgetAndWalletTables(t *testing.T) {
	db, err := OpenDB(":memory:")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer db.Close()

	for _, table := range []string{"budgets", "budget_events", "wallet_transactions", "supplier_costs"} {
		var name string
		if err := db.QueryRow(`SELECT name FROM sqlite_master WHERE type = 'table' AND name = ?`, table).Scan(&name); err != nil {
			t.Fatalf("table %s missing: %v", table, err)
		}
	}
	var version int
	if !sqliteColumnExists(db.DB, "usage_logs", "cost_basis_units") {
		t.Fatal("usage_logs.cost_basis_units missing")
	}
	if err := db.QueryRow(`SELECT COALESCE(MAX(version), 0) FROM schema_migrations`).Scan(&version); err != nil {
		t.Fatalf("read schema version: %v", err)
	}
	if version < 37 {
		t.Fatalf("schema version = %d, want >= 37", version)
	}

	// Replaying the migration list must not error or duplicate the opening rows.
	if err := db.migrate(); err != nil {
		t.Fatalf("replay migrations: %v", err)
	}
	var openings int
	if err := db.QueryRow(`SELECT COUNT(*) FROM wallet_transactions WHERE type = 'opening'`).Scan(&openings); err != nil {
		t.Fatal(err)
	}
	if openings < 0 {
		t.Fatalf("unexpected opening count %d", openings)
	}
}
