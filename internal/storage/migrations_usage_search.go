package storage

import "github.com/ifnodoraemon/airoute/internal/telemetry"

// addUsageLogSearchIndexes makes the console's free-text search usable at scale.
//
// The keyword box searches six columns with `LIKE '%q%'` (rewritten to ILIKE on
// PostgreSQL), a pattern no B-tree index can serve, so every search degraded to a
// sequential scan. pg_trgm's GIN indexes support exactly that shape.
//
// The extension may be unavailable on a managed database where the gateway role
// cannot install extensions. In that case search keeps working through the scan:
// the migration reports it and continues instead of blocking startup.
func addUsageLogSearchIndexes(db *DB) error {
	log := telemetry.ScopedLogger("system")
	if db.Dialect() != "postgres" {
		// SQLite deployments hold small audit trails and modernc's SQLite has no
		// equivalent index; the LIKE scan stays.
		return nil
	}
	if _, err := db.DB.Exec(`CREATE EXTENSION IF NOT EXISTS pg_trgm`); err != nil {
		log.Warn("pg_trgm is unavailable; audit keyword search will fall back to a table scan", "error", err.Error())
		return nil
	}
	for _, column := range []string{"trace_id", "chat_id", "session_id", "model", "channel", "error_message"} {
		query := `CREATE INDEX IF NOT EXISTS idx_usage_trgm_` + column + ` ON usage_logs USING GIN (` + column + ` gin_trgm_ops)`
		if _, err := db.DB.Exec(query); err != nil {
			log.Warn("failed to create audit trigram index", "column", column, "error", err.Error())
		}
	}
	return nil
}
