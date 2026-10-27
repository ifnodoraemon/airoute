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
	OrgID            int64     `json:"org_id"`
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

// MaskStoredAPIKey renders a stored key digest for display. A digest is not a
// credential, so a short prefix/suffix keeps console rows distinguishable; the
// plaintext credential is returned only once, by the creation response.
func MaskStoredAPIKey(stored string) string {
	if stored == "" {
		return ""
	}
	if !IsAPIKeyHash(stored) {
		return MaskAPIKey(stored)
	}
	return stored[:8] + "***" + stored[len(stored)-4:]
}

// APIKeyFingerprint returns a stable, non-reversible display identifier for an API
// key. It accepts either the credential or the stored digest and normalizes to the
// digest first, so the same key has one fingerprint whether it is derived at
// creation, from a database row, or from a presented credential during auth.
//
// The credential has 128 bits of entropy, so the digest cannot be reversed; the
// fingerprint is a further digest of it and is used for console rows, audit lines
// and support references, never for authentication.
func APIKeyFingerprint(keyOrDigest string) string {
	if keyOrDigest == "" {
		return ""
	}
	digest := keyOrDigest
	if !IsAPIKeyHash(digest) {
		digest = HashAPIKey(digest)
	}
	sum := sha256.Sum256([]byte("airoute-api-key-fingerprint-v2|" + digest))
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

// attachFingerprint fills the display identifier for a row read from storage.
//
// The stored value is a digest, so the fingerprint is derived from that digest: it
// is stable for a given credential, non-reversible, and never the credential
// itself. Console rows, exports and audit filters use it to refer to a key.
func (rec *APIKeyRecord) attachFingerprint() {
	if rec == nil {
		return
	}
	if rec.Fingerprint == "" {
		rec.Fingerprint = APIKeyFingerprint(rec.Key)
	}
}

// MaskForDisplay replaces the stored key digest with a display mask so a console
// response never presents the digest as if it were a usable credential. The
// fingerprint stays available for correlation.
func (rec *APIKeyRecord) MaskForDisplay() {
	if rec == nil {
		return
	}
	rec.Key = MaskStoredAPIKey(rec.Key)
	if rec.Fingerprint != "" {
		return
	}
	rec.Fingerprint = APIKeyFingerprint(rec.Key)
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
	rows, err := r.db.Query(`SELECT id, key, tenant_id, COALESCE(allowed_models, ''), rpm, tpm, budget, used_tokens, COALESCE(used_cost, 0.0), COALESCE(budget_units, 0), COALESCE(used_cost_units, 0), COALESCE(group_name, 'default'), COALESCE(user_id, 0), COALESCE(org_id, 0), status, COALESCE(format_validation, ''), created_at, updated_at FROM api_keys WHERE status <> 'deleted' ORDER BY id ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	list := make([]*APIKeyRecord, 0)
	for rows.Next() {
		var rec APIKeyRecord
		var allowedJSON string
		err := rows.Scan(&rec.ID, &rec.Key, &rec.TenantID, &allowedJSON, &rec.RPM, &rec.TPM, &rec.Budget, &rec.UsedTokens, &rec.UsedCost, &rec.BudgetUnits, &rec.UsedCostUnits, &rec.GroupName, &rec.UserID, &rec.OrgID, &rec.Status, &rec.FormatValidation, &rec.CreatedAt, &rec.UpdatedAt)
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
	rows, err := r.db.Query(`SELECT id, key, tenant_id, COALESCE(allowed_models, ''), rpm, tpm, budget, used_tokens, COALESCE(used_cost, 0.0), COALESCE(budget_units, 0), COALESCE(used_cost_units, 0), COALESCE(group_name, 'default'), COALESCE(user_id, 0), COALESCE(org_id, 0), status, COALESCE(format_validation, ''), created_at, updated_at FROM api_keys WHERE user_id = ? AND status <> 'deleted' ORDER BY id ASC`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	list := make([]*APIKeyRecord, 0)
	for rows.Next() {
		var rec APIKeyRecord
		var allowedJSON string
		err := rows.Scan(&rec.ID, &rec.Key, &rec.TenantID, &allowedJSON, &rec.RPM, &rec.TPM, &rec.Budget, &rec.UsedTokens, &rec.UsedCost, &rec.BudgetUnits, &rec.UsedCostUnits, &rec.GroupName, &rec.UserID, &rec.OrgID, &rec.Status, &rec.FormatValidation, &rec.CreatedAt, &rec.UpdatedAt)
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
	if rec.OrgID == 0 {
		// New keys inherit the owner's organization; without this an upgraded
		// gateway would write NULL attribution and only a restart's backfill would
		// repair it.
		if orgID, err := r.UserOrgID(rec.UserID); err == nil {
			rec.OrgID = orgID
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
	// Persist only the digest. rec.Key is replaced with it so the in-memory record
	// matches the stored row; callers that need the plaintext for the one-time
	// creation response keep their own copy (see the key creation handlers).
	plaintextKey := rec.Key
	fingerprint := APIKeyFingerprint(plaintextKey)
	rec.Key = HashAPIKey(plaintextKey)
	id, err := db.InsertGetID(`INSERT INTO api_keys (key, tenant_id, allowed_models, rpm, tpm, budget, budget_units, used_tokens, used_cost, used_cost_units, group_name, user_id, org_id, status, format_validation, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP)`,
		rec.Key, rec.TenantID, string(allowedBytes), rec.RPM, rec.TPM, rec.Budget, ToUnits(rec.Budget), rec.UsedTokens, rec.UsedCost, ToUnits(rec.UsedCost), rec.GroupName, rec.UserID, rec.OrgID, rec.Status, rec.FormatValidation)
	if err != nil {
		rec.Key = plaintextKey
		return err
	}
	rec.ID = id
	rec.Fingerprint = fingerprint
	return nil
}

// DeleteAPIKey revokes a key permanently while retaining its billing ownership.
func (r *Repository) DeleteAPIKey(id int64) error {
	_, err := r.db.Exec(`UPDATE api_keys SET status = 'deleted', updated_at = CURRENT_TIMESTAMP WHERE id=?`, id)
	return err
}

// GetAPIKey returns a single API key by ID.
func (r *Repository) GetAPIKey(id int64) (*APIKeyRecord, error) {
	row := r.db.QueryRow(`SELECT id, key, tenant_id, COALESCE(allowed_models, ''), rpm, tpm, budget, used_tokens, COALESCE(used_cost, 0.0), COALESCE(budget_units, 0), COALESCE(used_cost_units, 0), COALESCE(group_name, 'default'), COALESCE(user_id, 0), COALESCE(org_id, 0), status, COALESCE(format_validation, ''), created_at, updated_at FROM api_keys WHERE id = ? AND status <> 'deleted'`, id)
	var rec APIKeyRecord
	var allowedJSON string
	if err := row.Scan(&rec.ID, &rec.Key, &rec.TenantID, &allowedJSON, &rec.RPM, &rec.TPM, &rec.Budget, &rec.UsedTokens, &rec.UsedCost, &rec.BudgetUnits, &rec.UsedCostUnits, &rec.GroupName, &rec.UserID, &rec.OrgID, &rec.Status, &rec.FormatValidation, &rec.CreatedAt, &rec.UpdatedAt); err != nil {
		return nil, err
	}
	if allowedJSON != "" {
		_ = json.Unmarshal([]byte(allowedJSON), &rec.AllowedModels)
	}
	rec.resolveMoney()
	rec.attachFingerprint()
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
	row := r.db.QueryRowContext(ctx, `SELECT k.id, k.key, k.tenant_id, COALESCE(k.allowed_models, ''), k.rpm, k.tpm, k.budget, k.used_tokens, COALESCE(k.used_cost, 0.0), COALESCE(k.budget_units, 0), COALESCE(k.used_cost_units, 0), COALESCE(k.group_name, 'default'), COALESCE(k.user_id, 0), k.status, COALESCE(k.format_validation, ''), k.created_at, k.updated_at, COALESCE(NULLIF(k.org_id, 0), NULLIF(u.org_id, 0), 0), COALESCE(u.status, ''), COALESCE(u.role, ''), COALESCE(u.balance, 0.0) FROM api_keys k LEFT JOIN users u ON (k.user_id > 0 AND u.id = k.user_id) OR (COALESCE(k.user_id, 0) = 0 AND k.tenant_id != '' AND u.username = k.tenant_id) WHERE k.key = ?`, key)
	var rec APIKeyRecord
	var allowedJSON string
	if err := row.Scan(&rec.ID, &rec.Key, &rec.TenantID, &allowedJSON, &rec.RPM, &rec.TPM, &rec.Budget, &rec.UsedTokens, &rec.UsedCost, &rec.BudgetUnits, &rec.UsedCostUnits, &rec.GroupName, &rec.UserID, &rec.Status, &rec.FormatValidation, &rec.CreatedAt, &rec.UpdatedAt, &rec.OrgID, &rec.UserStatus, &rec.UserRole, &rec.UserBalance); err != nil {
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
			OrgID:            rec.OrgID,
			FormatValidation: rec.FormatValidation,
			UserStatus:       userStatus,
			UserRole:         userRole,
			UserBalance:      userBalance,
		})
	}
	return res, nil
}
