package controlplane

import (
	"testing"

	"github.com/ifnodoraemon/airoute/internal/config"
	"github.com/ifnodoraemon/airoute/internal/storage"
	"golang.org/x/crypto/bcrypt"
)

// adminBootstrapFixture isolates the test from credentials in the ambient
// environment: the config value must be the one that is applied.
func adminBootstrapFixture(t *testing.T) (*storage.Repository, *config.Config) {
	t.Helper()
	t.Setenv("GATEWAY_ADMIN_USER", "")
	t.Setenv("GATEWAY_ADMIN_USERNAME", "")
	t.Setenv("GATEWAY_ADMIN_PASSWORD", "")
	t.Setenv("GATEWAY_ADMIN_RESET", "")

	db, err := storage.OpenDB(":memory:")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	cfg := &config.Config{Admin: config.AdminConfig{Username: "root", Password: "root-pass-123"}}
	return storage.NewRepository(db), cfg
}

// TestInitDefaultAdminBootstrapsAnEmptyDatabase is the regression test for a
// fresh install that used to come up without any administrator: the lookup
// returned sql.ErrNoRows, the bootstrap reported it as a failed lookup and
// skipped account creation, so nobody could log in.
func TestInitDefaultAdminBootstrapsAnEmptyDatabase(t *testing.T) {
	repo, cfg := adminBootstrapFixture(t)

	InitDefaultAdmin(repo, cfg)

	admin, err := repo.GetUserByUsername("root")
	if err != nil {
		t.Fatalf("administrator was not created: %v", err)
	}
	if admin.Role != "admin" {
		t.Fatalf("role = %q, want admin", admin.Role)
	}
	if admin.Status != "active" {
		t.Fatalf("status = %q, want active", admin.Status)
	}
	if admin.Balance <= 0 {
		t.Fatalf("balance = %v, want a usable administrator balance", admin.Balance)
	}
	if err := bcrypt.CompareHashAndPassword([]byte(admin.PasswordHash), []byte("root-pass-123")); err != nil {
		t.Fatalf("stored password does not match the configured one: %v", err)
	}

	// A restart must not create a second account.
	InitDefaultAdmin(repo, cfg)
	users, err := repo.ListUsers()
	if err != nil {
		t.Fatalf("list users: %v", err)
	}
	if len(users) != 1 {
		t.Fatalf("users = %d, want 1 after a repeated bootstrap", len(users))
	}
}

// TestInitDefaultAdminForceReset covers the escape hatch used when the stored
// credential no longer matches the configuration.
func TestInitDefaultAdminForceReset(t *testing.T) {
	repo, cfg := adminBootstrapFixture(t)
	InitDefaultAdmin(repo, cfg)

	t.Setenv("GATEWAY_ADMIN_RESET", "true")
	InitDefaultAdmin(repo, &config.Config{Admin: config.AdminConfig{Username: "root", Password: "rotated-pass-456"}})

	admin, err := repo.GetUserByUsername("root")
	if err != nil {
		t.Fatalf("load admin: %v", err)
	}
	if err := bcrypt.CompareHashAndPassword([]byte(admin.PasswordHash), []byte("rotated-pass-456")); err != nil {
		t.Fatalf("forced reset did not apply the configured password: %v", err)
	}
}
