package storage

import "fmt"

// addSupplierCosts creates the supplier cost table and the nullable cost-basis
// column on usage_logs. Cost basis NULL means "unknown" (no configured supplier
// price at write time) and must never be treated as zero, otherwise every
// unpriced channel would look infinitely profitable.
func addSupplierCosts(db *DB) error {
	table := `CREATE TABLE IF NOT EXISTS supplier_costs (
		id BIGSERIAL PRIMARY KEY,
		channel VARCHAR(128) NOT NULL,
		model VARCHAR(128) NOT NULL,
		input_cost_units_per_million BIGINT NOT NULL DEFAULT 0,
		output_cost_units_per_million BIGINT NOT NULL DEFAULT 0,
		status VARCHAR(16) NOT NULL DEFAULT 'active',
		created_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP,
		updated_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP,
		UNIQUE (channel, model)
	)`
	if db.Dialect() != "postgres" {
		table = `CREATE TABLE IF NOT EXISTS supplier_costs (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			channel TEXT NOT NULL,
			model TEXT NOT NULL,
			input_cost_units_per_million INTEGER NOT NULL DEFAULT 0,
			output_cost_units_per_million INTEGER NOT NULL DEFAULT 0,
			status TEXT NOT NULL DEFAULT 'active',
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			UNIQUE (channel, model)
		)`
	}
	if _, err := db.DB.Exec(table); err != nil {
		return fmt.Errorf("create supplier_costs: %w", err)
	}
	if _, err := db.DB.Exec(`CREATE INDEX IF NOT EXISTS idx_supplier_costs_lookup ON supplier_costs(channel, model, status)`); err != nil {
		return err
	}
	return addUsageLogsCostBasis(db)
}

// addUsageLogsCostBasis adds the nullable integer column. Existing rows stay
// NULL (unknown), which is the truthful value for logs written before pricing
// was configured.
func addUsageLogsCostBasis(db *DB) error {
	if db.Dialect() == "postgres" {
		if _, err := db.DB.Exec(`ALTER TABLE usage_logs ADD COLUMN IF NOT EXISTS cost_basis_units BIGINT`); err != nil {
			return fmt.Errorf("add cost_basis_units: %w", err)
		}
		return nil
	}
	if !sqliteColumnExists(db.DB, "usage_logs", "cost_basis_units") {
		if _, err := db.DB.Exec(`ALTER TABLE usage_logs ADD COLUMN cost_basis_units INTEGER`); err != nil {
			return fmt.Errorf("add cost_basis_units: %w", err)
		}
	}
	return nil
}

// CreateSupplierCosts is the exported migration entrypoint.
func CreateSupplierCosts(db *DB) error {
	return addSupplierCosts(db)
}
