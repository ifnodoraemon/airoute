package controlplane

import (
	"strings"

	"github.com/ifnodoraemon/airoute/internal/storage"
)

const (
	// DefaultUserKeyTPM is the token-per-minute allowance a self-service key gets
	// when the request does not ask for one. The value is a default rather than a
	// cap: a caller may ask for more (up to MaxSelfServiceKeyTPM).
	DefaultUserKeyTPM = 100000
	// DefaultUserKeyRPM is the request-per-minute allowance a self-service key
	// gets when the request does not ask for one.
	//
	// It mirrors the key every account receives at registration (RPM 60). Leaving
	// zero here used to mean "unlimited", so a tenant could mint keys with no rate
	// limit at all; an unlimited key is now an administrator decision, made on the
	// admin key-creation endpoint, not the default of a self-service form.
	DefaultUserKeyRPM = 60
	// MaxSelfServiceKeyRPM caps what a self-service form may request. Values above
	// it are clamped rather than rejected: the effective value is returned in the
	// response, and granting more needs an administrator-created key.
	MaxSelfServiceKeyRPM = 6000
	// MaxSelfServiceKeyTPM caps the token allowance a self-service form may
	// request, for the same reason.
	MaxSelfServiceKeyTPM = 10_000_000
)

// clampSelfServiceRPM applies the self-service RPM policy to one value: a
// non-positive request means "no preference" and gets the safe default, and a
// request above the cap is clamped rather than rejected.
//
// This is the single definition of the policy. It used to be written twice — in
// self-service creation and in the shared creation rules — and key update was
// missing it entirely, so a tenant could PATCH a key to rpm=0 (which the storage
// and routing layers treat as unlimited) and bypass the cap.
func clampSelfServiceRPM(rpm int) int {
	switch {
	case rpm <= 0:
		return DefaultUserKeyRPM
	case rpm > MaxSelfServiceKeyRPM:
		return MaxSelfServiceKeyRPM
	default:
		return rpm
	}
}

// clampSelfServiceTPM is clampSelfServiceRPM for tokens per minute.
func clampSelfServiceTPM(tpm int) int {
	switch {
	case tpm <= 0:
		return DefaultUserKeyTPM
	case tpm > MaxSelfServiceKeyTPM:
		return MaxSelfServiceKeyTPM
	default:
		return tpm
	}
}

// clampSelfServiceQuotas clamps both quotas together, which is what the creation
// paths need: a request that omits or zeroes either value must end up with a
// bounded key.
func clampSelfServiceQuotas(rpm, tpm int) (int, int) {
	return clampSelfServiceRPM(rpm), clampSelfServiceTPM(tpm)
}

// PlanUserKey turns a personal key request into the row to store.
//
// It takes the generated credential rather than minting one, so the rule itself is
// pure and the entropy source stays in the handler.
//
// Rules enforced here:
//
//   - Tenant identity is server-owned. It is the caller's account identity and can
//     never come from the request body: deriving it from the display name let a
//     tenant label its traffic as another tenant (for example "admin") in
//     usage_logs, dashboards and exports. The administrator creation path is
//     unchanged — an operator may still deliberately create a key under another
//     tenant there.
//   - Quotas get safe defaults. Omitted RPM used to stay 0 (unlimited); it now
//     defaults to DefaultUserKeyRPM and is capped at MaxSelfServiceKeyRPM. TPM gets
//     the documented default and the same cap.
func PlanUserKey(user *storage.UserRecord, req CreateUserKeyRequest, generatedKey string) *storage.APIKeyRecord {
	allowedModels := req.AllowedModels
	if len(allowedModels) == 0 {
		// An empty list would deny every model; the documented default is "all".
		allowedModels = []string{"*"}
	}

	rpm, tpm := clampSelfServiceQuotas(req.RPM, req.TPM)

	budget := req.Budget
	if budget < 0 {
		budget = 0
	}

	// Tenant identity is server-owned: a personal key belongs to the authenticated
	// account. Request fields (tenant_id, name) are deliberately ignored here.
	tenantID := ""
	if user != nil {
		tenantID = strings.TrimSpace(user.Username)
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
