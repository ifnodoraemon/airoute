package controlplane

import (
	"os"

	"github.com/ifnodoraemon/airoute/internal/config"
	"github.com/ifnodoraemon/airoute/internal/storage"
	"github.com/ifnodoraemon/airoute/internal/telemetry"
	"golang.org/x/crypto/bcrypt"
)

// InitDefaultAdmin ensures an admin account is ready based on config or env.
func InitDefaultAdmin(repo *storage.Repository, cfgs ...*config.Config) {
	var cfg *config.Config
	if len(cfgs) > 0 && cfgs[0] != nil {
		cfg = cfgs[0]
	} else {
		cfg = config.GetGlobalConfig()
	}

	adminUser := cfg.GetAdminUsername()
	adminPass := cfg.GetAdminPassword()
	forceReset := os.Getenv("GATEWAY_ADMIN_RESET") == "true" || os.Getenv("GATEWAY_ADMIN_RESET") == "1"

	existing, _ := repo.GetUserByUsername(adminUser)
	if existing == nil {
		_ = repo.EnsureDefaultAdmin(adminUser, adminPass)
		telemetry.Logger.Info("initialized administrator account", "username", adminUser)
	} else if forceReset {
		hash, err := bcrypt.GenerateFromPassword([]byte(adminPass), bcrypt.DefaultCost)
		if err == nil {
			_ = repo.UpdateUserPassword(adminUser, string(hash))
			telemetry.Logger.Info("force-reset administrator password from configuration", "username", adminUser)
		}
	} else if adminPass != "admin123" {
		// If custom admin password is provided and current user is still using default "admin123", synchronize it
		if err := bcrypt.CompareHashAndPassword([]byte(existing.PasswordHash), []byte("admin123")); err == nil {
			hash, err := bcrypt.GenerateFromPassword([]byte(adminPass), bcrypt.DefaultCost)
			if err == nil {
				_ = repo.UpdateUserPassword(adminUser, string(hash))
				telemetry.Logger.Info("synchronized administrator password with configured credentials", "username", adminUser)
			}
		}
	}

	if adminPass == "admin123" {
		telemetry.Logger.Warn("security notice: default admin credentials in use",
			"username", adminUser,
			"remediation", "change password via web console or set GATEWAY_ADMIN_PASSWORD",
		)
	}
}
