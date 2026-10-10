package storage

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/ifnodoraemon/airoute/internal/model"
)

// APIKeyRecord represents the database row for API keys.
type APIKeyRecord struct {
	ID               int64     `json:"id"`
	Key              string    `json:"key"`
	TenantID         string    `json:"tenant_id"`
	AllowedModels    []string  `json:"allowed_models"`
	RPM              int       `json:"rpm"`
	TPM              int       `json:"tpm"`
	Budget           float64   `json:"budget"`
	UsedTokens       int64     `json:"used_tokens"`
	UsedCost         float64   `json:"used_cost"`
	GroupName        string    `json:"group_name"`
	UserID           int64     `json:"user_id"`
	Status           string    `json:"status"`
	FormatValidation string    `json:"format_validation,omitempty"`
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt        time.Time `json:"updated_at"`
}

// ListAPIKeys returns all API keys.
func (r *Repository) ListAPIKeys() ([]*APIKeyRecord, error) {
	rows, err := r.db.Query(`SELECT id, key, tenant_id, allowed_models, rpm, tpm, budget, used_tokens, COALESCE(used_cost, 0.0), COALESCE(group_name, 'default'), COALESCE(user_id, 0), status, COALESCE(format_validation, ''), created_at, updated_at FROM api_keys ORDER BY id ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	list := make([]*APIKeyRecord, 0)
	for rows.Next() {
		var rec APIKeyRecord
		var allowedJSON string
		err := rows.Scan(&rec.ID, &rec.Key, &rec.TenantID, &allowedJSON, &rec.RPM, &rec.TPM, &rec.Budget, &rec.UsedTokens, &rec.UsedCost, &rec.GroupName, &rec.UserID, &rec.Status, &rec.FormatValidation, &rec.CreatedAt, &rec.UpdatedAt)
		if err != nil {
			return nil, err
		}
		if allowedJSON != "" {
			_ = json.Unmarshal([]byte(allowedJSON), &rec.AllowedModels)
		}
		list = append(list, &rec)
	}
	return list, nil
}

// ListAPIKeysByUser returns API keys belonging to a specific user.
func (r *Repository) ListAPIKeysByUser(userID int64) ([]*APIKeyRecord, error) {
	rows, err := r.db.Query(`SELECT id, key, tenant_id, allowed_models, rpm, tpm, budget, used_tokens, COALESCE(used_cost, 0.0), COALESCE(group_name, 'default'), COALESCE(user_id, 0), status, COALESCE(format_validation, ''), created_at, updated_at FROM api_keys WHERE user_id = ? ORDER BY id ASC`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	list := make([]*APIKeyRecord, 0)
	for rows.Next() {
		var rec APIKeyRecord
		var allowedJSON string
		err := rows.Scan(&rec.ID, &rec.Key, &rec.TenantID, &allowedJSON, &rec.RPM, &rec.TPM, &rec.Budget, &rec.UsedTokens, &rec.UsedCost, &rec.GroupName, &rec.UserID, &rec.Status, &rec.FormatValidation, &rec.CreatedAt, &rec.UpdatedAt)
		if err != nil {
			return nil, err
		}
		if allowedJSON != "" {
			_ = json.Unmarshal([]byte(allowedJSON), &rec.AllowedModels)
		}
		list = append(list, &rec)
	}
	return list, nil
}

// CreateAPIKey inserts a new API key.
func (r *Repository) CreateAPIKey(rec *APIKeyRecord) error {
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

	id, err := r.db.InsertGetID(`INSERT INTO api_keys (key, tenant_id, allowed_models, rpm, tpm, budget, used_tokens, used_cost, group_name, user_id, status, format_validation, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP)`,
		rec.Key, rec.TenantID, string(allowedBytes), rec.RPM, rec.TPM, rec.Budget, rec.UsedTokens, rec.UsedCost, rec.GroupName, rec.UserID, rec.Status, rec.FormatValidation)
	if err != nil {
		return err
	}
	rec.ID = id
	return nil
}

// DeleteAPIKey deletes an API key by ID.
func (r *Repository) DeleteAPIKey(id int64) error {
	_, err := r.db.Exec(`DELETE FROM api_keys WHERE id=?`, id)
	return err
}

// GetAPIKey returns a single API key by ID.
func (r *Repository) GetAPIKey(id int64) (*APIKeyRecord, error) {
	row := r.db.QueryRow(`SELECT id, key, tenant_id, allowed_models, rpm, tpm, budget, used_tokens, COALESCE(used_cost, 0.0), COALESCE(group_name, 'default'), COALESCE(user_id, 0), status, COALESCE(format_validation, ''), created_at, updated_at FROM api_keys WHERE id = ?`, id)
	var rec APIKeyRecord
	var allowedJSON string
	if err := row.Scan(&rec.ID, &rec.Key, &rec.TenantID, &allowedJSON, &rec.RPM, &rec.TPM, &rec.Budget, &rec.UsedTokens, &rec.UsedCost, &rec.GroupName, &rec.UserID, &rec.Status, &rec.FormatValidation, &rec.CreatedAt, &rec.UpdatedAt); err != nil {
		return nil, err
	}
	if allowedJSON != "" {
		_ = json.Unmarshal([]byte(allowedJSON), &rec.AllowedModels)
	}
	return &rec, nil
}

// GetAPIKeyByKey returns a single API key by key string.
func (r *Repository) GetAPIKeyByKey(key string) (*APIKeyRecord, error) {
	row := r.db.QueryRow(`SELECT id, key, tenant_id, allowed_models, rpm, tpm, budget, used_tokens, COALESCE(used_cost, 0.0), COALESCE(group_name, 'default'), COALESCE(user_id, 0), status, COALESCE(format_validation, ''), created_at, updated_at FROM api_keys WHERE key = ?`, key)
	var rec APIKeyRecord
	var allowedJSON string
	if err := row.Scan(&rec.ID, &rec.Key, &rec.TenantID, &allowedJSON, &rec.RPM, &rec.TPM, &rec.Budget, &rec.UsedTokens, &rec.UsedCost, &rec.GroupName, &rec.UserID, &rec.Status, &rec.FormatValidation, &rec.CreatedAt, &rec.UpdatedAt); err != nil {
		return nil, err
	}
	if allowedJSON != "" {
		_ = json.Unmarshal([]byte(allowedJSON), &rec.AllowedModels)
	}
	return &rec, nil
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
	_, err := r.db.Exec(`UPDATE api_keys SET tenant_id = ?, allowed_models = ?, rpm = ?, tpm = ?, budget = ?, group_name = ?, user_id = ?, status = ?, format_validation = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?`,
		rec.TenantID, string(allowedBytes), rec.RPM, rec.TPM, rec.Budget, rec.GroupName, rec.UserID, rec.Status, rec.FormatValidation, rec.ID)
	return err
}

// BatchDeleteAPIKeys deletes multiple API keys by their IDs.
func (r *Repository) BatchDeleteAPIKeys(ids []int64) (int64, error) {
	if len(ids) == 0 {
		return 0, nil
	}
	placeholders := make([]string, len(ids))
	args := make([]interface{}, len(ids))
	for i, id := range ids {
		placeholders[i] = "?"
		args[i] = id
	}
	q := fmt.Sprintf(`DELETE FROM api_keys WHERE id IN (%s)`, strings.Join(placeholders, ","))
	res, err := r.db.Exec(q, args...)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// BatchUpdateAPIKeyStatus updates status ('active'/'disabled') for multiple API keys.
func (r *Repository) BatchUpdateAPIKeyStatus(ids []int64, status string) (int64, error) {
	if len(ids) == 0 {
		return 0, nil
	}
	placeholders := make([]string, len(ids))
	args := make([]interface{}, len(ids)+1)
	args[0] = status
	for i, id := range ids {
		placeholders[i] = "?"
		args[i+1] = id
	}
	q := fmt.Sprintf(`UPDATE api_keys SET status = ?, updated_at = CURRENT_TIMESTAMP WHERE id IN (%s)`, strings.Join(placeholders, ","))
	res, err := r.db.Exec(q, args...)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// CountActiveKeys returns the count of active API keys in the database.
func (r *Repository) CountActiveKeys() int {
	if r == nil || r.db == nil {
		return 0
	}
	var count int
	_ = r.db.QueryRow(`SELECT COUNT(*) FROM api_keys WHERE status='active'`).Scan(&count)
	return count
}

// ToModelAPIKeys converts database APIKeyRecords to Data Plane model.APIKeyConfigs.
func (r *Repository) ToModelAPIKeys() ([]model.APIKeyConfig, error) {
	records, err := r.ListAPIKeys()
	if err != nil {
		return nil, err
	}
	var res []model.APIKeyConfig
	for _, rec := range records {
		if rec.Status != "active" {
			continue
		}
		res = append(res, model.APIKeyConfig{
			Key:              rec.Key,
			TenantID:         rec.TenantID,
			AllowedModels:    rec.AllowedModels,
			RPM:              rec.RPM,
			TPM:              rec.TPM,
			Budget:           rec.Budget,
			GroupName:        rec.GroupName,
			UserID:           rec.UserID,
			FormatValidation: rec.FormatValidation,
		})
	}
	return res, nil
}
