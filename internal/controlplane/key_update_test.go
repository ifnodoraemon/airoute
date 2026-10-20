package controlplane

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/ifnodoraemon/airoute/internal/storage"
)

func strPtr(s string) *string     { return &s }
func intPtr(i int) *int           { return &i }
func floatPtr(f float64) *float64 { return &f }

func existingKey() *storage.APIKeyRecord {
	return &storage.APIKeyRecord{
		ID: 10, Key: "sk-alice", UserID: 1, TenantID: "alice",
		AllowedModels: []string{"gpt-x"}, RPM: 60, TPM: 100000,
		Budget: 50, Status: "active", GroupName: "vip",
	}
}

// TestApplyKeyUpdatePartialSemantics pins what an omitted field means: every
// field is a pointer so "not provided" and "explicitly zero" stay distinct.
func TestApplyKeyUpdatePartialSemantics(t *testing.T) {
	adminAccess := KeyAccess{Allowed: true, IsAdmin: true}

	t.Run("an empty request changes nothing", func(t *testing.T) {
		before := existingKey()
		got := ApplyKeyUpdate(adminAccess, before, UpdateAPIKeyRequest{})

		if !got.Allowed {
			t.Fatalf("refused: %v", got.Denial)
		}
		if got.Record.TenantID != "alice" || got.Record.RPM != 60 || got.Record.Budget != 50 ||
			got.Record.Status != "active" || got.Record.GroupName != "vip" {
			t.Errorf("an empty request mutated the row: %+v", got.Record)
		}
	})

	t.Run("an explicit zero applies", func(t *testing.T) {
		got := ApplyKeyUpdate(adminAccess, existingKey(), UpdateAPIKeyRequest{
			RPM: intPtr(0), Budget: floatPtr(0), TPM: intPtr(0),
		})

		if !got.Allowed {
			t.Fatalf("refused: %v", got.Denial)
		}
		if got.Record.RPM != 0 || got.Record.Budget != 0 || got.Record.TPM != 0 {
			t.Errorf("explicit zeros were ignored: rpm=%d budget=%v tpm=%d",
				got.Record.RPM, got.Record.Budget, got.Record.TPM)
		}
	})

	t.Run("a nil model list is unchanged and an empty one clears it", func(t *testing.T) {
		kept := ApplyKeyUpdate(adminAccess, existingKey(), UpdateAPIKeyRequest{})
		if len(kept.Record.AllowedModels) != 1 {
			t.Errorf("a nil list changed the models: %v", kept.Record.AllowedModels)
		}

		cleared := ApplyKeyUpdate(adminAccess, existingKey(), UpdateAPIKeyRequest{AllowedModels: []string{}})
		if len(cleared.Record.AllowedModels) != 0 {
			t.Errorf("an empty list did not clear the models: %v", cleared.Record.AllowedModels)
		}
	})

	t.Run("other fields apply", func(t *testing.T) {
		got := ApplyKeyUpdate(adminAccess, existingKey(), UpdateAPIKeyRequest{
			Status: strPtr("disabled"), FormatValidation: strPtr("off"),
			AllowedModels: []string{"a", "b"},
		})

		if !got.Allowed {
			t.Fatalf("refused: %v", got.Denial)
		}
		if got.Record.Status != "disabled" || got.Record.FormatValidation != "off" {
			t.Errorf("fields not applied: status=%q format=%q", got.Record.Status, got.Record.FormatValidation)
		}
		if len(got.Record.AllowedModels) != 2 {
			t.Errorf("models not applied: %v", got.Record.AllowedModels)
		}
	})
}

// TestApplyKeyUpdateTenantIsServerOwned pins the rule that stops a regular user
// re-labelling their own key as another tenant.
func TestApplyKeyUpdateTenantIsServerOwned(t *testing.T) {
	alice := aliceWithGroup("vip")
	userAccess := KeyAccess{Allowed: true, User: alice}

	t.Run("an administrator may re-label the tenant", func(t *testing.T) {
		got := ApplyKeyUpdate(KeyAccess{Allowed: true, IsAdmin: true}, existingKey(),
			UpdateAPIKeyRequest{TenantID: strPtr("someone-else")})

		if !got.Allowed {
			t.Fatalf("refused: %v", got.Denial)
		}
		if got.Record.TenantID != "someone-else" {
			t.Errorf("TenantID = %q, want someone-else", got.Record.TenantID)
		}
	})

	t.Run("a regular user may not", func(t *testing.T) {
		got := ApplyKeyUpdate(userAccess, existingKey(), UpdateAPIKeyRequest{TenantID: strPtr("admin")})

		if !got.Allowed {
			t.Fatalf("refused: %v", got.Denial)
		}
		if got.Record.TenantID != "alice" {
			t.Errorf("a regular user re-labelled the tenant: %q", got.Record.TenantID)
		}
	})
}

// TestApplyKeyUpdateGroupRule covers the rule that used to be duplicated between
// key creation and key update, with only the wording differing.
func TestApplyKeyUpdateGroupRule(t *testing.T) {
	alice := aliceWithGroup("vip")
	userAccess := KeyAccess{Allowed: true, User: alice}
	adminAccess := KeyAccess{Allowed: true, IsAdmin: true}

	t.Run("a regular user may move a key to their own group", func(t *testing.T) {
		got := ApplyKeyUpdate(userAccess, existingKey(), UpdateAPIKeyRequest{GroupName: strPtr("vip")})

		if !got.Allowed {
			t.Fatalf("refused: %v", got.Denial)
		}
		if got.Record.GroupName != "vip" {
			t.Errorf("GroupName = %q, want vip", got.Record.GroupName)
		}
	})

	t.Run("a regular user may move a key to the default group", func(t *testing.T) {
		got := ApplyKeyUpdate(userAccess, existingKey(), UpdateAPIKeyRequest{GroupName: strPtr("default")})

		if !got.Allowed {
			t.Fatalf("refused: %v", got.Denial)
		}
		if got.Record.GroupName != "default" {
			t.Errorf("GroupName = %q, want default", got.Record.GroupName)
		}
	})

	t.Run("a regular user may not use another group", func(t *testing.T) {
		got := ApplyKeyUpdate(userAccess, existingKey(), UpdateAPIKeyRequest{GroupName: strPtr("enterprise")})

		if got.Allowed {
			t.Fatal("a forbidden group was accepted")
		}
		if got.Denial != KeyUpdateGroupForbidden {
			t.Fatalf("Denial = %v, want KeyUpdateGroupForbidden", got.Denial)
		}
		if !strings.Contains(got.Message, "enterprise") || !strings.Contains(got.Message, "vip") {
			t.Errorf("message %q should name both groups", got.Message)
		}
	})

	t.Run("an administrator may use any group", func(t *testing.T) {
		got := ApplyKeyUpdate(adminAccess, existingKey(), UpdateAPIKeyRequest{GroupName: strPtr("Enterprise")})

		if !got.Allowed {
			t.Fatalf("refused: %v", got.Denial)
		}
		if got.Record.GroupName != "enterprise" {
			t.Errorf("GroupName = %q, want it lower-cased", got.Record.GroupName)
		}
	})

	t.Run("a blank group means default", func(t *testing.T) {
		got := ApplyKeyUpdate(adminAccess, existingKey(), UpdateAPIKeyRequest{GroupName: strPtr("   ")})

		if !got.Allowed {
			t.Fatalf("refused: %v", got.Denial)
		}
		if got.Record.GroupName != "default" {
			t.Errorf("GroupName = %q, want default", got.Record.GroupName)
		}
	})
}

func TestApplyKeyUpdateFailsClosed(t *testing.T) {
	// A refused ownership decision, a missing row and the zero value all deny.
	if got := ApplyKeyUpdate(KeyAccess{}, existingKey(), UpdateAPIKeyRequest{Status: strPtr("disabled")}); got.Allowed {
		t.Error("an unowned access decision was accepted")
	}
	if got := ApplyKeyUpdate(KeyAccess{Allowed: true, IsAdmin: true}, nil, UpdateAPIKeyRequest{}); got.Allowed {
		t.Error("a nil row was accepted")
	}
	var zero KeyUpdateOutcome
	if zero.Allowed || zero.Record != nil {
		t.Error("the zero value of KeyUpdateOutcome grants an update")
	}
}

// TestKeyUpdateGroupRefusalKeepsItsWireShape pins the status and message the
// handler produced before the rule was extracted.
func TestKeyUpdateGroupRefusalKeepsItsWireShape(t *testing.T) {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	writeKeyUpdateDenial(c, KeyUpdateOutcome{
		Denial:  KeyUpdateGroupForbidden,
		Message: "无权切换至分组 [x]，您的账号保障分组等级为 [y]",
	})

	if w.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", w.Code)
	}
	if msg, _ := decodeBody(t, w)["error"].(string); msg != "无权切换至分组 [x]，您的账号保障分组等级为 [y]" {
		t.Errorf("error = %q", msg)
	}

	// The zero denial falls back to the ownership message.
	w2 := httptest.NewRecorder()
	c2, _ := gin.CreateTestContext(w2)
	writeKeyUpdateDenial(c2, KeyUpdateOutcome{})
	if w2.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", w2.Code)
	}
	if msg, _ := decodeBody(t, w2)["error"].(string); msg != "无权修改该密钥" {
		t.Errorf("error = %q, want the ownership message", msg)
	}
}

func TestGroupPermittedIsShared(t *testing.T) {
	cases := []struct {
		target, userGroup string
		want              bool
	}{
		{"default", "vip", true},
		{"vip", "vip", true},
		{"enterprise", "vip", false},
		{"vip", "default", false},
		{"default", "default", true},
	}
	for _, tc := range cases {
		if got := groupPermitted(tc.target, tc.userGroup); got != tc.want {
			t.Errorf("groupPermitted(%q, %q) = %v, want %v", tc.target, tc.userGroup, got, tc.want)
		}
	}
}
