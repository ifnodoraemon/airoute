package controlplane

import (
	"testing"

	"github.com/ifnodoraemon/airoute/internal/storage"
)

// ApplyKeyUpdate must enforce the same quota policy as key creation.
//
// UpdateAPIKey is not admin-only — the session middleware gates it — so before
// this clamp a regular tenant could PATCH their own key to rpm:0/tpm:0 (which the
// storage and routing layers read as unlimited) or to an over-cap value, and get
// exactly the rate-limit bypass that creation refuses. An administrator keeps
// 0 = unlimited and arbitrary values, because issuing an unlimited key is an
// operator decision.
func TestApplyKeyUpdateClampsSelfServiceQuotas(t *testing.T) {
	userAccess := KeyAccess{Allowed: true, User: aliceWithGroup("default")}
	adminAccess := KeyAccess{Allowed: true, IsAdmin: true}

	t.Run("a regular user cannot turn their key unlimited with zeros", func(t *testing.T) {
		got := ApplyKeyUpdate(userAccess, existingKey(), UpdateAPIKeyRequest{
			RPM: intPtr(0), TPM: intPtr(0),
		})

		if !got.Allowed {
			t.Fatalf("refused: %v", got.Denial)
		}
		if got.Record.RPM != DefaultUserKeyRPM {
			t.Errorf("RPM = %d, want the safe default %d (0 means unlimited downstream)", got.Record.RPM, DefaultUserKeyRPM)
		}
		if got.Record.TPM != DefaultUserKeyTPM {
			t.Errorf("TPM = %d, want the safe default %d", got.Record.TPM, DefaultUserKeyTPM)
		}
	})

	t.Run("a regular user cannot exceed the self-service cap", func(t *testing.T) {
		got := ApplyKeyUpdate(userAccess, existingKey(), UpdateAPIKeyRequest{
			RPM: intPtr(MaxSelfServiceKeyRPM + 1000),
			TPM: intPtr(MaxSelfServiceKeyTPM + 1000),
		})

		if !got.Allowed {
			t.Fatalf("refused: %v", got.Denial)
		}
		if got.Record.RPM != MaxSelfServiceKeyRPM {
			t.Errorf("RPM = %d, want the cap %d", got.Record.RPM, MaxSelfServiceKeyRPM)
		}
		if got.Record.TPM != MaxSelfServiceKeyTPM {
			t.Errorf("TPM = %d, want the cap %d", got.Record.TPM, MaxSelfServiceKeyTPM)
		}
	})

	t.Run("an in-policy value is applied unchanged", func(t *testing.T) {
		got := ApplyKeyUpdate(userAccess, existingKey(), UpdateAPIKeyRequest{
			RPM: intPtr(120), TPM: intPtr(5000),
		})

		if !got.Allowed {
			t.Fatalf("refused: %v", got.Denial)
		}
		if got.Record.RPM != 120 || got.Record.TPM != 5000 {
			t.Errorf("quotas = %d/%d, want 120/5000", got.Record.RPM, got.Record.TPM)
		}
	})

	t.Run("an omitted quota is left untouched", func(t *testing.T) {
		got := ApplyKeyUpdate(userAccess, existingKey(), UpdateAPIKeyRequest{RPM: intPtr(0)})

		if !got.Allowed {
			t.Fatalf("refused: %v", got.Denial)
		}
		if got.Record.TPM != 100000 {
			t.Errorf("an update that only provided rpm changed tpm to %d", got.Record.TPM)
		}
	})

	t.Run("an administrator keeps zero and arbitrary values", func(t *testing.T) {
		zero := ApplyKeyUpdate(adminAccess, existingKey(), UpdateAPIKeyRequest{RPM: intPtr(0), TPM: intPtr(0)})
		if !zero.Allowed {
			t.Fatalf("refused: %v", zero.Denial)
		}
		if zero.Record.RPM != 0 || zero.Record.TPM != 0 {
			t.Errorf("admin quotas = %d/%d, want 0/0 (unlimited)", zero.Record.RPM, zero.Record.TPM)
		}

		big := ApplyKeyUpdate(adminAccess, existingKey(), UpdateAPIKeyRequest{
			RPM: intPtr(MaxSelfServiceKeyRPM * 10), TPM: intPtr(MaxSelfServiceKeyTPM * 10),
		})
		if big.Record.RPM != MaxSelfServiceKeyRPM*10 || big.Record.TPM != MaxSelfServiceKeyTPM*10 {
			t.Errorf("admin quotas were clamped to %d/%d", big.Record.RPM, big.Record.TPM)
		}
	})
}

// The clamp policy lives in one helper, so the self-service creation path must
// apply exactly the same bounds.
func TestPlanUserKeySharesTheQuotaClamp(t *testing.T) {
	user := &storage.UserRecord{ID: 7, Username: "alice", GroupName: "default"}

	got := PlanUserKey(user, CreateUserKeyRequest{RPM: -10, TPM: MaxSelfServiceKeyTPM + 1}, "sk-test")
	if got.RPM != DefaultUserKeyRPM {
		t.Errorf("PlanUserKey RPM = %d, want %d", got.RPM, DefaultUserKeyRPM)
	}
	if got.TPM != MaxSelfServiceKeyTPM {
		t.Errorf("PlanUserKey TPM = %d, want the cap %d", got.TPM, MaxSelfServiceKeyTPM)
	}
}
