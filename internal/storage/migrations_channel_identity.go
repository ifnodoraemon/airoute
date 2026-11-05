package storage

import (
	"context"
	"fmt"
)

// separateChannelTrails repairs identity only. Billing amounts, audit IDs,
// timestamps and settlements are deliberately untouched.
func separateChannelTrails(db *DB) error {
	for _, table := range []string{"usage_logs", "request_traces"} {
		if !tableHasColumn(db, table, "channel") {
			continue
		}
		if db.Dialect() == "postgres" {
			if _, err := db.Exec(`ALTER TABLE ` + table + ` ADD COLUMN IF NOT EXISTS channel_trail TEXT NOT NULL DEFAULT ''`); err != nil {
				return err
			}
		} else if !sqliteColumnExists(db.DB, table, "channel_trail") {
			if _, err := db.Exec(`ALTER TABLE ` + table + ` ADD COLUMN channel_trail TEXT NOT NULL DEFAULT ''`); err != nil {
				return err
			}
		}
		if err := repairChannelIdentities(db, table); err != nil {
			return fmt.Errorf("repair %s channel identities: %w", table, err)
		}
	}
	// The new path stays searchable, including channels attempted before the
	// terminal upstream. Rebuild triggers and index against the current columns.
	if db.Dialect() == "sqlite" && tableHasColumn(db, "usage_logs", "channel_trail") {
		return addUsageLogFTSIndex(db)
	}
	if db.Dialect() == "postgres" && tableHasColumn(db, "usage_logs", "channel_trail") {
		return addUsageLogSearchIndexes(db)
	}
	return nil
}

func repairChannelIdentities(db *DB, table string) error {
	tx, err := db.BeginTx(context.Background())
	if err != nil {
		return err
	}
	defer tx.Rollback()
	rows, err := tx.Query(`SELECT DISTINCT channel FROM ` + table + ` WHERE channel LIKE '%→%'`)
	if err != nil {
		return err
	}
	var trails []string
	for rows.Next() {
		var trail string
		if err := rows.Scan(&trail); err != nil {
			rows.Close()
			return err
		}
		trails = append(trails, trail)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	for _, trail := range trails {
		channel, _ := channelIdentity(trail, "")
		if _, err := tx.Exec(`UPDATE `+table+` SET channel_trail=CASE WHEN COALESCE(channel_trail,'')='' THEN channel ELSE channel_trail END, channel=? WHERE channel=?`, channel, trail); err != nil {
			return err
		}
	}
	return tx.Commit()
}
