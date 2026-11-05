package storage

import "fmt"

func addSupplierQuoteFields(db *DB) error {
	exists, err := tableExists(db, "supplier_costs")
	if err != nil || !exists {
		return err
	}
	fields := []struct{ name, definition string }{
		{"currency", "TEXT NOT NULL DEFAULT 'CNY'"},
		{"billing_mode", "TEXT NOT NULL DEFAULT 'token'"},
		{"cache_read_cost_units_per_million", "BIGINT"},
		{"cache_write_cost_units_per_million", "BIGINT"},
		{"fixed_cost_units", "BIGINT"},
	}
	for _, field := range fields {
		if db.Dialect() == "postgres" {
			_, err = db.DB.Exec(`ALTER TABLE supplier_costs ADD COLUMN IF NOT EXISTS ` + field.name + ` ` + field.definition)
		} else if !sqliteColumnExists(db.DB, "supplier_costs", field.name) {
			_, err = db.DB.Exec(`ALTER TABLE supplier_costs ADD COLUMN ` + field.name + ` ` + field.definition)
		}
		if err != nil {
			return fmt.Errorf("add supplier_costs.%s: %w", field.name, err)
		}
	}
	return nil
}
