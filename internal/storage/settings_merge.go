package storage

import (
	"context"
	"database/sql"
	"errors"
)

// MergeSetting atomically reads, transforms and writes a system setting under a
// row lock so concurrent replicas cannot lose each other's updates. The plain
// GetSetting+SetSetting pair is a read-modify-write race: two replicas reading the
// same value and writing back their own merge silently drop the peer's change.
// When merge returns the current value unchanged the write is skipped.
func (r *Repository) MergeSetting(key string, merge func(current string) (string, error)) error {
	tx, err := r.db.BeginTx(context.Background())
	if err != nil {
		return err
	}
	defer tx.Rollback()
	query := `SELECT value FROM system_settings WHERE key = ?`
	if tx.db != nil && tx.db.Dialect() == "postgres" {
		query += ` FOR UPDATE`
	}
	var current string
	err = tx.QueryRow(query, key).Scan(&current)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	next, err := merge(current)
	if err != nil {
		return err
	}
	if next == current {
		return tx.Commit()
	}
	if _, err := tx.Exec(`INSERT INTO system_settings (key, value, updated_at) VALUES (?, ?, CURRENT_TIMESTAMP)
		ON CONFLICT(key) DO UPDATE SET value = excluded.value, updated_at = CURRENT_TIMESTAMP`, key, next); err != nil {
		return err
	}
	return tx.Commit()
}