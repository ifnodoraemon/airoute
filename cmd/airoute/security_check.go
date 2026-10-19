package main

import (
	"os"

	"github.com/ifnodoraemon/airoute/internal/config"
)

// weakAdminSecrets are placeholder values that ship in documentation, sample
// manifests or compose files. They must never be trusted in production.
var weakAdminSecrets = map[string]struct{}{
	"":                             {},
	"airoute-secret-key-change-me": {},
	"airoute-cluster-admin-shared-secret-key-32b":   {},
	"nano-swarm-enterprise-secret-key":              {},
	"change_me_super_secret_production_key_32bytes": {},
	"REPLACE_WITH_STRONG_RANDOM_SECRET":             {},
}

// productionSecurityIssues reports insecure default credentials that would let
// an attacker forge admin tokens or log in with well-known passwords.
func productionSecurityIssues(cfg *config.Config) []string {
	var issues []string

	secret := os.Getenv("GATEWAY_ADMIN_SECRET")
	if secret == "" {
		secret = os.Getenv("NANO_SECRET_KEY")
	}
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
