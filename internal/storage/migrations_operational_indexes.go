package storage

import "github.com/ifnodoraemon/airoute/internal/telemetry"

// addOperationalIndexes adds the missing secondary indexes for the tables the
// console and the background maintenance jobs query.
//
//   - verification_codes: VerifyCode() filters on (email, purpose, used) and takes
//     the newest row, but the only index was (email, code) — the second column is
//     useless for that query shape, so every code check scanned and sorted. The
//     expires_at index serves the retention purge.
//   - request_traces: ListRequestTraces() filters by api_key for tenant-scoped
//     lookups; the table carried indexes for trace_id/chat_id/session_id/created_at
//     but not api_key, so those lookups were sequential scans over the largest
//     audit table.
//   - video_tasks: only had a primary key. Operator lookups by account and the
//     retention purge both need (owner, created_at).
//
// Every statement is idempotent (IF NOT EXISTS), so a crash between the DDL and
// the version record replays safely.
func addOperationalIndexes(db *DB) error {
	statements := []string{
		`CREATE INDEX IF NOT EXISTS idx_verify_lookup ON verification_codes(email, purpose, used, id)`,
		`CREATE INDEX IF NOT EXISTS idx_verify_expires ON verification_codes(expires_at)`,
		`CREATE INDEX IF NOT EXISTS idx_trace_api_key ON request_traces(api_key)`,
		`CREATE INDEX IF NOT EXISTS idx_video_tasks_owner ON video_tasks(owner, created_at)`,
	}
	log := telemetry.ScopedLogger("system")
	for _, statement := range statements {
		if _, err := db.DB.Exec(statement); err != nil {
			// An index is a performance aid, not a correctness requirement; do not
			// block startup for it, but make the gap visible.
			log.Warn("could not create operational index", "statement", statement, "error", err.Error())
		}
	}
	return nil
}
