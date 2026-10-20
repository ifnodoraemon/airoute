package config

import (
	"sync"
	"testing"

	"github.com/ifnodoraemon/airoute/internal/model"
)

// TestConcurrentAPIKeyAccessNoRace is a regression test for an unsynchronized
// read of Config.APIKeys / Config.HasConfiguredKeys in the auth middleware.
// The synchronizer rewrites both fields on every hot reload (every 10s and on
// each admin change), so reading them directly was a data race on every request.
// Run with -race to make this meaningful.
func TestConcurrentAPIKeyAccessNoRace(t *testing.T) {
	c := &Config{}

	const iterations = 20000
	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		for i := 0; i < iterations; i++ {
			c.UpdateAPIKeys([]model.APIKeyConfig{{Key: "sk-a"}}, true)
		}
	}()

	go func() {
		defer wg.Done()
		for i := 0; i < iterations; i++ {
			// Mirrors the auth middleware's open-mode check.
			if c.ActiveAPIKeyCount() == 0 && !c.HasRegisteredAPIKeys() {
				continue
			}
			_ = c.GetAPIKey("sk-a")
		}
	}()

	wg.Wait()
}
