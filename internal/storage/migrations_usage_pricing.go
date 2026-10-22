package storage

func addUsagePricingSnapshot(db *DB) error {
	if db.Dialect() == "postgres" {
		_, err := db.DB.Exec(`ALTER TABLE usage_logs ADD COLUMN IF NOT EXISTS pricing_snapshot TEXT DEFAULT ''`)
		return err
	}
	if sqliteColumnExists(db.DB, "usage_logs", "pricing_snapshot") {
		return nil
	}
	_, err := db.DB.Exec(`ALTER TABLE usage_logs ADD COLUMN pricing_snapshot TEXT DEFAULT ''`)
	return err
}
