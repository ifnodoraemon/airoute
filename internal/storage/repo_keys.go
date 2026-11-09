package storage

import (
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
	Key           string   `json:"-"`
	TenantID      string   `json:"tenant_id"`
	AllowedModels []string `json:"allowed_models"`
	RPM           int      `json:"rpm"`
	TPM           int      `json:"tpm"`
	Budget        float64  `json:"budget"`
	UsedTokens    int64    `json:"used_tokens"`
	UsedCost      float64  `json:"used_cost"`
	BudgetUnits   int64    `json:"budget_units"`
	UsedCostUnits int64    `json:"used_cost_units"`
	GroupName     string   `json:"group_name"`
	UserID        int64    `json:"user_id"`
	Status        string   `json:"status"`
	// AdminDisabled records that an administrator (not the owner) revoked this
	// key. A tenant may still toggle their own key between active and disabled,
	// but may not re-activate one an administrator disabled; admin_disabled is the
	// durable state that decision needs (see migration 45 and BUG-CP-01).
	AdminDisabled    bool      `json:"admin_disabled"`
	FormatValidation string    `json:"format_validation,omitempty"`
	UserStatus       string    `json:"-"`
	UserRole         string    `json:"-"`
	UserBalance      float64   `json:"-"`
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt        time.Time `json:"updated_at"`
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
