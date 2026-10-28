package storage

import "fmt"

// addSupplierCosts creates the supplier cost table. It is a price reference
// only: the gateway keeps the contracted per-million-token rates so an operator
// can compare them with our own list price, and no request cost is derived from
// them (the margin analytics that used to consume them was cut on 2026-10-28).
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
	return nil
}

// CreateSupplierCosts is the exported migration entrypoint.
func CreateSupplierCosts(db *DB) error {
	return addSupplierCosts(db)
}
