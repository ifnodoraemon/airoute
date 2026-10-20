package controlplane

import (
	"testing"
)

// TestPlanUserKeyLimits covers the quota rules behind self-service key creation.
// They were inline in the handler and had no test at all, even though they are the
// only place a personal key's allowance is decided.
func TestPlanUserKeyLimits(t *testing.T) {
	alice := aliceWithGroup("vip")

	cases := []struct {
		name       string
		req        CreateUserKeyRequest
		wantRPM    int
		wantTPM    int
		wantBudget float64
		wantTenant string
		wantModels []string
	}{
		{
			name:       "an empty request gets the documented defaults",
			req:        CreateUserKeyRequest{},
			wantRPM:    0,
			wantTPM:    DefaultUserKeyTPM,
			wantBudget: 0,
			wantTenant: "alice",
			wantModels: []string{"*"},
		},
		{
			name:       "a negative rpm floors at zero",
			req:        CreateUserKeyRequest{RPM: -5},
			wantRPM:    0,
			wantTPM:    DefaultUserKeyTPM,
			wantTenant: "alice",
			wantModels: []string{"*"},
		},
		{
			name:       "a negative budget floors at zero",
			req:        CreateUserKeyRequest{Budget: -12.5},
			wantTPM:    DefaultUserKeyTPM,
			wantTenant: "alice",
			wantModels: []string{"*"},
		},
		{
			name:       "a non-positive tpm falls back to the default",
			req:        CreateUserKeyRequest{TPM: -1},
			wantTPM:    DefaultUserKeyTPM,
			wantTenant: "alice",
			wantModels: []string{"*"},
		},
		{
			name:       "positive values are kept as asked",
			req:        CreateUserKeyRequest{RPM: 60, TPM: 500, Budget: 25},
			wantRPM:    60,
			wantTPM:    500,
			wantBudget: 25,
			wantTenant: "alice",
			wantModels: []string{"*"},
		},
		{
			name:       "an explicit model list is kept",
			req:        CreateUserKeyRequest{AllowedModels: []string{"gpt-x"}},
			wantTPM:    DefaultUserKeyTPM,
			wantTenant: "alice",
			wantModels: []string{"gpt-x"},
		},
		{
			name:       "the tenant falls back to the request name",
			req:        CreateUserKeyRequest{Name: "my-app"},
			wantTPM:    DefaultUserKeyTPM,
			wantTenant: "my-app",
			wantModels: []string{"*"},
		},
		{
			name:       "an explicit tenant wins over the name",
			req:        CreateUserKeyRequest{TenantID: "explicit", Name: "my-app"},
			wantTPM:    DefaultUserKeyTPM,
			wantTenant: "explicit",
			wantModels: []string{"*"},
		},
		{
			name:       "a blank tenant and name fall back to the username",
			req:        CreateUserKeyRequest{TenantID: "   ", Name: "  "},
			wantTPM:    DefaultUserKeyTPM,
			wantTenant: "alice",
			wantModels: []string{"*"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := PlanUserKey(alice, tc.req, "sk-airoute-generated")

			if rec.RPM != tc.wantRPM {
				t.Errorf("RPM = %d, want %d", rec.RPM, tc.wantRPM)
			}
			if rec.TPM != tc.wantTPM {
				t.Errorf("TPM = %d, want %d", rec.TPM, tc.wantTPM)
			}
			if rec.Budget != tc.wantBudget {
				t.Errorf("Budget = %v, want %v", rec.Budget, tc.wantBudget)
			}
			if rec.TenantID != tc.wantTenant {
				t.Errorf("TenantID = %q, want %q", rec.TenantID, tc.wantTenant)
			}
			if len(rec.AllowedModels) != len(tc.wantModels) {
				t.Fatalf("AllowedModels = %v, want %v", rec.AllowedModels, tc.wantModels)
			}
			for i := range rec.AllowedModels {
				if rec.AllowedModels[i] != tc.wantModels[i] {
					t.Errorf("AllowedModels = %v, want %v", rec.AllowedModels, tc.wantModels)
				}
			}
		})
	}
}

// TestPlanUserKeyServerOwnedFields pins the fields the caller never chooses.
func TestPlanUserKeyServerOwnedFields(t *testing.T) {
	alice := aliceWithGroup("vip")
	rec := PlanUserKey(alice, CreateUserKeyRequest{}, "sk-airoute-generated")

	if rec.Key != "sk-airoute-generated" {
		t.Errorf("Key = %q, want the generated credential", rec.Key)
	}
	if rec.UserID != alice.ID {
		t.Errorf("UserID = %d, want the session user (%d)", rec.UserID, alice.ID)
	}
	if rec.GroupName != "vip" {
		t.Errorf("GroupName = %q, want the user's own group", rec.GroupName)
	}
	if rec.Status != "active" {
		t.Errorf("Status = %q, want active", rec.Status)
	}
}

// TestPlanUserKeyDoesNotWriteThroughTheCallersSlice covers the aliasing that is
// actually possible: the request is passed by value, so replacing its slice cannot
// reach the caller, but writing into the existing backing array would.
//
// An earlier version of this test asserted that the request struct was unchanged
// after an empty AllowedModels was defaulted. That could never fail — the struct is
// a value — so it proved nothing while looking like coverage.
func TestPlanUserKeyDoesNotWriteThroughTheCallersSlice(t *testing.T) {
	alice := aliceWithGroup("default")
	models := []string{"gpt-x", "gpt-y"}

	rec := PlanUserKey(alice, CreateUserKeyRequest{AllowedModels: models}, "sk-airoute-generated")

	if models[0] != "gpt-x" || models[1] != "gpt-y" {
		t.Errorf("the caller's backing array was modified: %v", models)
	}
	if len(rec.AllowedModels) != 2 || rec.AllowedModels[0] != "gpt-x" || rec.AllowedModels[1] != "gpt-y" {
		t.Errorf("AllowedModels = %v, want the caller's models", rec.AllowedModels)
	}
}

// TestPlanUserKeyToleratesAMissingUser documents the defensive branch: the handler
// resolves the account first, so this should not happen, but it must not panic.
func TestPlanUserKeyToleratesAMissingUser(t *testing.T) {
	rec := PlanUserKey(nil, CreateUserKeyRequest{}, "sk-airoute-generated")

	if rec == nil {
		t.Fatal("PlanUserKey returned nil")
	}
	if rec.UserID != 0 || rec.GroupName != "" {
		t.Errorf("a missing user should leave identity unset: user=%d group=%q", rec.UserID, rec.GroupName)
	}
	if rec.Status != "active" || rec.TPM != DefaultUserKeyTPM {
		t.Errorf("the quota rules should still apply: status=%q tpm=%d", rec.Status, rec.TPM)
	}
}

// TestAdminAndUserPathsDisagreeOnZeroQuotas pins the asymmetry noted on
// PlanUserKey, so a future change to either path has to be deliberate.
func TestAdminAndUserPathsDisagreeOnZeroQuotas(t *testing.T) {
	alice := aliceWithGroup("default")

	personal := PlanUserKey(alice, CreateUserKeyRequest{TPM: 0, RPM: -1, Budget: -1}, "sk-airoute-personal")
	if personal.TPM != DefaultUserKeyTPM || personal.RPM != 0 || personal.Budget != 0 {
		t.Errorf("the personal path did not clamp: tpm=%d rpm=%d budget=%v",
			personal.TPM, personal.RPM, personal.Budget)
	}

	// The administrator path stores what it is given, including zeros.
	rules := NewKeyCreationRules(&fakeKeyOwnerLookup{})
	admin := rules.PlanCreate(nil, CreateAPIKeyRequest{TPM: 0, RPM: 0, Budget: 0})
	if !admin.Allowed {
		t.Fatalf("admin plan refused: %v", admin.Denial)
	}
	if admin.Record.TPM != 0 || admin.Record.RPM != 0 || admin.Record.Budget != 0 {
		t.Errorf("the administrator path changed the values: tpm=%d rpm=%d budget=%v",
			admin.Record.TPM, admin.Record.RPM, admin.Record.Budget)
	}
}
