package controlplane

import (
	"time"

	"github.com/ifnodoraemon/airoute/internal/storage"
)

// apiKeyView is the wire representation of an API key.
//
// storage.APIKeyRecord also carries the stored digest. Handing the row straight
// to c.JSON meant the digest left the process and had to be masked on the way
// out, and every new column became an API field by accident. The view names its
// fields explicitly and exposes the public identifier (key_id, minted
// independently of the credential) instead of any credential material.
//
// Key is filled only by the one-time creation response; every other handler
// leaves it empty and `omitempty` keeps it out of the payload.
type apiKeyView struct {
	ID            int64    `json:"id"`
	KeyID         string   `json:"key_id"`
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
	// AdminDisabled tells the console (and API clients) that an administrator
	// revoked this key, so the owner cannot re-enable it. The UI does not consume
	// it yet; it is exposed so the distinction is visible and testable.
	AdminDisabled    bool      `json:"admin_disabled"`
	FormatValidation string    `json:"format_validation,omitempty"`
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt        time.Time `json:"updated_at"`
	Key              string    `json:"key,omitempty"`
}

// apiKeyViewFrom projects a stored row onto the wire representation.
func apiKeyViewFrom(rec *storage.APIKeyRecord) apiKeyView {
	if rec == nil {
		return apiKeyView{}
	}
	return apiKeyView{
		ID:               rec.ID,
		KeyID:            rec.PublicID,
		TenantID:         rec.TenantID,
		AllowedModels:    rec.AllowedModels,
		RPM:              rec.RPM,
		TPM:              rec.TPM,
		Budget:           rec.Budget,
		UsedTokens:       rec.UsedTokens,
		UsedCost:         rec.UsedCost,
		BudgetUnits:      rec.BudgetUnits,
		UsedCostUnits:    rec.UsedCostUnits,
		GroupName:        rec.GroupName,
		UserID:           rec.UserID,
		Status:           rec.Status,
		AdminDisabled:    rec.AdminDisabled,
		FormatValidation: rec.FormatValidation,
		CreatedAt:        rec.CreatedAt,
		UpdatedAt:        rec.UpdatedAt,
	}
}

// apiKeyViews projects a list, preserving order and an empty (non-null) result
// so a caller with no keys receives [] rather than null.
func apiKeyViews(records []*storage.APIKeyRecord) []apiKeyView {
	views := make([]apiKeyView, 0, len(records))
	for _, rec := range records {
		views = append(views, apiKeyViewFrom(rec))
	}
	return views
}
