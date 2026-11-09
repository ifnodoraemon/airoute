package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
)

// GetAPIKeyByKeyContext reads the authoritative credential state, including
// deleted keys so authentication can reject them even before a runtime reload.
// The data-plane auth middleware uses this on every request: without a deadline
// a lock-contended SQLite (or a stalled Postgres) query pins the request forever.
func (r *Repository) GetAPIKeyByKeyContext(ctx context.Context, key string) (*APIKeyRecord, error) {
	return r.getAuthAPIKeyContext(ctx, "key", key)
}

// GetAPIKeyContext reads the selected console key and its owner's current state.
// It includes revoked rows so admission can distinguish revocation from a miss.
func (r *Repository) GetAPIKeyContext(ctx context.Context, id int64) (*APIKeyRecord, error) {
	return r.getAuthAPIKeyContext(ctx, "id", id)
}

// column is chosen only by the two methods above, never by request input.
func (r *Repository) getAuthAPIKeyContext(ctx context.Context, column string, value any) (*APIKeyRecord, error) {
	if r == nil || r.db == nil {
		return nil, sql.ErrNoRows
	}
	// Reader() keeps this per-request auth lookup off the single SQLite write
	// connection, where it used to queue behind settlement flushes. Rebind is still
	// required: the read pool is a raw *sql.DB, so PostgreSQL needs $1.
	row := r.db.Reader().QueryRowContext(ctx, r.db.Rebind(`SELECT k.id, k.public_id, k.key, k.tenant_id, COALESCE(k.allowed_models, ''), k.rpm, k.tpm, k.budget, k.used_tokens, COALESCE(k.used_cost, 0.0), COALESCE(k.budget_units, 0), COALESCE(k.used_cost_units, 0), COALESCE(k.group_name, 'default'), COALESCE(k.user_id, 0), k.status, COALESCE(k.format_validation, ''), COALESCE(k.admin_disabled, FALSE), k.created_at, k.updated_at, COALESCE(u.status, ''), COALESCE(u.role, ''), COALESCE(u.balance, 0.0) FROM api_keys k LEFT JOIN users u ON (k.user_id > 0 AND u.id = k.user_id) OR (COALESCE(k.user_id, 0) = 0 AND k.tenant_id != '' AND u.username = k.tenant_id) WHERE k.`+column+` = ?`), value)
	var rec APIKeyRecord
	var allowedJSON string
	if err := row.Scan(&rec.ID, &rec.PublicID, &rec.Key, &rec.TenantID, &allowedJSON, &rec.RPM, &rec.TPM, &rec.Budget, &rec.UsedTokens, &rec.UsedCost, &rec.BudgetUnits, &rec.UsedCostUnits, &rec.GroupName, &rec.UserID, &rec.Status, &rec.FormatValidation, &rec.AdminDisabled, &rec.CreatedAt, &rec.UpdatedAt, &rec.UserStatus, &rec.UserRole, &rec.UserBalance); err != nil {
		return nil, err
	}
	if allowedJSON != "" {
		_ = json.Unmarshal([]byte(allowedJSON), &rec.AllowedModels)
	}
	rec.resolveMoney()
	return &rec, nil
}

// HasRegisteredAPIKeys includes deleted credentials: revoking the last key must
// never reopen the gateway's unauthenticated development mode. Read failures are
// returned so configuration reloads keep the previous authentication policy.
func (r *Repository) HasRegisteredAPIKeys() (bool, error) {
	if r == nil || r.db == nil {
		return false, fmt.Errorf("API key storage is unavailable")
	}
	var hasKeys bool
	err := r.db.QueryRow(`SELECT EXISTS (SELECT 1 FROM api_keys)`).Scan(&hasKeys)
	return hasKeys, err
}

// CountAPIKeys returns the total number of API keys ever registered, including
// disabled ones. Authentication gates use this instead of CountActiveKeys so a
// deployment whose keys are all disabled does not silently fall back to the
// "no keys configured" open mode.
func (r *Repository) CountAPIKeys() int {
	if r == nil || r.db == nil {
		return 0
	}
	var count int
	_ = r.db.QueryRow(`SELECT COUNT(*) FROM api_keys`).Scan(&count)
	return count
}
