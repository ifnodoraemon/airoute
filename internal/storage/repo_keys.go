package storage

import (
	"context"
	"crypto/rand"
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
	ID int64 `json:"id"`
	// PublicID is the key's public identifier (ak_…). It is minted independently
	// of the credential (see NewAPIKeyPublicID) and is what the console, logs,
	// exports and support tickets use to name a key. It never authenticates a
	// call and carries no material from the secret.
	PublicID string `json:"key_id"`
	// Key holds the SHA-256 digest of the credential. It is never serialized: the
	// console receives PublicID instead, and the plaintext credential appears
	// only in the one-time creation response (see controlplane.apiKeyView).
	Key              string    `json:"-"`
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
}

// HashAPIKey returns the value stored for a bearer credential.
//
// Credentials are never persisted in cleartext: api_keys.key holds this digest,
// and the ledger columns that reference a key (usage_logs.api_key,
// billing_settlements.api_key, request_traces.api_key) hold the same digest. That
// keeps every existing join intact - settlement still runs
// "UPDATE api_keys ... WHERE key = ?", reconciliation still groups by
// usage_logs.api_key, and the log console still filters by
// "api_key IN (SELECT key FROM api_keys WHERE id = ?)" - while no readable
// credential exists at rest.
//
// The credential has 128 bits of system entropy, so a plain SHA-256 digest cannot
// be brute-forced and needs no salt (a per-row salt would make the deterministic
// ledger join impossible). Authentication hashes the presented credential and
// compares digests.
func HashAPIKey(secret string) string {
	if secret == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(secret))
	return hex.EncodeToString(sum[:])
}

// IsAPIKeyHash reports whether value has the shape of a stored key digest (64
// lowercase hex characters). The migration uses it to stay idempotent, and the
// ingestion path uses it to refuse legacy cleartext credentials.
func IsAPIKeyHash(value string) bool {
	if len(value) != 64 {
		return false
	}
	for i := 0; i < len(value); i++ {
		c := value[i]
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') {
			return false
		}
	}
	return true
}

const (
	// apiKeyPublicIDPrefix marks a key's public identifier so a value copied out
	// of the console cannot be mistaken for a bearer credential.
	apiKeyPublicIDPrefix = "ak_"
	// apiKeyPublicIDBytes is 80 bits of entropy. Collisions stay negligible at
	// any realistic key count; the partial unique index from migration 39 is the
	// backstop, not the primary defence.
	apiKeyPublicIDBytes = 10
)

// NewAPIKeyPublicID mints the public identifier shown for a key in the console,
// logs, exports and support tickets.
//
// It is deliberately independent of the credential: it is not derived from the
// secret or from its digest, so it reveals nothing about what authenticates a
// call, does not change when the stored digest is replaced, and cannot be
// mistaken for a credential (it is not a digest and never matches a stored key).
func NewAPIKeyPublicID() (string, error) {
	b := make([]byte, apiKeyPublicIDBytes)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return apiKeyPublicIDPrefix + hex.EncodeToString(b), nil
}

// MaskAPIKey renders a credential as a short, non-reusable prefix so audit rows
// can tell unrecognised submissions apart without storing the value.
//
// Console key listings use NewAPIKeyPublicID instead: they never show a fragment
// of a credential, because the fragment is what operators copy by mistake.
func MaskAPIKey(key string) string {
	if key == "" {
		return ""
	}
	if len(key) <= 12 {
		return "sk-***"
	}
	return key[:8] + "***" + key[len(key)-4:]
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

// APIKeyExists reports whether an active/deleted flag allows this id to own a
// budget. It avoids loading the full record just to validate a scope.
func (r *Repository) APIKeyExists(id int64) (bool, error) {
	if id <= 0 {
		return false, nil
	}
	var count int
	if err := r.db.QueryRow(`SELECT COUNT(*) FROM api_keys WHERE id = ? AND status <> 'deleted'`, id).Scan(&count); err != nil {
		return false, err
	}
	return count > 0, nil
}

// ListAPIKeys returns all non-deleted API keys for management.
func (r *Repository) ListAPIKeys() ([]*APIKeyRecord, error) {
	rows, err := r.db.Query(`SELECT id, public_id, key, tenant_id, COALESCE(allowed_models, ''), rpm, tpm, budget, used_tokens, COALESCE(used_cost, 0.0), COALESCE(budget_units, 0), COALESCE(used_cost_units, 0), COALESCE(group_name, 'default'), COALESCE(user_id, 0), status, COALESCE(format_validation, ''), created_at, updated_at FROM api_keys WHERE status <> 'deleted' ORDER BY id ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	list := make([]*APIKeyRecord, 0)
	for rows.Next() {
		var rec APIKeyRecord
		var allowedJSON string
		err := rows.Scan(&rec.ID, &rec.PublicID, &rec.Key, &rec.TenantID, &allowedJSON, &rec.RPM, &rec.TPM, &rec.Budget, &rec.UsedTokens, &rec.UsedCost, &rec.BudgetUnits, &rec.UsedCostUnits, &rec.GroupName, &rec.UserID, &rec.Status, &rec.FormatValidation, &rec.CreatedAt, &rec.UpdatedAt)
		if err != nil {
			return nil, err
		}
		if allowedJSON != "" {
			_ = json.Unmarshal([]byte(allowedJSON), &rec.AllowedModels)
		}
		rec.resolveMoney()
		list = append(list, &rec)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return list, nil
}

// ListAPIKeysByUser returns API keys belonging to a specific user.
func (r *Repository) ListAPIKeysByUser(userID int64) ([]*APIKeyRecord, error) {
	rows, err := r.db.Query(`SELECT id, public_id, key, tenant_id, COALESCE(allowed_models, ''), rpm, tpm, budget, used_tokens, COALESCE(used_cost, 0.0), COALESCE(budget_units, 0), COALESCE(used_cost_units, 0), COALESCE(group_name, 'default'), COALESCE(user_id, 0), status, COALESCE(format_validation, ''), created_at, updated_at FROM api_keys WHERE user_id = ? AND status <> 'deleted' ORDER BY id ASC`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	list := make([]*APIKeyRecord, 0)
	for rows.Next() {
		var rec APIKeyRecord
		var allowedJSON string
		err := rows.Scan(&rec.ID, &rec.PublicID, &rec.Key, &rec.TenantID, &allowedJSON, &rec.RPM, &rec.TPM, &rec.Budget, &rec.UsedTokens, &rec.UsedCost, &rec.BudgetUnits, &rec.UsedCostUnits, &rec.GroupName, &rec.UserID, &rec.Status, &rec.FormatValidation, &rec.CreatedAt, &rec.UpdatedAt)
		if err != nil {
			return nil, err
		}
		if allowedJSON != "" {
			_ = json.Unmarshal([]byte(allowedJSON), &rec.AllowedModels)
		}
		rec.resolveMoney()
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
	id, err := db.InsertGetID(`INSERT INTO api_keys (key, public_id, tenant_id, allowed_models, rpm, tpm, budget, budget_units, used_tokens, used_cost, used_cost_units, group_name, user_id, status, format_validation, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP)`,
		rec.Key, rec.PublicID, rec.TenantID, string(allowedBytes), rec.RPM, rec.TPM, rec.Budget, ToUnits(rec.Budget), rec.UsedTokens, rec.UsedCost, ToUnits(rec.UsedCost), rec.GroupName, rec.UserID, rec.Status, rec.FormatValidation)
	if err != nil {
		rec.Key = plaintextKey
		return err
	}
	rec.ID = id
	return nil
}

// DeleteAPIKey revokes a key permanently while retaining its billing ownership.
func (r *Repository) DeleteAPIKey(id int64) error {
	_, err := r.db.Exec(`UPDATE api_keys SET status = 'deleted', updated_at = CURRENT_TIMESTAMP WHERE id=?`, id)
	return err
}

// GetAPIKey returns a single API key by ID.
func (r *Repository) GetAPIKey(id int64) (*APIKeyRecord, error) {
	row := r.db.QueryRow(`SELECT id, public_id, key, tenant_id, COALESCE(allowed_models, ''), rpm, tpm, budget, used_tokens, COALESCE(used_cost, 0.0), COALESCE(budget_units, 0), COALESCE(used_cost_units, 0), COALESCE(group_name, 'default'), COALESCE(user_id, 0), status, COALESCE(format_validation, ''), created_at, updated_at FROM api_keys WHERE id = ? AND status <> 'deleted'`, id)
	var rec APIKeyRecord
	var allowedJSON string
	if err := row.Scan(&rec.ID, &rec.PublicID, &rec.Key, &rec.TenantID, &allowedJSON, &rec.RPM, &rec.TPM, &rec.Budget, &rec.UsedTokens, &rec.UsedCost, &rec.BudgetUnits, &rec.UsedCostUnits, &rec.GroupName, &rec.UserID, &rec.Status, &rec.FormatValidation, &rec.CreatedAt, &rec.UpdatedAt); err != nil {
		return nil, err
	}
	if allowedJSON != "" {
		_ = json.Unmarshal([]byte(allowedJSON), &rec.AllowedModels)
	}
	rec.resolveMoney()
	return &rec, nil
}

// GetAPIKeyBySecret resolves a presented bearer credential.
//
// The credential is hashed and matched against the stored digest, so the database
// never sees - and never stores - the cleartext value. This is the lookup the
// request-path authentication must use; GetAPIKeyByKey expects an already hashed
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

// GetAPIKeyByKeyContext reads the authoritative credential state, including
// deleted keys so authentication can reject them even before a runtime reload.
// The data-plane auth middleware uses this on every request: without a deadline
// a lock-contended SQLite (or a stalled Postgres) query pins the request forever.
func (r *Repository) GetAPIKeyByKeyContext(ctx context.Context, key string) (*APIKeyRecord, error) {
	if r == nil || r.db == nil {
		return nil, sql.ErrNoRows
	}
	row := r.db.QueryRowContext(ctx, `SELECT k.id, k.public_id, k.key, k.tenant_id, COALESCE(k.allowed_models, ''), k.rpm, k.tpm, k.budget, k.used_tokens, COALESCE(k.used_cost, 0.0), COALESCE(k.budget_units, 0), COALESCE(k.used_cost_units, 0), COALESCE(k.group_name, 'default'), COALESCE(k.user_id, 0), k.status, COALESCE(k.format_validation, ''), k.created_at, k.updated_at, COALESCE(u.status, ''), COALESCE(u.role, ''), COALESCE(u.balance, 0.0) FROM api_keys k LEFT JOIN users u ON (k.user_id > 0 AND u.id = k.user_id) OR (COALESCE(k.user_id, 0) = 0 AND k.tenant_id != '' AND u.username = k.tenant_id) WHERE k.key = ?`, key)
	var rec APIKeyRecord
	var allowedJSON string
	if err := row.Scan(&rec.ID, &rec.PublicID, &rec.Key, &rec.TenantID, &allowedJSON, &rec.RPM, &rec.TPM, &rec.Budget, &rec.UsedTokens, &rec.UsedCost, &rec.BudgetUnits, &rec.UsedCostUnits, &rec.GroupName, &rec.UserID, &rec.Status, &rec.FormatValidation, &rec.CreatedAt, &rec.UpdatedAt, &rec.UserStatus, &rec.UserRole, &rec.UserBalance); err != nil {
		return nil, err
	}
	if allowedJSON != "" {
		_ = json.Unmarshal([]byte(allowedJSON), &rec.AllowedModels)
	}
	rec.resolveMoney()
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

// ToConfig renders the row as the data-plane view of a credential.
//
// It is the single conversion used both when the runtime snapshot is built
// (ToModelAPIKeys) and when the auth middleware has to resolve a credential the
// snapshot does not know yet (see middleware.lookupStorageCredential). Keeping
// one mapping means a fallback lookup cannot authenticate with different
// entitlements than a snapshot lookup would have granted.
//
// UserStatus/UserRole/UserBalance are carried when the row was read through a
// query that joins users (GetAPIKeyByKeyContext does); otherwise they stay empty
// and the caller can fill them in.
func (rec *APIKeyRecord) ToConfig() model.APIKeyConfig {
	if rec == nil {
		return model.APIKeyConfig{}
	}
	return model.APIKeyConfig{
		ID:               rec.ID,
		PublicID:         rec.PublicID,
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
		UserStatus:       rec.UserStatus,
		UserRole:         rec.UserRole,
		UserBalance:      rec.UserBalance,
	}
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

		config := rec.ToConfig()
		// The snapshot resolves the owner from one pass over users rather than a
		// join per key, so it overwrites whatever the row itself carried.
		config.UserStatus = ""
		config.UserRole = ""
		config.UserBalance = 0
		if user != nil {
			config.UserStatus = user.Status
			config.UserRole = user.Role
			config.UserBalance = user.Balance
		}
		res = append(res, config)
	}
	return res, nil
}
