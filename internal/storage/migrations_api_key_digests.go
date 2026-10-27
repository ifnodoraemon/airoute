package storage

import (
	"fmt"

	"github.com/ifnodoraemon/airoute/internal/telemetry"
)

// migrateAPIKeysToDigests replaces every cleartext bearer credential at rest with
// its SHA-256 digest.
//
// Before this migration api_keys.key, usage_logs.api_key, billing_settlements.api_key
// and request_traces.api_key all held the credential itself, so a database dump,
// backup or read-only replica exposed working credentials. The digest is
// deterministic (see HashAPIKey), which keeps the entire ledger intact:
//
//   - settlement still runs "UPDATE api_keys SET used_cost_units = ... WHERE key = ?"
//     with the digest recorded on the usage row;
//   - reconciliation still groups by usage_logs.api_key;
//   - the console's key filter still uses
//     "api_key IN (SELECT key FROM api_keys WHERE id = ?)";
//   - existing customers keep working: authentication hashes the presented
//     credential and matches the stored digest.
//
// SQLite has no sha256(), so the mapping is computed in Go: read the plaintext
// keys, update api_keys.key, then rewrite the referencing columns. The function is
// idempotent — rows whose key already is a digest are skipped — so a crash between
// the DDL-less updates and the version record simply replays it.
//
// Operational note: old binaries still compare the presented cleartext against
// these digests, so a rolling restart must not mix old and new replicas for longer
// than the rollout window. This is called out in the release note instead of being
// hidden behind a compatibility shim, which would mean keeping cleartext around.
func migrateAPIKeysToDigests(db *DB) error {
	rows, err := db.DB.Query(`SELECT id, key FROM api_keys`)
	if err != nil {
		return fmt.Errorf("read api keys for digest migration: %w", err)
	}
	type pendingKey struct {
		id     int64
		plain  string
		hashed string
	}
	pending := make([]pendingKey, 0, 64)
	for rows.Next() {
		var id int64
		var key string
		if err := rows.Scan(&id, &key); err != nil {
			rows.Close()
			return fmt.Errorf("scan api key for digest migration: %w", err)
		}
		if key == "" || IsAPIKeyHash(key) {
			continue
		}
		pending = append(pending, pendingKey{id: id, plain: key, hashed: HashAPIKey(key)})
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return fmt.Errorf("iterate api keys for digest migration: %w", err)
	}
	rows.Close()

	// Referencing tables are rewritten only if they exist: a database upgraded from
	// a schema older than the request_traces migration has no such table yet, and
	// failing here would block the upgrade for a table that is created later.
	referencingTables := []string{"usage_logs", "billing_settlements", "request_traces"}
	existing := make(map[string]bool, len(referencingTables))
	for _, table := range referencingTables {
		exists, err := db.tableExists(table)
		if err != nil {
			return fmt.Errorf("probe table %s for digest migration: %w", table, err)
		}
		existing[table] = exists
	}

	if len(pending) == 0 {
		telemetry.ScopedLogger("system").Info("api key digest migration: all stored keys already hashed")
		return nil
	}

	// Rewrite in bounded batches so a large key set does not hold one long write
	// transaction (relevant on the SQLite deployment, where writes are serialized).
	const batchSize = 50
	for start := 0; start < len(pending); start += batchSize {
		end := start + batchSize
		if end > len(pending) {
			end = len(pending)
		}
		tx, err := db.Begin()
		if err != nil {
			return fmt.Errorf("begin api key digest migration: %w", err)
		}
		for _, key := range pending[start:end] {
			if _, err := tx.Exec(`UPDATE api_keys SET key = ? WHERE id = ?`, key.hashed, key.id); err != nil {
				_ = tx.Rollback()
				return fmt.Errorf("rewrite api key %d to digest: %w", key.id, err)
			}
			// The referencing columns are the ledger's join keys; rewrite them with
			// the same digest so nothing needs to join on cleartext afterwards.
			for _, table := range referencingTables {
				if !existing[table] {
					continue
				}
				statement := `UPDATE ` + table + ` SET api_key = ? WHERE api_key = ?`
				if _, err := tx.Exec(statement, key.hashed, key.plain); err != nil {
					_ = tx.Rollback()
					return fmt.Errorf("rewrite key reference in ledger for key %d: %w", key.id, err)
				}
			}
		}
		if err := tx.Commit(); err != nil {
			return fmt.Errorf("commit api key digest migration: %w", err)
		}
	}

	telemetry.ScopedLogger("system").Info("api key digest migration complete",
		"rewritten_keys", len(pending),
		"note", "api_keys.key, usage_logs.api_key, billing_settlements.api_key and request_traces.api_key now hold SHA-256 digests",
	)
	return nil
}
