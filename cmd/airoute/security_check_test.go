package main

import (
	"testing"

	"github.com/ifnodoraemon/airoute/internal/config"
)

// The startup gate must be fail-closed: the shipped Compose/Swarm deployments do
// not set GIN_MODE, so the old release-mode-only check never ran on the
// deployments it was meant to protect.
func TestAllowInsecureDefaultsRequiresExplicitOptIn(t *testing.T) {
	for _, raw := range []string{"1", "true", "TRUE", "yes", "on", " true "} {
		t.Run("enabled/"+raw, func(t *testing.T) {
			t.Setenv("GATEWAY_ALLOW_INSECURE_DEFAULTS", raw)
			if !allowInsecureDefaults() {
				t.Fatalf("GATEWAY_ALLOW_INSECURE_DEFAULTS=%q should enable the escape hatch", raw)
			}
		})
	}
	for _, raw := range []string{"", "0", "false", "no", "off", "garbage"} {
		t.Run("disabled/"+raw, func(t *testing.T) {
			t.Setenv("GATEWAY_ALLOW_INSECURE_DEFAULTS", raw)
			if allowInsecureDefaults() {
				t.Fatalf("GATEWAY_ALLOW_INSECURE_DEFAULTS=%q must not enable the escape hatch", raw)
			}
		})
	}
}

func TestProductionSecurityIssuesStrongCredentials(t *testing.T) {
	t.Setenv("GATEWAY_ADMIN_PASSWORD", "Zx9-some-long-random-admin-password-1234")
	t.Setenv("GATEWAY_ADMIN_SECRET", "9f2c5e8a1b4d7f0a3c6e9b2d5f8a1c4e7b0d3f6a9c2e5b8d1f4a7c0e3b6d9f2a")
	if issues := productionSecurityIssues(&config.Config{}); len(issues) != 0 {
		t.Fatalf("strong credentials must pass the startup check, got %v", issues)
	}
}

func TestProductionSecurityIssuesRejectsWeakCredentials(t *testing.T) {
	cases := []struct {
		name   string
		pass   string
		secret string
	}{
		{"default password", "admin123", "9f2c5e8a1b4d7f0a3c6e9b2d5f8a1c4e7b0d3f6a9c2e5b8d1f4a7c0e3b6d9f2a"},
		{"short password", "short", "9f2c5e8a1b4d7f0a3c6e9b2d5f8a1c4e7b0d3f6a9c2e5b8d1f4a7c0e3b6d9f2a"},
		{"empty secret", "Zx9-some-long-random-admin-password-1234", ""},
		{"placeholder secret", "Zx9-some-long-random-admin-password-1234", "airoute-secret-key-change-me"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("GATEWAY_ADMIN_PASSWORD", tc.pass)
			t.Setenv("GATEWAY_ADMIN_SECRET", tc.secret)
			if issues := productionSecurityIssues(&config.Config{}); len(issues) == 0 {
				t.Fatal("weak credentials must be reported as startup issues")
			}
		})
	}
}
