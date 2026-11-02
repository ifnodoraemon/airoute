package storage

import "fmt"

// addChannelCapacity leaves existing channels unlimited. Each column is added
// independently so replay after a partial migration is safe on both dialects.
func addChannelCapacity(db *DB) error {
	if exists, err := tableExists(db, "channels"); err != nil || !exists {
		return err
	}
	for _, column := range []string{"max_concurrency", "queue_limit", "queue_wait_seconds"} {
		if db.Dialect() == "postgres" {
			if _, err := db.DB.Exec(`ALTER TABLE channels ADD COLUMN IF NOT EXISTS ` + column + ` INTEGER NOT NULL DEFAULT 0`); err != nil {
				return fmt.Errorf("add channels.%s: %w", column, err)
			}
		} else if !sqliteColumnExists(db.DB, "channels", column) {
			if _, err := db.DB.Exec(`ALTER TABLE channels ADD COLUMN ` + column + ` INTEGER NOT NULL DEFAULT 0`); err != nil {
				return fmt.Errorf("add channels.%s: %w", column, err)
			}
		}
	}
	return nil
}
