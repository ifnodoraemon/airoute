package controlplane

import "github.com/ifnodoraemon/airoute/internal/storage"

// KeyAccessDenial names why an ownership check refused a request.
//
// The rule layer reports the reason and the HTTP layer decides the status code
// and payload. That split is the point: the single-key ownership guard used to
// call c.JSON directly, which is why the IDOR check behind UpdateAPIKey and
// DeleteAPIKey had no test at all.
type KeyAccessDenial int

const (
	// KeyAccessNotOwner is the zero value, so a KeyAccess that was never filled in
	// refuses rather than grants.
	KeyAccessNotOwner KeyAccessDenial = iota
	// KeyAccessNoSession means there were no usable claims on the request.
	KeyAccessNoSession
	// KeyAccessUserUnknown means the caller's account could not be resolved.
	KeyAccessUserUnknown
	// KeyAccessLookupFailed means the ownership lookup itself failed.
	KeyAccessLookupFailed
)

// KeyAccess is the outcome of an API key ownership check. The zero value denies.
type KeyAccess struct {
	Allowed bool
	// User is the caller's account, set only for a non-admin caller.
	User *storage.UserRecord
	// IsAdmin reports whether the caller bypassed the ownership check.
	IsAdmin bool
	// Denial is only meaningful when Allowed is false.
	Denial KeyAccessDenial
	// Message is the caller-specific refusal text for KeyAccessNotOwner.
	Message string
}

// KeyOwnerLookup is the narrow read surface the ownership rules need.
type KeyOwnerLookup interface {
	GetUserByUsername(username string) (*storage.UserRecord, error)
	ListAPIKeysByUser(userID int64) ([]*storage.APIKeyRecord, error)
}

// KeyAccessRules decides who may read or mutate a given API key.
//
// An administrator may act on any key. A non-administrator may act only on a key
// they own. A missing session, an unresolvable account and a key owned by someone
// else all fail closed.
type KeyAccessRules struct {
	users KeyOwnerLookup
}

// NewKeyAccessRules wires the ownership rule to its lookup.
func NewKeyAccessRules(users KeyOwnerLookup) *KeyAccessRules {
	return &KeyAccessRules{users: users}
}

// CheckOwnership reports whether claims may act on keyRecord.
//
// A failed account lookup is reported as "user unknown" rather than as a lookup
// failure, because that is what the handler this replaced did: it discarded the
// error and treated any non-result as a missing account. Distinguishing the two
// would change the status code a database outage produces, which is a contract
// change rather than a refactor.
func (r *KeyAccessRules) CheckOwnership(claims *AdminClaims, keyRecord *storage.APIKeyRecord, forbiddenMsg string) KeyAccess {
	if claims == nil {
		return KeyAccess{Denial: KeyAccessNoSession}
	}
	if claims.Role == "admin" {
		return KeyAccess{Allowed: true, IsAdmin: true}
	}

	// A nil rule set has no lookup, so it cannot prove ownership. Fail closed.
	if r == nil || r.users == nil {
		return KeyAccess{Denial: KeyAccessUserUnknown}
	}
	user, _ := r.users.GetUserByUsername(claims.Username)
	if user == nil {
		return KeyAccess{Denial: KeyAccessUserUnknown}
	}
	if keyRecord == nil || keyRecord.UserID != user.ID {
		return KeyAccess{Denial: KeyAccessNotOwner, Message: forbiddenMsg}
	}
	return KeyAccess{Allowed: true, User: user}
}

// FilterAuthorizedIDs scopes a batch operation to the keys the caller may touch.
//
// An administrator may touch every requested key. A non-administrator is scoped to
// their own, and a selection that includes none of them is refused rather than
// silently becoming a no-op.
//
// A missing session used to return every requested id as authorised, which is the
// opposite of what CheckOwnership does with the same condition. Both now fail
// closed. The batch routes sit behind the admin middleware, so the change is
// defence in depth rather than a reachable fix.
func (r *KeyAccessRules) FilterAuthorizedIDs(claims *AdminClaims, requestedIDs []int64) ([]int64, KeyAccess) {
	if claims == nil {
		return nil, KeyAccess{Denial: KeyAccessNoSession}
	}
	if claims.Role == "admin" {
		return requestedIDs, KeyAccess{Allowed: true, IsAdmin: true}
	}

	if r == nil || r.users == nil {
		return nil, KeyAccess{Denial: KeyAccessUserUnknown}
	}
	user, _ := r.users.GetUserByUsername(claims.Username)
	if user == nil {
		return nil, KeyAccess{Denial: KeyAccessUserUnknown}
	}

	userKeys, err := r.users.ListAPIKeysByUser(user.ID)
	if err != nil {
		return nil, KeyAccess{Denial: KeyAccessLookupFailed, Message: err.Error()}
	}

	owned := make(map[int64]bool, len(userKeys))
	for _, k := range userKeys {
		owned[k.ID] = true
	}

	var filtered []int64
	for _, id := range requestedIDs {
		if owned[id] {
			filtered = append(filtered, id)
		}
	}
	if len(filtered) == 0 {
		return nil, KeyAccess{Denial: KeyAccessNotOwner, Message: "无权操作所选密钥"}
	}
	return filtered, KeyAccess{Allowed: true, User: user}
}
