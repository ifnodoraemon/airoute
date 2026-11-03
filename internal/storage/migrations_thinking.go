package storage

import "fmt"

func addThinkingConfiguration(db *DB) error {
	for _, item := range []struct{ table, column string }{{"channels", "model_capabilities"}, {"model_routes", "thinking_policy"}, {"usage_logs", "thinking"}} {
		exists, err := tableExists(db, item.table)
		if err != nil {
			return err
		}
		if !exists {
			continue
		}
		if db.Dialect() == "postgres" {
			_, err = db.DB.Exec(`ALTER TABLE ` + item.table + ` ADD COLUMN IF NOT EXISTS ` + item.column + ` TEXT`)
		} else if !sqliteColumnExists(db.DB, item.table, item.column) {
			_, err = db.DB.Exec(`ALTER TABLE ` + item.table + ` ADD COLUMN ` + item.column + ` TEXT`)
		}
		if err != nil {
			return fmt.Errorf("add %s.%s: %w", item.table, item.column, err)
		}
	}
	return nil
}
