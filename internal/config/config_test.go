package config

import (
	"os"
	"strings"
	"testing"
)

func TestLoadConfigExpandsDocumentedDefaults(t *testing.T) {
	t.Setenv("GATEWAY_PORT", "")
	t.Setenv("GATEWAY_ADMIN_USER", "")
	path := t.TempDir() + "/config.yaml"
	if err := os.WriteFile(path, []byte("server:\n  port: ${GATEWAY_PORT:-8080}\nadmin:\n  username: ${GATEWAY_ADMIN_USER:-admin}\n"), 0600); err != nil {
		t.Fatal(err)
	}
	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Server.Port != 8080 || cfg.Admin.Username != "admin" {
		t.Fatalf("defaults not expanded: %+v", cfg)
	}
	if strings.Contains(expandConfigEnv("${GATEWAY_PORT:-8080}"), "${") {
		t.Fatal("unexpanded default")
	}
}
