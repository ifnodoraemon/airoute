package controlplane

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/ifnodoraemon/airoute/internal/storage"
)

// fakeKeyOwnerLookup serves the account lookup the ownership rules depend on.
type fakeKeyOwnerLookup struct {
	users map[string]*storage.UserRecord
	err   error
	calls int

	// keys is the key inventory ListAPIKeysByUser scopes against.
	keys      []*storage.APIKeyRecord
	listErr   error
	listCalls int
}

func (f *fakeKeyOwnerLookup) GetUserByUsername(username string) (*storage.UserRecord, error) {
	f.calls++
	if f.err != nil {
		return nil, f.err
	}
	return f.users[username], nil
}

func (f *fakeKeyOwnerLookup) ListAPIKeysByUser(userID int64) ([]*storage.APIKeyRecord, error) {
	f.listCalls++
	if f.listErr != nil {
		return nil, f.listErr
	}
	var out []*storage.APIKeyRecord
	for _, k := range f.keys {
		if k.UserID == userID {
			out = append(out, k)
		}
	}
	return out, nil
}

func owner(id int64, username string) *storage.UserRecord {
	return &storage.UserRecord{ID: id, Username: username, Role: "user", Status: "active"}
}

// TestKeyAccessRulesCheckOwnership covers the IDOR guard behind UpdateAPIKey and
// DeleteAPIKey. It had no test at all before the rule was extracted, because the
// only way to reach it was through a handler that wrote its own HTTP response.
func TestKeyAccessRulesCheckOwnership(t *testing.T) {
	alice := owner(1, "alice")
	bob := owner(2, "bob")
	aliceKey := &storage.APIKeyRecord{ID: 10, Key: "sk-alice", UserID: alice.ID}
	bobKey := &storage.APIKeyRecord{ID: 11, Key: "sk-bob", UserID: bob.ID}

	cases := []struct {
		name         string
		lookup       *fakeKeyOwnerLookup
		claims       *AdminClaims
		key          *storage.APIKeyRecord
		forbiddenMsg string
		wantAllowed  bool
		wantAdmin    bool
		wantDenial   KeyAccessDenial
		wantMessage  string
	}{
		{
			name:        "an administrator may act on any key",
			lookup:      &fakeKeyOwnerLookup{},
			claims:      &AdminClaims{Username: "root", Role: "admin"},
			key:         bobKey,
			wantAllowed: true,
			wantAdmin:   true,
		},
		{
			name:       "no session is refused",
			lookup:     &fakeKeyOwnerLookup{},
			claims:     nil,
			key:        aliceKey,
			wantDenial: KeyAccessNoSession,
		},
		{
			name:       "an unresolvable account is refused",
			lookup:     &fakeKeyOwnerLookup{users: map[string]*storage.UserRecord{}},
			claims:     &AdminClaims{Username: "ghost", Role: "user"},
			key:        aliceKey,
			wantDenial: KeyAccessUserUnknown,
		},
		{
			name:       "a failed account lookup is refused the same way",
			lookup:     &fakeKeyOwnerLookup{err: errors.New("connection reset")},
			claims:     &AdminClaims{Username: "alice", Role: "user"},
			key:        aliceKey,
			wantDenial: KeyAccessUserUnknown,
		},
		{
			name:         "another tenant's key is refused",
			lookup:       &fakeKeyOwnerLookup{users: map[string]*storage.UserRecord{"alice": alice}},
			claims:       &AdminClaims{Username: "alice", Role: "user"},
			key:          bobKey,
			forbiddenMsg: "无权修改该密钥",
			wantDenial:   KeyAccessNotOwner,
			wantMessage:  "无权修改该密钥",
		},
		{
			name:       "a missing key is refused",
			lookup:     &fakeKeyOwnerLookup{users: map[string]*storage.UserRecord{"alice": alice}},
			claims:     &AdminClaims{Username: "alice", Role: "user"},
			key:        nil,
			wantDenial: KeyAccessNotOwner,
		},
		{
			name:        "the owner may act on their own key",
			lookup:      &fakeKeyOwnerLookup{users: map[string]*storage.UserRecord{"alice": alice}},
			claims:      &AdminClaims{Username: "alice", Role: "user"},
			key:         aliceKey,
			wantAllowed: true,
			wantAdmin:   false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rules := NewKeyAccessRules(tc.lookup)

			got := rules.CheckOwnership(tc.claims, tc.key, tc.forbiddenMsg)

			if got.Allowed != tc.wantAllowed {
				t.Fatalf("Allowed = %v, want %v (denial %v)", got.Allowed, tc.wantAllowed, got.Denial)
			}
			if got.IsAdmin != tc.wantAdmin {
				t.Errorf("IsAdmin = %v, want %v", got.IsAdmin, tc.wantAdmin)
			}
			if !tc.wantAllowed {
				if got.Denial != tc.wantDenial {
					t.Errorf("Denial = %v, want %v", got.Denial, tc.wantDenial)
				}
				if tc.wantMessage != "" && got.Message != tc.wantMessage {
					t.Errorf("Message = %q, want %q", got.Message, tc.wantMessage)
				}
			}
			if tc.wantAllowed && !tc.wantAdmin && got.User == nil {
				t.Error("a non-admin grant must carry the caller's account")
			}
			if tc.wantAllowed && tc.wantAdmin && got.User != nil {
				t.Error("an admin grant must not carry an account")
			}
		})
	}
}

func TestKeyAccessRulesFailsClosedWithoutLookup(t *testing.T) {
	// A nil rule set cannot prove ownership, and the zero value must deny.
	var nilRules *KeyAccessRules
	key := &storage.APIKeyRecord{ID: 1, UserID: 1}
	claims := &AdminClaims{Username: "alice", Role: "user"}

	if got := nilRules.CheckOwnership(claims, key, ""); got.Allowed {
		t.Error("a nil rule set granted access")
	}
	if got := NewKeyAccessRules(nil).CheckOwnership(claims, key, ""); got.Allowed {
		t.Error("a rule set without a lookup granted access")
	}

	// The zero value of the decision type must also deny.
	var zero KeyAccess
	if zero.Allowed {
		t.Error("the zero value of KeyAccess grants access")
	}
	if zero.Denial != KeyAccessNotOwner {
		t.Errorf("the zero denial = %v, want KeyAccessNotOwner", zero.Denial)
	}
}

// TestKeyOwnershipRefusalsKeepTheirWireShape pins the exact status and payload of
// each refusal. The rule was extracted from a handler that wrote these directly,
// so the extraction is only behaviour-preserving while they stay identical.
func TestKeyOwnershipRefusalsKeepTheirWireShape(t *testing.T) {
	cases := []struct {
		name        string
		access      KeyAccess
		wantStatus  int
		wantMessage string
		wantCodeKey bool
	}{
		{
			name:        "no session is a 401 that carries the envelope code",
			access:      KeyAccess{Denial: KeyAccessNoSession},
			wantStatus:  http.StatusUnauthorized,
			wantMessage: "请先登录账号",
			wantCodeKey: true,
		},
		{
			name:        "an unknown account is a 403 without a code",
			access:      KeyAccess{Denial: KeyAccessUserUnknown},
			wantStatus:  http.StatusForbidden,
			wantMessage: "用户不存在",
			wantCodeKey: false,
		},
		{
			name:        "a foreign key keeps the caller-supplied message",
			access:      KeyAccess{Denial: KeyAccessNotOwner, Message: "无权修改该密钥"},
			wantStatus:  http.StatusForbidden,
			wantMessage: "无权修改该密钥",
			wantCodeKey: false,
		},
		{
			name:        "a missing message falls back to the default",
			access:      KeyAccess{Denial: KeyAccessNotOwner},
			wantStatus:  http.StatusForbidden,
			wantMessage: "无权操作该密钥",
			wantCodeKey: false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)

			writeKeyAccessDenial(c, tc.access)

			if w.Code != tc.wantStatus {
				t.Fatalf("status = %d, want %d", w.Code, tc.wantStatus)
			}
			body := decodeBody(t, w)
			if msg, _ := body["error"].(string); msg != tc.wantMessage {
				t.Errorf("error = %q, want %q", msg, tc.wantMessage)
			}
			_, hasCode := body["code"]
			if hasCode != tc.wantCodeKey {
				t.Errorf("code present = %v, want %v (body: %s)", hasCode, tc.wantCodeKey, w.Body.String())
			}
		})
	}
}

// TestKeyAccessRulesFilterAuthorizedIDs covers the batch scoping rule, which used
// to live in a handler that wrote its own responses.
func TestKeyAccessRulesFilterAuthorizedIDs(t *testing.T) {
	alice := owner(1, "alice")
	bob := owner(2, "bob")
	aliceKeys := []*storage.APIKeyRecord{
		{ID: 10, Key: "sk-a1", UserID: alice.ID},
		{ID: 11, Key: "sk-a2", UserID: alice.ID},
	}
	bobKey := &storage.APIKeyRecord{ID: 20, Key: "sk-b1", UserID: bob.ID}

	cases := []struct {
		name       string
		lookup     *fakeKeyOwnerLookup
		claims     *AdminClaims
		requested  []int64
		wantIDs    []int64
		wantDenied bool
		wantDenial KeyAccessDenial
	}{
		{
			name:      "an administrator may touch every requested key",
			lookup:    &fakeKeyOwnerLookup{},
			claims:    &AdminClaims{Username: "root", Role: "admin"},
			requested: []int64{10, 20},
			wantIDs:   []int64{10, 20},
		},
		{
			// The regression: this used to return every requested id as
			// authorised, the opposite of what the single-key rule does.
			name:       "no session is refused instead of authorised",
			lookup:     &fakeKeyOwnerLookup{},
			claims:     nil,
			requested:  []int64{10, 20},
			wantDenied: true,
			wantDenial: KeyAccessNoSession,
		},
		{
			name:       "an unresolvable account is refused",
			lookup:     &fakeKeyOwnerLookup{users: map[string]*storage.UserRecord{}},
			claims:     &AdminClaims{Username: "ghost", Role: "user"},
			requested:  []int64{10},
			wantDenied: true,
			wantDenial: KeyAccessUserUnknown,
		},
		{
			name:       "a failed key lookup is reported as such",
			lookup:     &fakeKeyOwnerLookup{users: map[string]*storage.UserRecord{"alice": alice}, listErr: errors.New("connection reset")},
			claims:     &AdminClaims{Username: "alice", Role: "user"},
			requested:  []int64{10},
			wantDenied: true,
			wantDenial: KeyAccessLookupFailed,
		},
		{
			name:      "a non-admin is scoped to their own keys",
			lookup:    &fakeKeyOwnerLookup{users: map[string]*storage.UserRecord{"alice": alice}, keys: append(aliceKeys, bobKey)},
			claims:    &AdminClaims{Username: "alice", Role: "user"},
			requested: []int64{10, 20, 11},
			wantIDs:   []int64{10, 11},
		},
		{
			name:       "a selection with none of the caller's keys is refused",
			lookup:     &fakeKeyOwnerLookup{users: map[string]*storage.UserRecord{"alice": alice}, keys: append(aliceKeys, bobKey)},
			claims:     &AdminClaims{Username: "alice", Role: "user"},
			requested:  []int64{20, 999},
			wantDenied: true,
			wantDenial: KeyAccessNotOwner,
		},
		{
			name:       "a caller with no keys at all is refused",
			lookup:     &fakeKeyOwnerLookup{users: map[string]*storage.UserRecord{"alice": alice}},
			claims:     &AdminClaims{Username: "alice", Role: "user"},
			requested:  []int64{10},
			wantDenied: true,
			wantDenial: KeyAccessNotOwner,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rules := NewKeyAccessRules(tc.lookup)

			ids, access := rules.FilterAuthorizedIDs(tc.claims, tc.requested)

			if access.Allowed == tc.wantDenied {
				t.Fatalf("Allowed = %v, want %v (denial %v)", access.Allowed, !tc.wantDenied, access.Denial)
			}
			if tc.wantDenied {
				if access.Denial != tc.wantDenial {
					t.Errorf("Denial = %v, want %v", access.Denial, tc.wantDenial)
				}
				if ids != nil {
					t.Errorf("a refused selection returned ids: %v", ids)
				}
				return
			}
			if len(ids) != len(tc.wantIDs) {
				t.Fatalf("ids = %v, want %v", ids, tc.wantIDs)
			}
			for i := range ids {
				if ids[i] != tc.wantIDs[i] {
					t.Fatalf("ids = %v, want %v", ids, tc.wantIDs)
				}
			}
		})
	}
}

func TestKeyAccessRulesFilterFailsClosedWithoutLookup(t *testing.T) {
	var nilRules *KeyAccessRules
	claims := &AdminClaims{Username: "alice", Role: "user"}

	if _, access := nilRules.FilterAuthorizedIDs(claims, []int64{1}); access.Allowed {
		t.Error("a nil rule set authorised a batch")
	}
	if _, access := NewKeyAccessRules(nil).FilterAuthorizedIDs(claims, []int64{1}); access.Allowed {
		t.Error("a rule set without a lookup authorised a batch")
	}
}

// TestKeyAccessLookupFailureWireShape pins the 500 the batch endpoints returned
// when the key lookup itself failed.
func TestKeyAccessLookupFailureWireShape(t *testing.T) {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	writeKeyAccessDenial(c, KeyAccess{Denial: KeyAccessLookupFailed, Message: "connection reset"})

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", w.Code)
	}
	if msg, _ := decodeBody(t, w)["error"].(string); msg != "connection reset" {
		t.Errorf("error = %q, want the cause", msg)
	}
}

// TestKeyAccessBatchRefusalWireShapes pins the 403 the batch endpoints returned.
func TestKeyAccessBatchRefusalWireShapes(t *testing.T) {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	writeKeyAccessDenial(c, KeyAccess{Denial: KeyAccessNotOwner, Message: "无权操作所选密钥"})

	if w.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", w.Code)
	}
	if msg, _ := decodeBody(t, w)["error"].(string); msg != "无权操作所选密钥" {
		t.Errorf("error = %q", msg)
	}
}
