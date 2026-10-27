package controlplane

import (
	"testing"
)

// TestPlanUserKeyLimits covers the quota rules behind self-service key creation.
// They were inline in the handler and had no test at all, even though they are the
// only place a personal key's allowance is decided.
//
// A10/A11: the tenant is server-owned (it is the authenticated account) and an
// omitted RPM is no longer "unlimited".
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
			wantRPM:    DefaultUserKeyRPM,
			wantTPM:    DefaultUserKeyTPM,
			wantBudget: 0,
			wantTenant: "alice",
			wantModels: []string{"*"},
		},
		{
			name:       "a negative rpm falls back to the default instead of unlimited",
			req:        CreateUserKeyRequest{RPM: -5},
			wantRPM:    DefaultUserKeyRPM,
			wantTPM:    DefaultUserKeyTPM,
			wantTenant: "alice",
			wantModels: []string{"*"},
		},
		{
			name:       "a zero rpm means the default, not unlimited",
			req:        CreateUserKeyRequest{RPM: 0},
			wantRPM:    DefaultUserKeyRPM,
			wantTPM:    DefaultUserKeyTPM,
			wantTenant: "alice",
			wantModels: []string{"*"},
		},
		{
			name:       "an rpm above the self-service cap is clamped",
			req:        CreateUserKeyRequest{RPM: MaxSelfServiceKeyRPM + 1},
			wantRPM:    MaxSelfServiceKeyRPM,
			wantTPM:    DefaultUserKeyTPM,
			wantTenant: "alice",
			wantModels: []string{"*"},
		},
		{
			name:       "a negative budget floors at zero",
			req:        CreateUserKeyRequest{Budget: -12.5},
			wantRPM:    DefaultUserKeyRPM,
			wantTPM:    DefaultUserKeyTPM,
			wantTenant: "alice",
			wantModels: []string{"*"},
		},
		{
			name:       "a non-positive tpm falls back to the default",
			req:        CreateUserKeyRequest{TPM: -1},
			wantRPM:    DefaultUserKeyRPM,
			wantTPM:    DefaultUserKeyTPM,
			wantTenant: "alice",
			wantModels: []string{"*"},
		},
		{
			name:       "a tpm above the self-service cap is clamped",
			req:        CreateUserKeyRequest{TPM: MaxSelfServiceKeyTPM + 1},
			wantRPM:    DefaultUserKeyRPM,
			wantTPM:    MaxSelfServiceKeyTPM,
			wantTenant: "alice",
			wantModels: []string{"*"},
		},
		{
			name:       "positive values within the caps are kept as asked",
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
			wantRPM:    DefaultUserKeyRPM,
			wantTPM:    DefaultUserKeyTPM,
			wantTenant: "alice",
			wantModels: []string{"gpt-x"},
		},
		{
			// A10 regression: the display name is client-controlled and must never
			// become the tenant identity.
			name:       "the request name cannot become the tenant",
			req:        CreateUserKeyRequest{Name: "my-app"},
			wantRPM:    DefaultUserKeyRPM,
			wantTPM:    DefaultUserKeyTPM,
			wantTenant: "alice",
			wantModels: []string{"*"},
		},
		{
			// A10 regression: an explicit tenant_id in the body is ignored too.
			name:       "an explicit tenant id cannot override the owner",
			req:        CreateUserKeyRequest{TenantID: "admin", Name: "my-app"},
			wantRPM:    DefaultUserKeyRPM,
			wantTPM:    DefaultUserKeyTPM,
			wantTenant: "alice",
			wantModels: []string{"*"},
		},
		{
			name:       "a blank tenant and name still yield the username",
			req:        CreateUserKeyRequest{TenantID: "   ", Name: "  "},
			wantRPM:    DefaultUserKeyRPM,
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

// TestPlanUserKeyTenantIsNeverClientControlled is the A10 regression stated as an
// invariant: for every body shape, the stored tenant is the authenticated account.
func TestPlanUserKeyTenantIsNeverClientControlled(t *testing.T) {
	alice := aliceWithGroup("default")
	for _, req := range []CreateUserKeyRequest{
		{},
		{Name: "admin"},
		{TenantID: "admin"},
		{TenantID: "admin", Name: "admin"},
		{TenantID: "   ", Name: "admin"},
	} {
		rec := PlanUserKey(alice, req, "sk-airoute-generated")
		if rec.TenantID != alice.Username {
			t.Fatalf("req %+v produced tenant %q, want %q", req, rec.TenantID, alice.Username)
		}
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
	if rec.UserID != 0 || rec.GroupName != "" || rec.TenantID != "" {
		t.Errorf("a missing user should leave identity unset: user=%d group=%q tenant=%q",
			rec.UserID, rec.GroupName, rec.TenantID)
	}
	if rec.Status != "active" || rec.TPM != DefaultUserKeyTPM || rec.RPM != DefaultUserKeyRPM {
		t.Errorf("the quota rules should still apply: status=%q tpm=%d rpm=%d", rec.Status, rec.TPM, rec.RPM)
	}
}

// TestAdminAndUserPathsDifferOnZeroQuotas pins the remaining asymmetry between the
// self-service and administrator paths, so a future change to either has to be
// deliberate: the administrator deliberately configures zero (unlimited) quotas,
// while the self-service defaults are safe.
func TestAdminAndUserPathsDifferOnZeroQuotas(t *testing.T) {
	alice := aliceWithGroup("default")

	personal := PlanUserKey(alice, CreateUserKeyRequest{TPM: 0, RPM: -1, Budget: -1}, "sk-airoute-personal")
	if personal.TPM != DefaultUserKeyTPM || personal.RPM != DefaultUserKeyRPM || personal.Budget != 0 {
		t.Errorf("the personal path did not apply its defaults: tpm=%d rpm=%d budget=%v",
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
