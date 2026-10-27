package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// A named config file that does not exist must fail fast. The old behaviour
// silently fell back to DefaultConfig(), so a typo'd -config flag or an empty
// bind mount started a gateway with the default SQLite path, no channels and the
// built-in admin123 password instead of crash-looping where an operator sees it.
func TestLoadConfigFailsOnMissingFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "definitely-not-here.yaml")

	cfg, err := LoadConfig(path)
	if err == nil {
		t.Fatalf("LoadConfig(%q) returned a config and no error (port=%d); want a clear failure", path, cfg.Server.Port)
	}
	if !strings.Contains(err.Error(), path) {
		t.Fatalf("error %q does not name the missing path %q", err.Error(), path)
	}
}

// A file that exists but cannot be parsed already failed; keep that behaviour
// pinned so the fail-fast change does not accidentally turn parse errors into
// silent defaults.
func TestLoadConfigFailsOnInvalidYAML(t *testing.T) {
	path := filepath.Join(t.TempDir(), "broken.yaml")
	if err := os.WriteFile(path, []byte("server: [unclosed\n"), 0o600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	if _, err := LoadConfig(path); err == nil {
		t.Fatal("LoadConfig accepted invalid YAML")
	}
}
