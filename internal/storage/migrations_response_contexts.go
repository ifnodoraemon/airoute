package storage

// createResponseContexts creates the table that stores a Responses API
// conversation under its client-facing response id, so a follow-up request can
// reference it with previous_response_id.
//
// Rows are keyed by the response id the client received, carry the owning
// caller (user id, else API-key digest), and expire after a short TTL. The
// expires_at index is what both the lookup filter and the opportunistic prune
// scan, so it is created together with the table.
func createResponseContexts(db *DB) error {
	var ddl string
	if db.Dialect() == "postgres" {
		ddl = `CREATE TABLE IF NOT EXISTS response_contexts (
			id VARCHAR(128) PRIMARY KEY,
			owner VARCHAR(256) NOT NULL,
			messages TEXT NOT NULL,
			created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
			expires_at TIMESTAMPTZ NOT NULL
		);
		CREATE INDEX IF NOT EXISTS idx_response_contexts_expires ON response_contexts(expires_at);`
	} else {
		ddl = `CREATE TABLE IF NOT EXISTS response_contexts (
			id TEXT PRIMARY KEY,
			owner TEXT NOT NULL,
			messages TEXT NOT NULL,
			created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
			expires_at DATETIME NOT NULL
		);
		CREATE INDEX IF NOT EXISTS idx_response_contexts_expires ON response_contexts(expires_at);`
	}
	_, err := db.DB.Exec(ddl)
	return err
}
