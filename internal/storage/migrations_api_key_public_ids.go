package storage

import "fmt"

// addAPIKeyPublicIDs (migration 39) gives every api_keys row the independent
// public identifier the console, logs, exports and support tickets use to name a
// key.
//
// Before this migration the console derived a display identifier from the stored
// digest (the "kfp_…" fingerprint). That value looked like a credential, was
// derived from the secret, and invited operators to paste it as a bearer token.
// The replacement is minted at creation (see NewAPIKeyPublicID) and has no
// relationship to the credential, so it can be shown, logged and exported freely.
//
// Idempotent: the column is added only when missing, only empty identifiers are
// filled, and the index is created IF NOT EXISTS. The unique index is partial so
// a row written by a pre-migration binary (empty public_id, e.g. during a rolling
// restart) cannot block the upgrade or collide with another such row; every row
// created by the current binary always carries an identifier.
func addAPIKeyPublicIDs(db *DB) error {
	if db.Dialect() == "postgres" {
		if _, err := db.DB.Exec(`ALTER TABLE api_keys ADD COLUMN IF NOT EXISTS public_id VARCHAR(64) NOT NULL DEFAULT ''`); err != nil {
			return fmt.Errorf("add api_keys.public_id: %w", err)
		}
	} else if !sqliteColumnExists(db.DB, "api_keys", "public_id") {
		if _, err := db.DB.Exec(`ALTER TABLE api_keys ADD COLUMN public_id TEXT NOT NULL DEFAULT ''`); err != nil {
			return fmt.Errorf("add api_keys.public_id: %w", err)
		}
	}

	ids, err := apiKeyIDsWithoutPublicID(db)
	if err != nil {
		return err
	}
	update := `UPDATE api_keys SET public_id = ? WHERE id = ?`
	if db.Dialect() == "postgres" {
		update = `UPDATE api_keys SET public_id = $1 WHERE id = $2`
	}
	for _, id := range ids {
		publicID, err := NewAPIKeyPublicID()
		if err != nil {
			return fmt.Errorf("generate api key public id: %w", err)
		}
		if _, err := db.DB.Exec(update, publicID, id); err != nil {
			return fmt.Errorf("backfill api key public id for id %d: %w", id, err)
		}
	}

	if _, err := db.DB.Exec(`CREATE UNIQUE INDEX IF NOT EXISTS idx_api_keys_public_id ON api_keys(public_id) WHERE public_id <> ''`); err != nil {
		return fmt.Errorf("create api_keys.public_id index: %w", err)
	}
	return nil
}

// apiKeyIDsWithoutPublicID lists the rows the backfill still has to name. Rows
// are read into memory before updating so the write path never interleaves with
// an open result set on the shared SQLite connection.
func apiKeyIDsWithoutPublicID(db *DB) ([]int64, error) {
	rows, err := db.DB.Query(`SELECT id FROM api_keys WHERE public_id = '' ORDER BY id`)
	if err != nil {
		return nil, fmt.Errorf("read api keys without a public id: %w", err)
	}
	defer rows.Close()

	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("scan api key id: %w", err)
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}
