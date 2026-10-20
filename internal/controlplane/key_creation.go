package controlplane

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strings"

	"github.com/ifnodoraemon/airoute/internal/storage"
)

// KeyCreationDenial names why a key creation was refused.
type KeyCreationDenial int

const (
	// KeyCreationDenied is the zero value, so an outcome that was never filled in
	// refuses rather than grants.
	KeyCreationDenied KeyCreationDenial = iota
	// KeyCreationAccountUnknown means the caller's account could not be resolved.
	KeyCreationAccountUnknown
	// KeyCreationGroupForbidden means the caller asked for a group they do not have.
	KeyCreationGroupForbidden
	// KeyCreationEntropyUnavailable means a credential could not be generated.
	KeyCreationEntropyUnavailable
)

// KeyCreationOutcome is the result of planning an API key creation. The zero
// value denies.
type KeyCreationOutcome struct {
	Allowed bool
	Record  *storage.APIKeyRecord
	// Denial is only meaningful when Allowed is false.
	Denial  KeyCreationDenial
	Message string
}

// KeyCreationRules owns how a key request becomes a storable row: which fields
// the caller may choose, which the server owns, and when the credential is
// generated rather than accepted.
//
// These rules are what stop a tenant from minting a key for somebody else:
// identity, tenant and group come from the session for a non-administrator, the
// credential is generated unless an administrator supplies a well-formed one, and
// the usage counters and status are server-owned. They used to be inline in the
// handler, where the only way to reach them was through an HTTP request.
type KeyCreationRules struct {
	users   KeyOwnerLookup
	entropy func() (string, error)
}

// NewKeyCreationRules wires the creation rules to their account lookup.
func NewKeyCreationRules(users KeyOwnerLookup) *KeyCreationRules {
	return &KeyCreationRules{users: users, entropy: GenerateAPIKey}
}

// GenerateAPIKey mints a credential from the system entropy source.
//
// This is the single definition of the credential format. It used to be written
// out in five places — admin creation, self-service creation, user creation,
// OAuth signup and registration — each with its own copy of "sk-airoute-" plus 16
// random bytes. The format is a contract: the auth middleware looks keys up by it
// and isWellFormedAPIKey validates it.
func GenerateAPIKey() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return fmt.Sprintf("sk-airoute-%s", hex.EncodeToString(b)), nil
}

// normalizeKeyGroup lower-cases a group name and falls back to "default".
func normalizeKeyGroup(group string) string {
	group = strings.ToLower(strings.TrimSpace(group))
	if group == "" {
		return "default"
	}
	return group
}

// PlanCreate turns a bound request into the row to store, or a refusal.
func (r *KeyCreationRules) PlanCreate(claims *AdminClaims, req CreateAPIKeyRequest) KeyCreationOutcome {
	rec := req.toRecord()
	callerIsAdmin := claims != nil && claims.Role == "admin"

	switch {
	case claims == nil:
		// No session: nothing is known about the caller, so only the group is
		// normalised. The route is behind the admin middleware, so this is
		// defence in depth.
		rec.GroupName = normalizeKeyGroup(rec.GroupName)

	case callerIsAdmin:
		if rec.UserID <= 0 && r != nil && r.users != nil {
			if adminUser, _ := r.users.GetUserByUsername(claims.Username); adminUser != nil {
				rec.UserID = adminUser.ID
			}
		}
		rec.GroupName = normalizeKeyGroup(rec.GroupName)

	default:
		var user *storage.UserRecord
		if r != nil && r.users != nil {
			user, _ = r.users.GetUserByUsername(claims.Username)
		}
		if user == nil {
			return KeyCreationOutcome{Denial: KeyCreationAccountUnknown}
		}

		// Tenant identity and ownership are server-owned. Accepting them from the
		// body let a regular user mint keys under another tenant (for example
		// tenant_id "admin") and attribute usage to somebody else.
		rec.UserID = user.ID
		rec.TenantID = user.Username

		userGroup := normalizeKeyGroup(user.GroupName)
		// An omitted group means "the caller's own group", not "default".
		reqGroup := strings.ToLower(strings.TrimSpace(rec.GroupName))
		if reqGroup == "" {
			reqGroup = userGroup
		}
		if !groupPermitted(reqGroup, userGroup) {
			return KeyCreationOutcome{
				Denial:  KeyCreationGroupForbidden,
				Message: fmt.Sprintf("无权使用分组 [%s]，您的账号保障分组等级为 [%s]", reqGroup, userGroup),
			}
		}
		rec.GroupName = reqGroup
	}

	// The credential is a secret: a non-administrator never chooses it, and an
	// administrator only keeps one that is well formed. Anything else is replaced
	// by a generated value.
	rec.Key = strings.TrimSpace(rec.Key)
	if !callerIsAdmin || len(rec.Key) < 16 || !isWellFormedAPIKey(rec.Key) {
		generated, err := r.entropy()
		if err != nil {
			return KeyCreationOutcome{Denial: KeyCreationEntropyUnavailable}
		}
		rec.Key = generated
	}

	// Usage counters are server-owned; ignore client-supplied accounting.
	rec.UsedCost = 0
	rec.UsedTokens = 0
	if rec.Status == "" {
		rec.Status = "active"
	}
	// A non-administrator cannot self-elevate the key's state.
	if !callerIsAdmin {
		rec.Status = "active"
	}
	if rec.TenantID == "" {
		rec.TenantID = "default-app"
	}

	return KeyCreationOutcome{Allowed: true, Record: rec}
}
