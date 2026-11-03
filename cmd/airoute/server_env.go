package main

import (
	"os"
	"strconv"
	"strings"

	"github.com/ifnodoraemon/airoute/internal/config"
)

// applyServerEnvOverrides applies the environment knobs that override values loaded
// from the YAML config file.
//
// GATEWAY_PORT/GATEWAY_HOST predate this helper. The three runtime-tuning keys were
// added because the shipped config.yaml is mounted read-only by Compose: an operator
// who set them only in .env got a silent no-op, while the file header claimed every
// parameter was wired to .env. Invalid or non-positive values are ignored (the YAML
// value wins) rather than aborting startup, matching the existing port handling.
func applyServerEnvOverrides(cfg *config.Config) {
	if cfg == nil {
		return
	}
	if portEnv := strings.TrimSpace(os.Getenv("GATEWAY_PORT")); portEnv != "" {
		if p, err := strconv.Atoi(portEnv); err == nil && p > 0 {
			cfg.Server.Port = p
		}
	}
	if hostEnv := strings.TrimSpace(os.Getenv("GATEWAY_HOST")); hostEnv != "" {
		cfg.Server.Host = hostEnv
	}
	if v := strings.TrimSpace(os.Getenv("GATEWAY_READ_TIMEOUT_SEC")); v != "" {
		if sec, err := strconv.Atoi(v); err == nil && sec > 0 {
			cfg.Server.ReadTimeoutSec = sec
		}
	}
	if v := strings.TrimSpace(os.Getenv("GATEWAY_WRITE_TIMEOUT_SEC")); v != "" {
		if sec, err := strconv.Atoi(v); err == nil && sec > 0 {
			cfg.Server.WriteTimeoutSec = sec
		}
	}
	if v := strings.ToLower(strings.TrimSpace(os.Getenv("GATEWAY_LOG_LEVEL"))); v != "" {
		cfg.Server.LogLevel = v
	}
}
