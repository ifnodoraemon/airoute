package storage

import (
	"database/sql"
	"errors"
	"fmt"
)

// addUsageDaily adds the stable channel id to audit rows and the daily rollup
// table. Channel names are unique today, but a name is mutable display data; the
// rollup keeps the id so renaming a channel does not split history.
func addUsageDaily(db *DB) error {
	if err := addUsageLogChannelID(db); err != nil {
		return err
	}
	if err := backfillUsageLogChannelIDs(db); err != nil {
		return err
	}
	table := `CREATE TABLE IF NOT EXISTS usage_daily (
		day VARCHAR(10) NOT NULL,
		org_id BIGINT NOT NULL DEFAULT 0,
		channel_id BIGINT NOT NULL DEFAULT 0,
		model VARCHAR(128) NOT NULL DEFAULT '',
		requests BIGINT NOT NULL DEFAULT 0,
		prompt_tokens BIGINT NOT NULL DEFAULT 0,
		completion_tokens BIGINT NOT NULL DEFAULT 0,
		total_tokens BIGINT NOT NULL DEFAULT 0,
		billed_units BIGINT NOT NULL DEFAULT 0,
		cost_basis_units BIGINT NOT NULL DEFAULT 0,
		known_cost_requests BIGINT NOT NULL DEFAULT 0,
		PRIMARY KEY (day, org_id, channel_id, model)
	)`
	if db.Dialect() != "postgres" {
		table = `CREATE TABLE IF NOT EXISTS usage_daily (
			day TEXT NOT NULL,
			org_id INTEGER NOT NULL DEFAULT 0,
			channel_id INTEGER NOT NULL DEFAULT 0,
			model TEXT NOT NULL DEFAULT '',
			requests INTEGER NOT NULL DEFAULT 0,
			prompt_tokens INTEGER NOT NULL DEFAULT 0,
			completion_tokens INTEGER NOT NULL DEFAULT 0,
			total_tokens INTEGER NOT NULL DEFAULT 0,
			billed_units INTEGER NOT NULL DEFAULT 0,
			cost_basis_units INTEGER NOT NULL DEFAULT 0,
			known_cost_requests INTEGER NOT NULL DEFAULT 0,
			PRIMARY KEY (day, org_id, channel_id, model)
		)`
	}
	if _, err := db.DB.Exec(table); err != nil {
		return fmt.Errorf("create usage_daily: %w", err)
	}
	_, err := db.DB.Exec(`CREATE INDEX IF NOT EXISTS idx_usage_daily_day ON usage_daily(day)`)
	return err
}

func addUsageLogChannelID(db *DB) error {
	if db.Dialect() == "postgres" {
		if _, err := db.DB.Exec(`ALTER TABLE usage_logs ADD COLUMN IF NOT EXISTS channel_id BIGINT`); err != nil {
			return fmt.Errorf("add channel_id: %w", err)
		}
	} else if !sqliteColumnExists(db.DB, "usage_logs", "channel_id") {
		if _, err := db.DB.Exec(`ALTER TABLE usage_logs ADD COLUMN channel_id INTEGER`); err != nil {
			return fmt.Errorf("add channel_id: %w", err)
		}
	}
	if _, err := db.DB.Exec(`CREATE INDEX IF NOT EXISTS idx_usage_logs_channel_id ON usage_logs(channel_id)`); err != nil {
		return err
	}
	return nil
}

// backfillUsageLogChannelIDs resolves historical rows by the unique channel
// name. Rows whose channel no longer exists stay NULL, which means "unknown"
// rather than a fabricated id.
func backfillUsageLogChannelIDs(db *DB) error {
	// Legacy/partial fixtures may not have a channels table at all; there is
	// nothing to resolve there and failing the migration would block upgrades.
	var probe int
	if err := db.QueryRow(`SELECT 1 FROM channels LIMIT 1`).Scan(&probe); err != nil && !errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	_, err := db.DB.Exec(`UPDATE usage_logs SET channel_id = (
		SELECT id FROM channels WHERE channels.name = usage_logs.channel
	) WHERE channel_id IS NULL AND channel IS NOT NULL AND channel <> ''`)
	return err
}

// CreateUsageDaily is the exported migration entrypoint.
func CreateUsageDaily(db *DB) error {
	return addUsageDaily(db)
}
