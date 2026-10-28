package storage

import (
	"database/sql"
	"errors"
	"strings"

	"github.com/ifnodoraemon/airoute/internal/telemetry"
)

// repairStorageIndexesAndTraceSearch is migration 42: it clears out indexes that
// no query can use, gives video_tasks the retention index its purge actually
// scans, aligns video_tasks.created_at with the rest of the schema, and keeps the
// SQLite full-text audit index in step with updates.
//
// The three usage_logs single-column indexes it drops were created before the
// console filters were finalized. Every filter on trace_id/chat_id/session_id is
// `(column = ? OR column LIKE '%?%')`; the LIKE branch cannot use a B-tree and the
// OR forces both sides to be evaluated, so the index is never chosen — it only
// costs an extra write on the hottest insert path. The tenant/key/model filters,
// which really are range-queryable, already have (column, created_at) composites
// from migration 18.
func repairStorageIndexesAndTraceSearch(db *DB) error {
	// 1. Indexes that no query shape can use on either engine.
	for _, index := range []string{"idx_usage_trace", "idx_usage_chat", "idx_usage_session"} {
		if _, err := db.DB.Exec(`DROP INDEX IF EXISTS ` + index); err != nil {
			return err
		}
	}

	if err := repairVideoTaskIndexes(db); err != nil {
		return err
	}
	if err := repairUsageLogFTSTriggers(db); err != nil {
		return err
	}
	return nil
}

// repairVideoTaskIndexes replaces (owner, created_at) with two single-column
// indexes.
//
// The retention purge filters and orders by created_at alone, which a composite
// whose leading column is owner cannot serve; the only read path looks a row up
// by primary key. Recreating `idx_video_tasks_owner` on (owner) keeps the name
// the operational-index test and any operator documentation refer to.
func repairVideoTaskIndexes(db *DB) error {
	exists, err := db.tableExists("video_tasks")
	if err != nil {
		return err
	}
	if !exists {
		return nil
	}
	if _, err := db.DB.Exec(`DROP INDEX IF EXISTS idx_video_tasks_owner`); err != nil {
		return err
	}
	for _, statement := range []string{
		`CREATE INDEX IF NOT EXISTS idx_video_tasks_created ON video_tasks(created_at)`,
		`CREATE INDEX IF NOT EXISTS idx_video_tasks_owner ON video_tasks(owner)`,
	} {
		if _, err := db.DB.Exec(statement); err != nil {
			return err
		}
	}
	if db.Dialect() != "postgres" {
		// SQLite stores an untyped TIMESTAMP column as NUMERIC affinity and the
		// original DEFAULT CURRENT_TIMESTAMP text is already UTC, so there is
		// nothing to convert.
		return nil
	}
	// PostgreSQL upgrades: the column was created as zone-less `TIMESTAMP`, whose
	// CURRENT_TIMESTAMP default is the session's local wall clock. Convert the
	// stored values (written as local time) to the corresponding instant and make
	// future defaults UTC-correct like every other table. Guarded by the catalogue
	// so a re-run is a no-op, and skipped if the table predates the column.
	var dataType string
	err = db.QueryRow(`SELECT data_type FROM information_schema.columns
		WHERE table_schema = current_schema() AND table_name = 'video_tasks' AND column_name = 'created_at'`).Scan(&dataType)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	if !strings.EqualFold(dataType, "timestamp without time zone") {
		return nil
	}
	if _, err := db.DB.Exec(`ALTER TABLE video_tasks
		ALTER COLUMN created_at TYPE TIMESTAMPTZ USING created_at AT TIME ZONE current_setting('TimeZone')`); err != nil {
		return err
	}
	telemetry.ScopedLogger("system").Info("migration 42 converted video_tasks.created_at to TIMESTAMPTZ")
	return nil
}

// repairUsageLogFTSTriggers keeps the SQLite FTS5 index in step with UPDATEs.
//
// usage_logs is not append-only: migrations 24 and 32 rewrite status_code and
// error_message on rows that predate a precision/reason fix, and error_message is
// one of the indexed columns. The insert and delete triggers alone leave the
// index holding the pre-repair text, so a search for the reconstructed reason
// misses the row while a search for the old artefact still matches it. The final
// 'rebuild' repopulates external-content rows that were already updated before
// this trigger existed.
func repairUsageLogFTSTriggers(db *DB) error {
	if db.Dialect() != "sqlite" {
		return nil
	}
	var tables int
	if err := db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = 'usage_logs_fts'`).Scan(&tables); err != nil {
		return err
	}
	if tables == 0 {
		// FTS5/trigram was unavailable when migration 20 ran; keyword search uses
		// the LIKE fallback, so there is no index to maintain.
		return nil
	}
	statements := []string{
		`CREATE TRIGGER IF NOT EXISTS usage_logs_fts_au AFTER UPDATE OF trace_id, chat_id, session_id, model, channel, error_message ON usage_logs BEGIN
			INSERT INTO usage_logs_fts(usage_logs_fts, rowid, trace_id, chat_id, session_id, model, channel, error_message)
			VALUES ('delete', old.id, old.trace_id, old.chat_id, old.session_id, old.model, old.channel, old.error_message);
			INSERT INTO usage_logs_fts(rowid, trace_id, chat_id, session_id, model, channel, error_message)
			VALUES (new.id, new.trace_id, new.chat_id, new.session_id, new.model, new.channel, new.error_message);
		END`,
		`INSERT INTO usage_logs_fts(usage_logs_fts) VALUES('rebuild')`,
	}
	for _, statement := range statements {
		if _, err := db.DB.Exec(statement); err != nil {
			// The FTS index is a search accelerator, not a correctness
			// requirement; report and keep going rather than block startup.
			telemetry.ScopedLogger("system").Warn("could not repair the SQLite FTS audit index; keyword search keeps the LIKE scan",
				"error", err.Error())
			return nil
		}
	}
	return nil
}
