package storage

import "github.com/ifnodoraemon/airoute/internal/telemetry"

// addUsageLogFTSIndex adds a SQLite FTS5 trigram index for keyword search.
//
// SQLite cannot use a B-tree for `LIKE '%q%'`, so every keyword search scanned
// six columns. FTS5's trigram tokenizer indexes exactly those substrings, and its
// external-content table keeps the audit rows as the single source of truth.
//
// PostgreSQL already covers this with the pg_trgm GIN indexes from v19.
func addUsageLogFTSIndex(db *DB) error {
	if db.Dialect() != "sqlite" {
		return nil
	}
	log := telemetry.ScopedLogger("system")

	var existing int
	if err := db.DB.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = 'usage_logs_fts'`).Scan(&existing); err != nil {
		return err
	}
	if existing == 0 {
		if _, err := db.DB.Exec(`CREATE VIRTUAL TABLE usage_logs_fts USING fts5(
			trace_id, chat_id, session_id, model, channel, error_message,
			content='usage_logs', content_rowid='id', tokenize='trigram'
		)`); err != nil {
			// Older SQLite builds may lack FTS5 or the trigram tokenizer. Search
			// keeps working through the LIKE scan; do not block startup.
			log.Warn("SQLite FTS5 trigram index unavailable; audit keyword search keeps the LIKE scan", "error", err.Error())
			return nil
		}
	}

	statements := []string{
		// usage_logs rows are append-only, so insert and purge are the only events
		// the index has to follow.
		`CREATE TRIGGER IF NOT EXISTS usage_logs_fts_ai AFTER INSERT ON usage_logs BEGIN
			INSERT INTO usage_logs_fts(rowid, trace_id, chat_id, session_id, model, channel, error_message)
			VALUES (new.id, new.trace_id, new.chat_id, new.session_id, new.model, new.channel, new.error_message);
		END`,
		`CREATE TRIGGER IF NOT EXISTS usage_logs_fts_ad AFTER DELETE ON usage_logs BEGIN
			INSERT INTO usage_logs_fts(usage_logs_fts, rowid, trace_id, chat_id, session_id, model, channel, error_message)
			VALUES ('delete', old.id, old.trace_id, old.chat_id, old.session_id, old.model, old.channel, old.error_message);
		END`,
		// Backfill rows that predate the index; 'rebuild' reads the content table.
		`INSERT INTO usage_logs_fts(usage_logs_fts) VALUES('rebuild')`,
	}
	for _, statement := range statements {
		if _, err := db.DB.Exec(statement); err != nil {
			log.Warn("failed to prepare the SQLite FTS5 audit index; keyword search keeps the LIKE scan", "error", err.Error())
			return nil
		}
	}
	return nil
}
