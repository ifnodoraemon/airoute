package controlplane

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/ifnodoraemon/airoute/internal/storage"
)

func aliceWithGroup(group string) *storage.UserRecord {
	u := owner(1, "alice")
	u.GroupName = group
	return u
}

// TestKeyCreationRulesPlanCreate covers the rules that stop a tenant from minting
// a key for somebody else. They used to be inline in the handler, so the only way
// to reach them was through an HTTP request and none of them had a test.
func TestKeyCreationRulesPlanCreate(t *testing.T) {
	t.Run("a non-admin cannot choose the tenant or the owner", func(t *testing.T) {
		lookup := &fakeKeyOwnerLookup{users: map[string]*storage.UserRecord{"alice": aliceWithGroup("vip")}}
		rules := NewKeyCreationRules(lookup)

		got := rules.PlanCreate(&AdminClaims{Username: "alice", Role: "user"}, CreateAPIKeyRequest{
			UserID:    999,
			TenantID:  "admin",
			GroupName: "vip",
		})

		if !got.Allowed {
			t.Fatalf("refused: %v %q", got.Denial, got.Message)
		}
		if got.Record.UserID != 1 {
			t.Errorf("UserID = %d, want the session user (1)", got.Record.UserID)
		}
		if got.Record.TenantID != "alice" {
			t.Errorf("TenantID = %q, want the session username", got.Record.TenantID)
		}
	})

	t.Run("an omitted group means the caller's own group", func(t *testing.T) {
		lookup := &fakeKeyOwnerLookup{users: map[string]*storage.UserRecord{"alice": aliceWithGroup("vip")}}
		rules := NewKeyCreationRules(lookup)

		got := rules.PlanCreate(&AdminClaims{Username: "alice", Role: "user"}, CreateAPIKeyRequest{})

		if !got.Allowed {
			t.Fatalf("refused: %v", got.Denial)
		}
		if got.Record.GroupName != "vip" {
			t.Errorf("GroupName = %q, want vip (the caller's own group, not default)", got.Record.GroupName)
		}
	})

	t.Run("a group the caller does not have is refused", func(t *testing.T) {
		lookup := &fakeKeyOwnerLookup{users: map[string]*storage.UserRecord{"alice": aliceWithGroup("vip")}}
		rules := NewKeyCreationRules(lookup)

		got := rules.PlanCreate(&AdminClaims{Username: "alice", Role: "user"}, CreateAPIKeyRequest{GroupName: "enterprise"})

		if got.Allowed {
			t.Fatal("a forbidden group was accepted")
		}
		if got.Denial != KeyCreationGroupForbidden {
			t.Fatalf("Denial = %v, want KeyCreationGroupForbidden", got.Denial)
		}
		if !strings.Contains(got.Message, "enterprise") || !strings.Contains(got.Message, "vip") {
			t.Errorf("message %q should name both the requested and the permitted group", got.Message)
		}
	})

	t.Run("the default group is always allowed", func(t *testing.T) {
		lookup := &fakeKeyOwnerLookup{users: map[string]*storage.UserRecord{"alice": aliceWithGroup("vip")}}
		rules := NewKeyCreationRules(lookup)

		got := rules.PlanCreate(&AdminClaims{Username: "alice", Role: "user"}, CreateAPIKeyRequest{GroupName: "default"})

		if !got.Allowed {
			t.Fatalf("refused: %v %q", got.Denial, got.Message)
		}
		if got.Record.GroupName != "default" {
			t.Errorf("GroupName = %q, want default", got.Record.GroupName)
		}
	})

	t.Run("an unresolvable account is refused", func(t *testing.T) {
		rules := NewKeyCreationRules(&fakeKeyOwnerLookup{users: map[string]*storage.UserRecord{}})

		got := rules.PlanCreate(&AdminClaims{Username: "ghost", Role: "user"}, CreateAPIKeyRequest{})

		if got.Allowed || got.Denial != KeyCreationAccountUnknown {
			t.Fatalf("got allowed=%v denial=%v, want KeyCreationAccountUnknown", got.Allowed, got.Denial)
		}
	})

	t.Run("an administrator may choose the owner and the group", func(t *testing.T) {
		lookup := &fakeKeyOwnerLookup{users: map[string]*storage.UserRecord{"root": owner(9, "root")}}
		rules := NewKeyCreationRules(lookup)

		got := rules.PlanCreate(&AdminClaims{Username: "root", Role: "admin"}, CreateAPIKeyRequest{
			UserID: 42, GroupName: "Enterprise",
		})

		if !got.Allowed {
			t.Fatalf("refused: %v", got.Denial)
		}
		if got.Record.UserID != 42 {
			t.Errorf("UserID = %d, want the explicitly chosen 42", got.Record.UserID)
		}
		if got.Record.GroupName != "enterprise" {
			t.Errorf("GroupName = %q, want it lower-cased", got.Record.GroupName)
		}
	})

	t.Run("an administrator defaults the owner to themselves", func(t *testing.T) {
		lookup := &fakeKeyOwnerLookup{users: map[string]*storage.UserRecord{"root": owner(9, "root")}}
		rules := NewKeyCreationRules(lookup)

		got := rules.PlanCreate(&AdminClaims{Username: "root", Role: "admin"}, CreateAPIKeyRequest{})

		if !got.Allowed {
			t.Fatalf("refused: %v", got.Denial)
		}
		if got.Record.UserID != 9 {
			t.Errorf("UserID = %d, want the administrator's own id", got.Record.UserID)
		}
		if got.Record.GroupName != "default" {
			t.Errorf("GroupName = %q, want default", got.Record.GroupName)
		}
	})

	t.Run("no session only normalises the group", func(t *testing.T) {
		rules := NewKeyCreationRules(&fakeKeyOwnerLookup{})

		got := rules.PlanCreate(nil, CreateAPIKeyRequest{GroupName: "  "})

		if !got.Allowed {
			t.Fatalf("refused: %v", got.Denial)
		}
		if got.Record.GroupName != "default" {
			t.Errorf("GroupName = %q, want default", got.Record.GroupName)
		}
		if got.Record.TenantID != "default-app" {
			t.Errorf("TenantID = %q, want default-app", got.Record.TenantID)
		}
	})
}

// TestKeyCreationRulesCredentialHandling pins when the credential is generated
// rather than accepted: a non-administrator never chooses it, and an
// administrator only keeps one that is well formed.
func TestKeyCreationRulesCredentialHandling(t *testing.T) {
	wellFormed := "sk-airoute-0123456789abcdef0123456789abcdef"
	alice := aliceWithGroup("default")

	cases := []struct {
		name        string
		claims      *AdminClaims
		key         string
		wantKeepKey bool
	}{
		{
			name:        "an administrator keeps a well-formed key",
			claims:      &AdminClaims{Username: "root", Role: "admin"},
			key:         wellFormed,
			wantKeepKey: true,
		},
		{
			name:   "an administrator sending a short key gets a generated one",
			claims: &AdminClaims{Username: "root", Role: "admin"},
			key:    "sk-short",
		},
		{
			name:   "an administrator sending a malformed key gets a generated one",
			claims: &AdminClaims{Username: "root", Role: "admin"},
			key:    "sk-airoute-!!!!not-hex!!!!not-hex!!!!",
		},
		{
			name:   "a non-admin never chooses the credential",
			claims: &AdminClaims{Username: "alice", Role: "user"},
			key:    wellFormed,
		},
		{
			name:   "no session never chooses the credential",
			claims: nil,
			key:    wellFormed,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			lookup := &fakeKeyOwnerLookup{users: map[string]*storage.UserRecord{"alice": alice, "root": owner(9, "root")}}
			rules := NewKeyCreationRules(lookup)

			got := rules.PlanCreate(tc.claims, CreateAPIKeyRequest{Key: tc.key})

			if !got.Allowed {
				t.Fatalf("refused: %v", got.Denial)
			}
			if tc.wantKeepKey {
				if got.Record.Key != tc.key {
					t.Errorf("Key = %q, want the supplied key", got.Record.Key)
				}
				return
			}
			if got.Record.Key == tc.key {
				t.Error("the client-supplied credential was kept")
			}
			if !isWellFormedAPIKey(got.Record.Key) {
				t.Errorf("the generated key is not well formed: %q", got.Record.Key)
			}
		})
	}
}

func TestKeyCreationRulesGeneratedKeysAreUnique(t *testing.T) {
	rules := NewKeyCreationRules(&fakeKeyOwnerLookup{})
	seen := map[string]bool{}
	for i := 0; i < 50; i++ {
		got := rules.PlanCreate(nil, CreateAPIKeyRequest{})
		if !got.Allowed {
			t.Fatalf("refused: %v", got.Denial)
		}
		if seen[got.Record.Key] {
			t.Fatalf("duplicate generated key: %q", got.Record.Key)
		}
		seen[got.Record.Key] = true
	}
}

// TestGenerateAPIKeyFormat pins the credential format, which is a contract rather
// than an implementation detail: the auth middleware looks keys up by this string
// and isWellFormedAPIKey validates it.
//
// It is worth pinning now because the format used to be written out in five
// separate places — admin creation, self-service creation, user creation, OAuth
// signup and registration — so a change to any one of them could silently
// produce keys the rest of the system does not recognise.
func TestGenerateAPIKeyFormat(t *testing.T) {
	const (
		prefix    = "sk-airoute-"
		hexLength = 32 // 16 random bytes
	)

	for i := 0; i < 20; i++ {
		key, err := GenerateAPIKey()
		if err != nil {
			t.Fatalf("GenerateAPIKey: %v", err)
		}
		if !strings.HasPrefix(key, prefix) {
			t.Fatalf("key %q does not start with %q", key, prefix)
		}
		body := strings.TrimPrefix(key, prefix)
		if len(body) != hexLength {
			t.Fatalf("key body %q has length %d, want %d", body, len(body), hexLength)
		}
		if strings.ToLower(body) != body {
			t.Errorf("key body %q is not lower-case hex", body)
		}
		if !isWellFormedAPIKey(key) {
			t.Errorf("generated key %q is rejected by isWellFormedAPIKey", key)
		}
		if len(key) != len(prefix)+hexLength {
			t.Errorf("key %q has length %d, want %d", key, len(key), len(prefix)+hexLength)
		}
	}
}

// TestEveryCallerUsesTheSharedGenerator guards the consolidation itself: if a new
// key-minting path is added with its own copy of the format, this fails.
func TestEveryCallerUsesTheSharedGenerator(t *testing.T) {
	// The five paths that mint a key all go through GenerateAPIKey, so a key
	// produced anywhere in the gateway must satisfy the same shape.
	key, err := GenerateAPIKey()
	if err != nil {
		t.Fatalf("GenerateAPIKey: %v", err)
	}

	rules := NewKeyCreationRules(&fakeKeyOwnerLookup{})
	got := rules.PlanCreate(nil, CreateAPIKeyRequest{})
	if !got.Allowed {
		t.Fatalf("refused: %v", got.Denial)
	}

	if !isWellFormedAPIKey(key) || !isWellFormedAPIKey(got.Record.Key) {
		t.Fatal("keys from the two entry points are not the same shape")
	}
	if len(key) != len(got.Record.Key) {
		t.Errorf("key lengths differ: direct=%d planned=%d", len(key), len(got.Record.Key))
	}
}

func TestKeyCreationRulesEntropyFailureIsReported(t *testing.T) {
	rules := NewKeyCreationRules(&fakeKeyOwnerLookup{})
	rules.entropy = func() (string, error) { return "", errors.New("no entropy") }

	got := rules.PlanCreate(nil, CreateAPIKeyRequest{})

	if got.Allowed || got.Denial != KeyCreationEntropyUnavailable {
		t.Fatalf("got allowed=%v denial=%v, want KeyCreationEntropyUnavailable", got.Allowed, got.Denial)
	}
	if got.Record != nil {
		t.Error("a refused creation must not carry a record")
	}
}

func TestKeyCreationRulesServerOwnedFields(t *testing.T) {
	t.Run("usage counters are always zeroed", func(t *testing.T) {
		rules := NewKeyCreationRules(&fakeKeyOwnerLookup{users: map[string]*storage.UserRecord{"root": owner(9, "root")}})

		got := rules.PlanCreate(&AdminClaims{Username: "root", Role: "admin"}, CreateAPIKeyRequest{
			Key: "sk-airoute-0123456789abcdef0123456789abcdef",
		})

		if !got.Allowed {
			t.Fatalf("refused: %v", got.Denial)
		}
		if got.Record.UsedCost != 0 || got.Record.UsedTokens != 0 {
			t.Errorf("usage counters not zeroed: cost=%v tokens=%d", got.Record.UsedCost, got.Record.UsedTokens)
		}
	})

	t.Run("a non-admin cannot choose a non-active status", func(t *testing.T) {
		lookup := &fakeKeyOwnerLookup{users: map[string]*storage.UserRecord{"alice": aliceWithGroup("default")}}
		rules := NewKeyCreationRules(lookup)

		got := rules.PlanCreate(&AdminClaims{Username: "alice", Role: "user"}, CreateAPIKeyRequest{Status: "disabled"})

		if !got.Allowed {
			t.Fatalf("refused: %v", got.Denial)
		}
		if got.Record.Status != "active" {
			t.Errorf("Status = %q, want active", got.Record.Status)
		}
	})

	t.Run("an administrator may choose the status", func(t *testing.T) {
		lookup := &fakeKeyOwnerLookup{users: map[string]*storage.UserRecord{"root": owner(9, "root")}}
		rules := NewKeyCreationRules(lookup)

		got := rules.PlanCreate(&AdminClaims{Username: "root", Role: "admin"}, CreateAPIKeyRequest{Status: "disabled"})

		if !got.Allowed {
			t.Fatalf("refused: %v", got.Denial)
		}
		if got.Record.Status != "disabled" {
			t.Errorf("Status = %q, want disabled", got.Record.Status)
		}
	})
}

func TestKeyCreationOutcomeZeroValueDenies(t *testing.T) {
	var zero KeyCreationOutcome
	if zero.Allowed {
		t.Error("the zero value of KeyCreationOutcome grants creation")
	}
	if zero.Record != nil {
		t.Error("the zero value carries a record")
	}
}

// TestKeyCreationRefusalsKeepTheirWireShape pins the exact status and payload of
// each refusal, which the handler produced before the rules were extracted.
func TestKeyCreationRefusalsKeepTheirWireShape(t *testing.T) {
	cases := []struct {
		name        string
		outcome     KeyCreationOutcome
		wantStatus  int
		wantMessage string
	}{
		{
			name:        "an unknown account is a 401",
			outcome:     KeyCreationOutcome{Denial: KeyCreationAccountUnknown},
			wantStatus:  http.StatusUnauthorized,
			wantMessage: "账户不存在或暂时无法验证，请重新登录",
		},
		{
			name:        "a forbidden group keeps its formatted message",
			outcome:     KeyCreationOutcome{Denial: KeyCreationGroupForbidden, Message: "无权使用分组 [x]，您的账号保障分组等级为 [y]"},
			wantStatus:  http.StatusForbidden,
			wantMessage: "无权使用分组 [x]，您的账号保障分组等级为 [y]",
		},
		{
			name:        "an entropy failure is a 500",
			outcome:     KeyCreationOutcome{Denial: KeyCreationEntropyUnavailable},
			wantStatus:  http.StatusInternalServerError,
			wantMessage: "安全随机源不可用，无法生成密钥",
		},
		{
			name:        "the zero denial is a 403",
			outcome:     KeyCreationOutcome{},
			wantStatus:  http.StatusForbidden,
			wantMessage: "无法创建密钥",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)

			writeKeyCreationDenial(c, tc.outcome)

			if w.Code != tc.wantStatus {
				t.Fatalf("status = %d, want %d", w.Code, tc.wantStatus)
			}
			if msg, _ := decodeBody(t, w)["error"].(string); msg != tc.wantMessage {
				t.Errorf("error = %q, want %q", msg, tc.wantMessage)
			}
		})
	}
}
