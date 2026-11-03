package storage

import "fmt"

// dropLegacyGovernanceObjects removes every database object that the v1.1
// operational-governance slice left behind. That slice (organization tree,
// budgets and alerts, the wallet ledger, supplier-cost margin reporting) was cut
// on 2026-10-28: the code is gone, and keeping its tables and columns would leave
// dead schema behind on upgraded databases, so they are dropped here.
//
// The drop is irreversible. Databases upgraded past this version lose whatever
// organization attribution, budget usage, wallet ledger history and daily cost
// rollup they had accumulated; an operator who still needs that data must take a
// backup before upgrading. Fresh databases never create these objects, so for
// them this migration is a no-op.
func dropLegacyGovernanceObjects(db *DB) error {
	// Indexes that cover columns dropped below. Dropping them first keeps SQLite's
	// ALTER TABLE DROP COLUMN (which refuses to remove an indexed column) working,
	// and PostgreSQL would drop them implicitly anyway.
	for _, index := range []string{
		"idx_usage_org_created",
		"idx_usage_logs_channel_id",
		"idx_users_org",
		"idx_api_keys_org",
		"idx_billing_settlements_org",
	} {
		if _, err := db.DB.Exec(`DROP INDEX IF EXISTS ` + index); err != nil {
			return fmt.Errorf("drop legacy index %s: %w", index, err)
		}
	}

	// Tables, children before parents (org_memberships references orgs).
	for _, table := range []string{
		"budget_events",
		"budgets",
		"wallet_transactions",
		"org_memberships",
		"orgs",
		"usage_daily",
	} {
		if _, err := db.DB.Exec(`DROP TABLE IF EXISTS ` + table); err != nil {
			return fmt.Errorf("drop legacy table %s: %w", table, err)
		}
	}

	// Columns added by the cut slice. billing_settlements does not exist on some
	// legacy fixtures, hence the per-table existence check.
	for _, column := range []struct {
		table  string
		column string
	}{
		{"billing_settlements", "org_id"},
		{"users", "org_id"},
		{"api_keys", "org_id"},
		{"usage_logs", "org_id"},
		{"usage_logs", "cost_basis_units"},
		{"usage_logs", "channel_id"},
	} {
		if err := dropColumnIfExists(db, column.table, column.column); err != nil {
			return fmt.Errorf("drop column %s.%s: %w", column.table, column.column, err)
		}
	}
	return nil
}
