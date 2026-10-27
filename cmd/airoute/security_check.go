package main

import (
	"os"
	"strings"

	"github.com/ifnodoraemon/airoute/internal/config"
)

// allowInsecureDefaults reports whether the operator explicitly opted into the
// insecure development defaults. The startup check is fail-closed by default: a
// deployment that forgot to set strong credentials must not silently run with a
// forgeable admin signing key and a well-known password. The escape hatch exists
// only for throwaway local development.
func allowInsecureDefaults() bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv("GATEWAY_ALLOW_INSECURE_DEFAULTS"))) {
	case "1", "true", "yes", "on":
		return true
	}
	return false
}

// weakAdminSecrets are placeholder values that ship in documentation, sample
// manifests or compose files. They must never be trusted in production.
var weakAdminSecrets = map[string]struct{}{
	"":                             {},
	"airoute-secret-key-change-me": {},
	"airoute-cluster-admin-shared-secret-key-32b":   {},
	"change_me_super_secret_production_key_32bytes": {},
	"REPLACE_WITH_STRONG_RANDOM_SECRET":             {},
}

// productionSecurityIssues reports insecure default credentials that would let
// an attacker forge admin tokens or log in with well-known passwords.
func productionSecurityIssues(cfg *config.Config) []string {
	var issues []string

	secret := os.Getenv("GATEWAY_ADMIN_SECRET")
	if _, weak := weakAdminSecrets[secret]; weak {
		issues = append(issues, "GATEWAY_ADMIN_SECRET is empty or a well-known placeholder (admin tokens would be forgeable)")
	}

	if cfg != nil {
		if pwd := cfg.GetAdminPassword(); pwd == "admin123" || len(pwd) < 8 {
			issues = append(issues, "admin password is the default admin123 or shorter than 8 characters")
		}
	}

	return issues
}
