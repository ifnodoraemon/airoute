package tests

import (
	"testing"

	"github.com/ifnodoraemon/airoute/internal/billing"
	"github.com/ifnodoraemon/airoute/internal/config"
	"github.com/ifnodoraemon/airoute/internal/storage"
)

// Protocol-only fixtures have no account ledger. Never inherit another test's
// repository or logger: authoritative admission must fail closed for real errors.
func useConfigOnlyFixture(t *testing.T, cfg *config.Config) {
	t.Helper()
	repo, logger, engine := storage.GetGlobalRepository(), storage.GlobalAsyncLogger, billing.GlobalEngine
	previous := config.GetGlobalConfig()
	storage.SetGlobalRepository(nil)
	storage.GlobalAsyncLogger = nil
	billing.GlobalEngine = nil
	config.SetGlobalConfig(cfg)
	t.Cleanup(func() {
		storage.SetGlobalRepository(repo)
		storage.GlobalAsyncLogger = logger
		billing.GlobalEngine = engine
		config.SetGlobalConfig(previous)
	})
}
