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

	existing, err := repo.GetUserByUsername(adminUser)
	if err != nil {
		// A failed lookup (schema drift, NULL column, DB hiccup) used to be
		// indistinguishable from "no admin yet", which is how a drifted schema
		// silently produced a confusing login situation.
		telemetry.ScopedLogger("sync").Error("could not look up the administrator account; admin bootstrap skipped",
			"username", adminUser, "error", err.Error())
		return
	}
	switch {
	case existing == nil:
		_ = repo.EnsureDefaultAdmin(adminUser, adminPass)
		telemetry.ScopedLogger("sync").Info("initialized administrator account", "username", adminUser)
	case forceReset:
		hash, hashErr := bcrypt.GenerateFromPassword([]byte(adminPass), bcrypt.DefaultCost)
		if hashErr == nil {
			_ = repo.UpdateUserPassword(adminUser, string(hash))
			telemetry.ScopedLogger("sync").Info("force-reset administrator password from configuration", "username", adminUser)
		}
	case bcrypt.CompareHashAndPassword([]byte(existing.PasswordHash), []byte(adminPass)) == nil:
		// Configured credential already matches: nothing to do.
	case bcrypt.CompareHashAndPassword([]byte(existing.PasswordHash), []byte("admin123")) == nil:
		// The account still uses the shipped default, so the configured password
		// can safely be applied.
		hash, hashErr := bcrypt.GenerateFromPassword([]byte(adminPass), bcrypt.DefaultCost)
		if hashErr == nil {
			_ = repo.UpdateUserPassword(adminUser, string(hash))
			telemetry.ScopedLogger("sync").Info("synchronized administrator password with configured credentials", "username", adminUser)
		}
	default:
		// The stored password was changed (via the console) and deliberately
		// differs from configuration. Never overwrite it, but say so loudly:
		// otherwise operators change GATEWAY_ADMIN_PASSWORD, see the service
		// restart successfully, and still cannot log in.
		telemetry.ScopedLogger("sync").Warn("configured admin password does not match the stored credential; configuration does not override an existing account",
			"username", adminUser,
			"remediation", "log in with the password set in the console, or set GATEWAY_ADMIN_RESET=true once and restart to force the configured password",
		)
	}

	if adminPass == "admin123" {
		telemetry.ScopedLogger("sync").Warn("security notice: default admin credentials in use",
			"username", adminUser,
			"remediation", "change password via web console or set GATEWAY_ADMIN_PASSWORD",
		)
	}
}
