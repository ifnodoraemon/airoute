package storage

import (
	"context"
	"database/sql"
)

// ListAPIKeys returns all non-deleted API keys for management.
func (r *Repository) ListAPIKeys() ([]*APIKeyRecord, error) {
	rows, err := r.db.Query(`SELECT ` + apiKeyColumns + ` FROM api_keys WHERE status <> 'deleted' ORDER BY id ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	list := make([]*APIKeyRecord, 0)
	for rows.Next() {
		rec, err := scanAPIKey(rows)
		if err != nil {
			return nil, err
		}
		list = append(list, rec)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return list, nil
}

// ListAPIKeysByUser returns API keys belonging to a specific user.
func (r *Repository) ListAPIKeysByUser(userID int64) ([]*APIKeyRecord, error) {
	rows, err := r.db.Query(`SELECT `+apiKeyColumns+` FROM api_keys WHERE user_id = ? AND status <> 'deleted' ORDER BY id ASC`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	list := make([]*APIKeyRecord, 0)
	for rows.Next() {
		rec, err := scanAPIKey(rows)
		if err != nil {
			return nil, err
		}
		list = append(list, rec)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return list, nil
}

// ListHistoricalAPIKeysByUser is for audit scoping only. Deleted keys are kept
// here so revoking credentials does not erase a customer's historical charges.
func (r *Repository) ListHistoricalAPIKeysByUser(userID int64) ([]*APIKeyRecord, error) {
	rows, err := r.db.Query(`SELECT id, key FROM api_keys WHERE user_id = ?`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	keys := make([]*APIKeyRecord, 0)
	for rows.Next() {
		var key APIKeyRecord
		if err := rows.Scan(&key.ID, &key.Key); err != nil {
			return nil, err
		}
		keys = append(keys, &key)
	}
	return keys, rows.Err()
}

// GetAPIKey returns a single API key by ID.
func (r *Repository) GetAPIKey(id int64) (*APIKeyRecord, error) {
	row := r.db.QueryRow(`SELECT `+apiKeyColumns+` FROM api_keys WHERE id = ? AND status <> 'deleted'`, id)
	return scanAPIKey(row)
}

// GetAPIKeyBySecret resolves a presented bearer credential.
//
// The credential is hashed and matched against the stored digest, so the database
// never sees - and never stores - the cleartext value. It is a convenience for
// callers that hold the credential and want the revoked-key policy applied.
//
// The request-path authentication does not use it today: the middleware hashes the
// presented credential itself and calls GetAPIKeyByKeyContext, because it must see
// a deleted row in order to reject it explicitly (GetAPIKeyBySecret maps a revoked
// key to sql.ErrNoRows). GetAPIKeyByKey likewise expects an already hashed
// identifier and is for internal/administrative use.
func (r *Repository) GetAPIKeyBySecret(ctx context.Context, secret string) (*APIKeyRecord, error) {
	if secret == "" {
		return nil, sql.ErrNoRows
	}
	// Revoked keys are not resolvable through the credential-facing API, matching
	// GetAPIKeyByKey. (GetAPIKeyByKeyContext stays unfiltered for the auth path,
	// which needs to see a deleted row in order to reject it explicitly.)
	rec, err := r.GetAPIKeyByKeyContext(ctx, HashAPIKey(secret))
	if err != nil {
		return nil, err
	}
	if rec.Status == "deleted" {
		return nil, sql.ErrNoRows
	}
	return rec, nil
}

// GetAPIKeyByKey returns a single API key by its stored identifier.
//
// The identifier is the SHA-256 digest stored in api_keys.key (see HashAPIKey),
// not the bearer credential. Callers that hold the credential must use
// GetAPIKeyBySecret.
func (r *Repository) GetAPIKeyByKey(key string) (*APIKeyRecord, error) {
	rec, err := r.GetAPIKeyByKeyContext(context.Background(), key)
	if err == nil && rec.Status == "deleted" {
		return nil, sql.ErrNoRows
	}
	return rec, err
}
