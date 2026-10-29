package controlplane

import (
	"fmt"
	"strings"

	"github.com/ifnodoraemon/airoute/internal/storage"
)

// validToggleStatus reports whether status is one of the two states the data
// plane understands. Anything else is not stored: the admission check in
// internal/middleware only admits "" and "active", so an unrecognised value would
// silently revoke a key (and a typo in a disable command would silently enable
// one). See BUG-CP-03.
func validToggleStatus(status string) bool {
	return status == "active" || status == "disabled"
}

// UpdateAPIKeyRequest is the wire shape of an API key update.
//
// Every field is a pointer (or a nilable slice) so an omitted field means "leave
// unchanged" while an explicit zero still applies. It used to be an anonymous
// struct declared inside the handler, which also kept it out of the pinned
// contract in contract_test.go.
type UpdateAPIKeyRequest struct {
	TenantID         *string  `json:"tenant_id"`
	AllowedModels    []string `json:"allowed_models"`
	RPM              *int     `json:"rpm"`
	TPM              *int     `json:"tpm"`
	Budget           *float64 `json:"budget"`
	Status           *string  `json:"status"`
	FormatValidation *string  `json:"format_validation"`
	GroupName        *string  `json:"group_name"`
}

// KeyUpdateDenial names why a key update was refused.
type KeyUpdateDenial int

const (
	// KeyUpdateDenied is the zero value, so an outcome that was never filled in
	// refuses rather than grants.
	KeyUpdateDenied KeyUpdateDenial = iota
	// KeyUpdateGroupForbidden means the caller asked for a group they do not have.
	KeyUpdateGroupForbidden
	// KeyUpdateInvalidValue means a provided value is not one the field accepts.
	// Handlers map it to 400 rather than the generic 403.
	KeyUpdateInvalidValue
	// KeyUpdateAdminRevoked means a tenant tried to re-activate a key an
	// administrator disabled. Handlers map it to 403: the caller owns the key, but
	// the decision is not theirs to reverse.
	KeyUpdateAdminRevoked
)

// KeyUpdateOutcome is the result of applying an update to a key row. The zero
// value denies.
type KeyUpdateOutcome struct {
	Allowed bool
	Record  *storage.APIKeyRecord
	// Denial is only meaningful when Allowed is false.
	Denial  KeyUpdateDenial
	Message string
}

// groupPermitted reports whether a caller whose own group is userGroup may use
// target. "default" is always available.
//
// Shared by key creation and key update: the rule is one, only the wording of the
// refusal differs per endpoint.
func groupPermitted(target, userGroup string) bool {
	return target == "default" || target == userGroup
}

// ApplyKeyUpdate merges the provided fields onto existing, or refuses.
//
// It takes the ownership decision rather than re-deriving it, so the two
// authorisation concerns stay in one place. Tenant identity remains server-owned
// for regular users: accepting a new value here would let them re-label a key as
// another tenant (for example "admin").
//
// Quotas for regular callers go through the same clamp as key creation. The
// update endpoint is not admin-only — the session middleware gates it — so
// without that, a tenant could PATCH their own key to rpm=0 (which downstream
// means unlimited) or above the self-service cap, bypassing the policy that
// creation enforces. An administrator keeps 0 = unlimited and arbitrary values:
// issuing an unlimited key is an operator decision, never a tenant one.
func ApplyKeyUpdate(access KeyAccess, existing *storage.APIKeyRecord, req UpdateAPIKeyRequest) KeyUpdateOutcome {
	if existing == nil {
		return KeyUpdateOutcome{Denial: KeyUpdateDenied}
	}
	// Defence in depth: the caller is expected to have checked ownership already.
	if !access.Allowed {
		return KeyUpdateOutcome{Denial: KeyUpdateDenied}
	}

	if req.TenantID != nil && access.IsAdmin {
		existing.TenantID = *req.TenantID
	}
	if req.AllowedModels != nil {
		existing.AllowedModels = req.AllowedModels
	}
	if req.RPM != nil {
		existing.RPM = *req.RPM
		if !access.IsAdmin {
			existing.RPM = clampSelfServiceRPM(existing.RPM)
		}
	}
	if req.TPM != nil {
		existing.TPM = *req.TPM
		if !access.IsAdmin {
			existing.TPM = clampSelfServiceTPM(existing.TPM)
		}
	}
	if req.Budget != nil {
		existing.Budget = *req.Budget
	}
	if req.Status != nil {
		status := strings.ToLower(strings.TrimSpace(*req.Status))
		if !validToggleStatus(status) {
			return KeyUpdateOutcome{Denial: KeyUpdateInvalidValue, Message: "状态仅支持 active 或 disabled"}
		}
		switch {
		case access.IsAdmin:
			// An administrator's decision owns the durable flag: disabling records
			// that this was an operator revocation, re-enabling clears it.
			existing.Status = status
			existing.AdminDisabled = status == "disabled"
		case status == "active" && existing.AdminDisabled:
			// The tenant owns the key but not the revocation. Without this branch a
			// single PATCH undid an operator's decision (BUG-CP-01).
			return KeyUpdateOutcome{
				Denial:  KeyUpdateAdminRevoked,
				Message: "该密钥已被管理员停用，无法自行启用，请联系管理员",
			}
		default:
			// Self-service active <-> disabled stays available; the administrator's
			// revocation flag is never touched by a tenant.
			existing.Status = status
		}
	}
	if req.FormatValidation != nil {
		// The field is read at request time (internal/config.ResolveValidationLevel),
		// so an unknown level is a client error, not a value to store and forget.
		level, err := normalizeFormatValidation(*req.FormatValidation)
		if err != nil {
			return KeyUpdateOutcome{Denial: KeyUpdateInvalidValue, Message: err.Error()}
		}
		existing.FormatValidation = level
	}
	if req.GroupName != nil {
		targetGroup := normalizeKeyGroup(*req.GroupName)
		if !access.IsAdmin && access.User != nil {
			userGroup := normalizeKeyGroup(access.User.GroupName)
			if !groupPermitted(targetGroup, userGroup) {
				return KeyUpdateOutcome{
					Denial:  KeyUpdateGroupForbidden,
					Message: fmt.Sprintf("无权切换至分组 [%s]，您的账号保障分组等级为 [%s]", targetGroup, userGroup),
				}
			}
		}
		existing.GroupName = targetGroup
	}

	return KeyUpdateOutcome{Allowed: true, Record: existing}
}
