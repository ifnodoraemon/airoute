package storage

import (
	"database/sql"
	"errors"
)

// APIKeyPublicIDsByDigest maps stored key digests to their public identifiers.
//
// Audit columns (usage_logs.api_key, billing_settlements.api_key,
// request_traces.api_key) intentionally persist the digest so the reconciliation
// joins against api_keys.key stay valid. The digest is also what an operator
// cannot correlate with the key list, so read paths resolve it to the ak_…
// identifier the key list and access log already use.
func (r *Repository) APIKeyPublicIDsByDigest(digests []string) (map[string]string, error) {
	if r == nil || len(digests) == 0 {
		return nil, nil
	}
	uniq := make([]string, 0, len(digests))
	seen := make(map[string]struct{}, len(digests))
	for _, d := range digests {
		if d == "" {
			continue
		}
		if _, ok := seen[d]; ok {
			continue
		}
		seen[d] = struct{}{}
		uniq = append(uniq, d)
	}
	out := make(map[string]string, len(uniq))
	const chunk = 200
	for start := 0; start < len(uniq); start += chunk {
		end := min(start+chunk, len(uniq))
		placeholders, args := inClause[string](nil, uniq[start:end])
		rows, err := r.db.Query(`SELECT key, public_id FROM api_keys WHERE COALESCE(public_id, '') <> '' AND key IN (`+placeholders+`)`, args...)
		if err != nil {
			return nil, err
		}
		for rows.Next() {
			var digest, publicID string
			if err := rows.Scan(&digest, &publicID); err != nil {
				rows.Close()
				return nil, err
			}
			out[digest] = publicID
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			return nil, err
		}
		rows.Close()
	}
	return out, nil
}

// APIKeyDigestByPublicID resolves an ak_… identifier back to the digest the
// audit filters compare against. An unknown identifier yields an empty digest.
func (r *Repository) APIKeyDigestByPublicID(publicID string) (string, error) {
	if r == nil || !IsAPIKeyPublicID(publicID) {
		return "", nil
	}
	var digest string
	err := r.db.QueryRow(`SELECT key FROM api_keys WHERE public_id = ?`, publicID).Scan(&digest)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", nil
		}
		return "", err
	}
	return digest, nil
}
