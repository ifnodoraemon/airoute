package controlplane

import (
	"strings"

	"github.com/ifnodoraemon/airoute/internal/storage"
)

// DefaultUserKeyTPM is the token-per-minute allowance a self-service key gets when
// the request does not ask for one.
//
// The value is a default rather than a cap: a caller may ask for more.
const DefaultUserKeyTPM = 100000

// PlanUserKey turns a personal key request into the row to store.
//
// It takes the generated credential rather than minting one, so the rule itself is
// pure and the entropy source stays in the handler.
//
// These limits used to be inline in the handler, where nothing tested them. They
// are the only place a self-service key's quota is decided, and they are not
// mirrored by the administrator path: CreateAPIKey stores whatever rpm, tpm and
// budget it is given, including zero and negative values. That asymmetry is
// deliberate for now — changing the administrator path would change what an
// operator can deliberately configure — but it is worth knowing that the two
// creation paths do not agree on what a zero quota means.
func PlanUserKey(user *storage.UserRecord, req CreateUserKeyRequest, generatedKey string) *storage.APIKeyRecord {
	allowedModels := req.AllowedModels
	if len(allowedModels) == 0 {
		// An empty list would deny every model; the documented default is "all".
		allowedModels = []string{"*"}
	}

	rpm := req.RPM
	if rpm < 0 {
		rpm = 0
	}

	budget := req.Budget
	if budget < 0 {
		budget = 0
	}

	tpm := req.TPM
	if tpm <= 0 {
		tpm = DefaultUserKeyTPM
	}

	// Tenant identity is server-owned: a personal key belongs to the caller, and a
	// blank tenant falls back to their own username.
	tenantID := strings.TrimSpace(req.TenantID)
	if tenantID == "" {
		tenantID = strings.TrimSpace(req.Name)
	}
	if tenantID == "" && user != nil {
		tenantID = user.Username
	}

	rec := &storage.APIKeyRecord{
		Key:           generatedKey,
		TenantID:      tenantID,
		AllowedModels: allowedModels,
		RPM:           rpm,
		TPM:           tpm,
		Budget:        budget,
		Status:        "active",
	}
	if user != nil {
		rec.UserID = user.ID
		rec.GroupName = user.GroupName
	}
	return rec
}
