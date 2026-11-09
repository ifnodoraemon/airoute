package storage

import (
	"encoding/json"
	"fmt"
	"time"
)

// CreateAPIKey inserts a new API key.
func (r *Repository) CreateAPIKey(rec *APIKeyRecord) error {
	if rec.UserID <= 0 {
		if rec.TenantID != "" {
			if u, _ := r.GetUserByUsername(rec.TenantID); u != nil && u.ID > 0 {
				rec.UserID = u.ID
			}
		}
		if rec.UserID <= 0 {
			if admin, _ := r.GetUserByUsername("admin"); admin != nil && admin.ID > 0 {
				rec.UserID = admin.ID
			}
		}
	}
	return insertAPIKey(r.db, rec)
}

func insertAPIKey(db recordInserter, rec *APIKeyRecord) error {
	allowedBytes, _ := json.Marshal(rec.AllowedModels)
	if rec.Status == "" {
		rec.Status = "active"
	}
	if rec.RPM < 0 {
		rec.RPM = 0
	}
	if rec.GroupName == "" {
		rec.GroupName = "default"
	}
	// The public identifier is minted here, independently of the credential, so a
	// caller never chooses it and every row created by this binary has one.
	if rec.PublicID == "" {
		publicID, err := NewAPIKeyPublicID()
		if err != nil {
			return fmt.Errorf("generate api key public id: %w", err)
		}
		rec.PublicID = publicID
	}
	// Persist only the digest. rec.Key is replaced with it so the in-memory record
	// matches the stored row; callers that need the plaintext for the one-time
	// creation response keep their own copy (see the key creation handlers).
	plaintextKey := rec.Key
	rec.Key = HashAPIKey(plaintextKey)
	id, err := db.InsertGetID(`INSERT INTO api_keys (key, public_id, tenant_id, allowed_models, rpm, tpm, budget, budget_units, used_tokens, used_cost, used_cost_units, group_name, user_id, status, admin_disabled, format_validation, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP)`,
		rec.Key, rec.PublicID, rec.TenantID, string(allowedBytes), rec.RPM, rec.TPM, rec.Budget, ToUnits(rec.Budget), rec.UsedTokens, rec.UsedCost, ToUnits(rec.UsedCost), rec.GroupName, rec.UserID, rec.Status, rec.AdminDisabled, rec.FormatValidation)
	if err != nil {
		rec.Key = plaintextKey
		return err
	}
	rec.ID = id
	// The INSERT lets the database stamp created_at/updated_at. Echo those values
	// back on the in-memory record: the creation endpoints return it as-is, and a
	// zero time reached the console as "0001-01-01T00:00:00Z" until the list
	// refreshed.
	now := time.Now().UTC()
	if rec.CreatedAt.IsZero() {
		rec.CreatedAt = now
	}
	if rec.UpdatedAt.IsZero() {
		rec.UpdatedAt = now
	}
	return nil
}

// DeleteAPIKey revokes a key permanently while retaining its billing ownership.
func (r *Repository) DeleteAPIKey(id int64) error {
	_, err := r.db.Exec(`UPDATE api_keys SET status = 'deleted', updated_at = CURRENT_TIMESTAMP WHERE id=?`, id)
	return err
}

// UpdateAPIKey updates an existing API key (e.g. status, RPM, tenant, allowed models, group, format_validation).
func (r *Repository) UpdateAPIKey(rec *APIKeyRecord) error {
	allowedBytes, _ := json.Marshal(rec.AllowedModels)
	if rec.Status == "" {
		rec.Status = "active"
	}
	if rec.GroupName == "" {
		rec.GroupName = "default"
	}
	_, err := r.db.Exec(`UPDATE api_keys SET tenant_id = ?, allowed_models = ?, rpm = ?, tpm = ?, budget = ?, budget_units = ?, group_name = ?, user_id = ?, status = ?, admin_disabled = ?, format_validation = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ? AND status <> 'deleted'`,
		rec.TenantID, string(allowedBytes), rec.RPM, rec.TPM, rec.Budget, ToUnits(rec.Budget), rec.GroupName, rec.UserID, rec.Status, rec.AdminDisabled, rec.FormatValidation, rec.ID)
	return err
}

// BatchDeleteAPIKeys deletes multiple API keys by their IDs.
func (r *Repository) BatchDeleteAPIKeys(ids []int64) (int64, error) {
	if len(ids) == 0 {
		return 0, nil
	}
	placeholders, args := inClause[int64](nil, ids)
	q := fmt.Sprintf(`UPDATE api_keys SET status = 'deleted', updated_at = CURRENT_TIMESTAMP WHERE status <> 'deleted' AND id IN (%s)`, placeholders)
	res, err := r.db.Exec(q, args...)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// BatchUpdateAPIKeyStatus updates status ('active'/'disabled') for multiple API keys.
//
// This is the self-service path (a tenant toggling their own keys). It must never
// resurrect a key an administrator disabled, so the "active" transition skips rows
// marked admin_disabled: the caller learns how many rows actually changed and can
// report the refusal. The administrator path is BatchUpdateAPIKeyStatusAsAdmin.
func (r *Repository) BatchUpdateAPIKeyStatus(ids []int64, status string) (int64, error) {
	if len(ids) == 0 {
		return 0, nil
	}
	placeholders, args := inClause[int64]([]any{status}, ids)
	q := fmt.Sprintf(`UPDATE api_keys SET status = ?, updated_at = CURRENT_TIMESTAMP WHERE status <> 'deleted' AND id IN (%s)`, placeholders)
	if status == "active" {
		q = fmt.Sprintf(`UPDATE api_keys SET status = ?, updated_at = CURRENT_TIMESTAMP WHERE status <> 'deleted' AND COALESCE(admin_disabled, FALSE) = FALSE AND id IN (%s)`, placeholders)
	}
	res, err := r.db.Exec(q, args...)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// BatchUpdateAPIKeyStatusAsAdmin applies an administrator's bulk status change and
// records the revocation in admin_disabled, so tenants cannot undo it one key at a
// time.
func (r *Repository) BatchUpdateAPIKeyStatusAsAdmin(ids []int64, status string) (int64, error) {
	if len(ids) == 0 {
		return 0, nil
	}
	placeholders, args := inClause[int64]([]any{status, status == "disabled"}, ids)
	q := fmt.Sprintf(`UPDATE api_keys SET status = ?, admin_disabled = ?, updated_at = CURRENT_TIMESTAMP WHERE status <> 'deleted' AND id IN (%s)`, placeholders)
	res, err := r.db.Exec(q, args...)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}
