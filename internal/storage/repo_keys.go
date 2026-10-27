package storage

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
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
	BudgetUnits      int64     `json:"budget_units"`
	UsedCostUnits    int64     `json:"used_cost_units"`
	GroupName        string    `json:"group_name"`
	UserID           int64     `json:"user_id"`
	Status           string    `json:"status"`
	FormatValidation string    `json:"format_validation,omitempty"`
	UserStatus       string    `json:"-"`
	UserRole         string    `json:"-"`
	UserBalance      float64   `json:"-"`
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt        time.Time `json:"updated_at"`
	// Fingerprint is a stable, non-reversible identifier for Key. It lets audit
	// rows, exports and support tickets refer to a credential without disclosing
	// it, and is the intended join key for a future migration that stops storing
	// plaintext keys (see APIKeyFingerprint).
	Fingerprint string `json:"key_fingerprint,omitempty"`
}

// APIKeyFingerprint returns a stable, non-reversible identifier for an API key.
//
// The key itself has 128 bits of system entropy, so the digest cannot be
// reversed by guessing; it is only ever used to correlate references to a key
// (audit rows, exports, operator support) without copying the credential.
//
// Residual (documented deliberately, not an oversight): api_keys.key,
// usage_logs.api_key and billing_settlements.api_key still hold cleartext
// credentials. Those strings are the ledger's join key — settlement updates
// counters with UPDATE api_keys ... WHERE key = ?, reconciliation groups by
// usage_logs.api_key, and the log console filters by key id through
// api_key IN (SELECT key FROM api_keys WHERE id = ?). Replacing them needs
// (a) a migration adding key_hash plus usage_logs/settlements key identifiers,
// (b) the request-path write sites to record the key id, and (c) settlement and
// reconcile rewritten to join on it. The fingerprint below is the identifier that
// migration should use, so it is added now (additive, no join changes) to make the
// later change a column swap instead of a redesign.
func APIKeyFingerprint(key string) string {
	if key == "" {
		return ""
	}
	sum := sha256.Sum256([]byte("airoute-api-key-fingerprint-v1|" + key))
	return "kfp_" + hex.EncodeToString(sum[:8])
}

// MaskAPIKey renders a credential as a short, non-reusable prefix so logs and
// console rows can tell keys apart without exposing them.
func MaskAPIKey(key string) string {
	if key == "" {
		return ""
	}
	if len(key) <= 12 {
		return "sk-***"
	}
	return key[:8] + "***" + key[len(key)-4:]
}

// attachFingerprint fills the derived identifier for a scanned row.
func (rec *APIKeyRecord) attachFingerprint() {
	if rec != nil {
		rec.Fingerprint = APIKeyFingerprint(rec.Key)
	}
}

// resolveMoney reconciles the integer ledger mirrors with the legacy float
// columns so callers always read a consistent pair.
func (rec *APIKeyRecord) resolveMoney() {
	if rec == nil {
		return
	}
	if rec.UsedCostUnits != 0 || rec.UsedCost == 0 {
		rec.UsedCost = FromUnits(rec.UsedCostUnits)
	} else {
		rec.UsedCostUnits = ToUnits(rec.UsedCost)
	}
	if rec.BudgetUnits != 0 || rec.Budget == 0 {
		rec.Budget = FromUnits(rec.BudgetUnits)
	} else {
		rec.BudgetUnits = ToUnits(rec.Budget)
	}
}

// ListAPIKeys returns all non-deleted API keys for management.
func (r *Repository) ListAPIKeys() ([]*APIKeyRecord, error) {
	rows, err := r.db.Query(`SELECT id, key, tenant_id, COALESCE(allowed_models, ''), rpm, tpm, budget, used_tokens, COALESCE(used_cost, 0.0), COALESCE(budget_units, 0), COALESCE(used_cost_units, 0), COALESCE(group_name, 'default'), COALESCE(user_id, 0), status, COALESCE(format_validation, ''), created_at, updated_at FROM api_keys WHERE status <> 'deleted' ORDER BY id ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	list := make([]*APIKeyRecord, 0)
	for rows.Next() {
		var rec APIKeyRecord
		var allowedJSON string
		err := rows.Scan(&rec.ID, &rec.Key, &rec.TenantID, &allowedJSON, &rec.RPM, &rec.TPM, &rec.Budget, &rec.UsedTokens, &rec.UsedCost, &rec.BudgetUnits, &rec.UsedCostUnits, &rec.GroupName, &rec.UserID, &rec.Status, &rec.FormatValidation, &rec.CreatedAt, &rec.UpdatedAt)
		if err != nil {
			return nil, err
		}
		if allowedJSON != "" {
			_ = json.Unmarshal([]byte(allowedJSON), &rec.AllowedModels)
		}
		rec.resolveMoney()
		rec.attachFingerprint()
		list = append(list, &rec)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return list, nil
}

// ListAPIKeysByUser returns API keys belonging to a specific user.
func (r *Repository) ListAPIKeysByUser(userID int64) ([]*APIKeyRecord, error) {
	rows, err := r.db.Query(`SELECT id, key, tenant_id, COALESCE(allowed_models, ''), rpm, tpm, budget, used_tokens, COALESCE(used_cost, 0.0), COALESCE(budget_units, 0), COALESCE(used_cost_units, 0), COALESCE(group_name, 'default'), COALESCE(user_id, 0), status, COALESCE(format_validation, ''), created_at, updated_at FROM api_keys WHERE user_id = ? AND status <> 'deleted' ORDER BY id ASC`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	list := make([]*APIKeyRecord, 0)
	for rows.Next() {
		var rec APIKeyRecord
		var allowedJSON string
		err := rows.Scan(&rec.ID, &rec.Key, &rec.TenantID, &allowedJSON, &rec.RPM, &rec.TPM, &rec.Budget, &rec.UsedTokens, &rec.UsedCost, &rec.BudgetUnits, &rec.UsedCostUnits, &rec.GroupName, &rec.UserID, &rec.Status, &rec.FormatValidation, &rec.CreatedAt, &rec.UpdatedAt)
		if err != nil {
			return nil, err
		}
		if allowedJSON != "" {
			_ = json.Unmarshal([]byte(allowedJSON), &rec.AllowedModels)
		}
		rec.resolveMoney()
		rec.attachFingerprint()
		list = append(list, &rec)
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
		key.attachFingerprint()
		keys = append(keys, &key)
	}
	return keys, rows.Err()
}

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
	id, err := db.InsertGetID(`INSERT INTO api_keys (key, tenant_id, allowed_models, rpm, tpm, budget, budget_units, used_tokens, used_cost, used_cost_units, group_name, user_id, status, format_validation, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP)`,
		rec.Key, rec.TenantID, string(allowedBytes), rec.RPM, rec.TPM, rec.Budget, ToUnits(rec.Budget), rec.UsedTokens, rec.UsedCost, ToUnits(rec.UsedCost), rec.GroupName, rec.UserID, rec.Status, rec.FormatValidation)
	if err != nil {
		return err
	}
	rec.ID = id
	rec.attachFingerprint()
	return nil
}

// DeleteAPIKey revokes a key permanently while retaining its billing ownership.
func (r *Repository) DeleteAPIKey(id int64) error {
	_, err := r.db.Exec(`UPDATE api_keys SET status = 'deleted', updated_at = CURRENT_TIMESTAMP WHERE id=?`, id)
	return err
}

// GetAPIKey returns a single API key by ID.
func (r *Repository) GetAPIKey(id int64) (*APIKeyRecord, error) {
	row := r.db.QueryRow(`SELECT id, key, tenant_id, COALESCE(allowed_models, ''), rpm, tpm, budget, used_tokens, COALESCE(used_cost, 0.0), COALESCE(budget_units, 0), COALESCE(used_cost_units, 0), COALESCE(group_name, 'default'), COALESCE(user_id, 0), status, COALESCE(format_validation, ''), created_at, updated_at FROM api_keys WHERE id = ? AND status <> 'deleted'`, id)
	var rec APIKeyRecord
	var allowedJSON string
	if err := row.Scan(&rec.ID, &rec.Key, &rec.TenantID, &allowedJSON, &rec.RPM, &rec.TPM, &rec.Budget, &rec.UsedTokens, &rec.UsedCost, &rec.BudgetUnits, &rec.UsedCostUnits, &rec.GroupName, &rec.UserID, &rec.Status, &rec.FormatValidation, &rec.CreatedAt, &rec.UpdatedAt); err != nil {
		return nil, err
	}
	if allowedJSON != "" {
		_ = json.Unmarshal([]byte(allowedJSON), &rec.AllowedModels)
	}
	rec.resolveMoney()
	rec.attachFingerprint()
	return &rec, nil
}

// GetAPIKeyByKey returns a single API key by key string.
func (r *Repository) GetAPIKeyByKey(key string) (*APIKeyRecord, error) {
	rec, err := r.GetAPIKeyByKeyContext(context.Background(), key)
	if err == nil && rec.Status == "deleted" {
		return nil, sql.ErrNoRows
	}
	return rec, err
}

// GetAPIKeyByKeyContext reads the authoritative credential state, including
// deleted keys so authentication can reject them even before a runtime reload.
// The data-plane auth middleware uses this on every request: without a deadline
// a lock-contended SQLite (or a stalled Postgres) query pins the request forever.
func (r *Repository) GetAPIKeyByKeyContext(ctx context.Context, key string) (*APIKeyRecord, error) {
	if r == nil || r.db == nil {
		return nil, sql.ErrNoRows
	}
	row := r.db.QueryRowContext(ctx, `SELECT k.id, k.key, k.tenant_id, COALESCE(k.allowed_models, ''), k.rpm, k.tpm, k.budget, k.used_tokens, COALESCE(k.used_cost, 0.0), COALESCE(k.budget_units, 0), COALESCE(k.used_cost_units, 0), COALESCE(k.group_name, 'default'), COALESCE(k.user_id, 0), k.status, COALESCE(k.format_validation, ''), k.created_at, k.updated_at, COALESCE(u.status, ''), COALESCE(u.role, ''), COALESCE(u.balance, 0.0) FROM api_keys k LEFT JOIN users u ON (k.user_id > 0 AND u.id = k.user_id) OR (COALESCE(k.user_id, 0) = 0 AND k.tenant_id != '' AND u.username = k.tenant_id) WHERE k.key = ?`, key)
	var rec APIKeyRecord
	var allowedJSON string
	if err := row.Scan(&rec.ID, &rec.Key, &rec.TenantID, &allowedJSON, &rec.RPM, &rec.TPM, &rec.Budget, &rec.UsedTokens, &rec.UsedCost, &rec.BudgetUnits, &rec.UsedCostUnits, &rec.GroupName, &rec.UserID, &rec.Status, &rec.FormatValidation, &rec.CreatedAt, &rec.UpdatedAt, &rec.UserStatus, &rec.UserRole, &rec.UserBalance); err != nil {
		return nil, err
	}
	if allowedJSON != "" {
		_ = json.Unmarshal([]byte(allowedJSON), &rec.AllowedModels)
	}
	rec.resolveMoney()
	rec.attachFingerprint()
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
	_, err := r.db.Exec(`UPDATE api_keys SET tenant_id = ?, allowed_models = ?, rpm = ?, tpm = ?, budget = ?, budget_units = ?, group_name = ?, user_id = ?, status = ?, format_validation = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ? AND status <> 'deleted'`,
		rec.TenantID, string(allowedBytes), rec.RPM, rec.TPM, rec.Budget, ToUnits(rec.Budget), rec.GroupName, rec.UserID, rec.Status, rec.FormatValidation, rec.ID)
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
	q := fmt.Sprintf(`UPDATE api_keys SET status = 'deleted', updated_at = CURRENT_TIMESTAMP WHERE status <> 'deleted' AND id IN (%s)`, strings.Join(placeholders, ","))
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
	q := fmt.Sprintf(`UPDATE api_keys SET status = ?, updated_at = CURRENT_TIMESTAMP WHERE status <> 'deleted' AND id IN (%s)`, strings.Join(placeholders, ","))
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

// ToModelAPIKeys converts database APIKeyRecords to Data Plane model.APIKeyConfigs with full in-memory quota snapshots.
func (r *Repository) ToModelAPIKeys() ([]model.APIKeyConfig, error) {
	records, err := r.ListAPIKeys()
	if err != nil {
		return nil, err
	}

	userMap := make(map[int64]*UserRecord)
	userByName := make(map[string]*UserRecord)
	if users, err := r.ListUsers(); err == nil {
		for _, u := range users {
			userMap[u.ID] = u
			userByName[strings.ToLower(u.Username)] = u
		}
	}

	var res []model.APIKeyConfig
	for _, rec := range records {
		if rec.Status != "active" {
			continue
		}

		var user *UserRecord
		if rec.UserID > 0 {
			user = userMap[rec.UserID]
		}
		if user == nil && rec.TenantID != "" {
			user = userByName[strings.ToLower(rec.TenantID)]
		}

		userStatus := ""
		userRole := ""
		userBalance := 0.0
		if user != nil {
			userStatus = user.Status
			userRole = user.Role
			userBalance = user.Balance
		}

		res = append(res, model.APIKeyConfig{
			Key:              rec.Key,
			TenantID:         rec.TenantID,
			AllowedModels:    rec.AllowedModels,
			RPM:              rec.RPM,
			TPM:              rec.TPM,
			Budget:           rec.Budget,
			UsedCost:         rec.UsedCost,
			BudgetUnits:      rec.BudgetUnits,
			UsedCostUnits:    rec.UsedCostUnits,
			Status:           rec.Status,
			GroupName:        rec.GroupName,
			UserID:           rec.UserID,
			FormatValidation: rec.FormatValidation,
			UserStatus:       userStatus,
			UserRole:         userRole,
			UserBalance:      userBalance,
		})
	}
	return res, nil
}
