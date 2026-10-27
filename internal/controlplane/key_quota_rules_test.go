package controlplane

import (
	"testing"

	"github.com/ifnodoraemon/airoute/internal/storage"
)

// The /admin/keys endpoint is reachable by regular accounts (it is gated by the
// session middleware, not by the admin role). Quotas therefore have to be
// clamped in the shared creation rules, otherwise a tenant can ask for rpm=0
// (unlimited) there and skip the self-service policy that /user/keys enforces.
func TestKeyCreationRulesClampSelfServiceQuotas(t *testing.T) {
	lookup := &fakeKeyOwnerLookup{users: map[string]*storage.UserRecord{
		"alice": aliceWithGroup("default"),
		"admin": owner(2, "admin"),
	}}
	rules := NewKeyCreationRules(lookup)

	got := rules.PlanCreate(&AdminClaims{Username: "alice", Role: "user"}, CreateAPIKeyRequest{RPM: 0, TPM: 0, Budget: -5})
	if !got.Allowed {
		t.Fatalf("refused: %v %q", got.Denial, got.Message)
	}
	if got.Record.RPM != DefaultUserKeyRPM {
		t.Errorf("self-service RPM = %d, want the safe default %d (0 means unlimited downstream)", got.Record.RPM, DefaultUserKeyRPM)
	}
	if got.Record.TPM != DefaultUserKeyTPM {
		t.Errorf("self-service TPM = %d, want the safe default %d", got.Record.TPM, DefaultUserKeyTPM)
	}
	if got.Record.Budget != 0 {
		t.Errorf("self-service Budget = %v, want 0 for a negative request", got.Record.Budget)
	}

	got = rules.PlanCreate(&AdminClaims{Username: "alice", Role: "user"}, CreateAPIKeyRequest{
		RPM: MaxSelfServiceKeyRPM + 1000,
		TPM: MaxSelfServiceKeyTPM + 1000,
	})
	if !got.Allowed {
		t.Fatalf("refused: %v", got.Denial)
	}
	if got.Record.RPM != MaxSelfServiceKeyRPM {
		t.Errorf("self-service RPM = %d, want the cap %d", got.Record.RPM, MaxSelfServiceKeyRPM)
	}
	if got.Record.TPM != MaxSelfServiceKeyTPM {
		t.Errorf("self-service TPM = %d, want the cap %d", got.Record.TPM, MaxSelfServiceKeyTPM)
	}

	// An administrator keeps the documented 0 = unlimited choice.
	got = rules.PlanCreate(&AdminClaims{Username: "admin", Role: "admin"}, CreateAPIKeyRequest{RPM: 0, TPM: 0})
	if !got.Allowed {
		t.Fatalf("admin refused: %v", got.Denial)
	}
	if got.Record.RPM != 0 || got.Record.TPM != 0 {
		t.Errorf("admin quotas = %d/%d, want 0/0 (unlimited)", got.Record.RPM, got.Record.TPM)
	}
}
