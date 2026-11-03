package main

import (
	"testing"

	"github.com/ifnodoraemon/airoute/internal/config"
)

// A-14: configs/config.yaml claimed every parameter was wired to .env, while the
// HTTP timeouts and the log level had no environment path at all (they could only be
// changed by editing a file Compose mounts read-only). These are the overrides that
// make the claim true, including the "garbage is ignored, YAML wins" behaviour of
// the pre-existing port handling.
func TestApplyServerEnvOverrides(t *testing.T) {
	base := func() *config.Config {
		cfg := &config.Config{}
		cfg.Server.Host = "0.0.0.0"
		cfg.Server.Port = 8080
		cfg.Server.ReadTimeoutSec = 600
		cfg.Server.WriteTimeoutSec = 600
		cfg.Server.LogLevel = "info"
		return cfg
	}

	t.Run("valid overrides win", func(t *testing.T) {
		for env, value := range map[string]string{
			"GATEWAY_HOST":              "127.0.0.1",
			"GATEWAY_PORT":              "9090",
			"GATEWAY_READ_TIMEOUT_SEC":  "60",
			"GATEWAY_WRITE_TIMEOUT_SEC": "1200",
			"GATEWAY_LOG_LEVEL":         " DEBUG ",
		} {
			t.Setenv(env, value)
		}
		cfg := base()
		applyServerEnvOverrides(cfg)
		if cfg.Server.Host != "127.0.0.1" || cfg.Server.Port != 9090 {
			t.Fatalf("host/port = %s/%d, want 127.0.0.1/9090", cfg.Server.Host, cfg.Server.Port)
		}
		if cfg.Server.ReadTimeoutSec != 60 || cfg.Server.WriteTimeoutSec != 1200 {
			t.Fatalf("timeouts = %d/%d, want 60/1200", cfg.Server.ReadTimeoutSec, cfg.Server.WriteTimeoutSec)
		}
		if cfg.Server.LogLevel != "debug" {
			t.Fatalf("log level = %q, want debug (trimmed and lower-cased)", cfg.Server.LogLevel)
		}
	})

	t.Run("unset keeps the file values", func(t *testing.T) {
		for _, env := range []string{
			"GATEWAY_HOST", "GATEWAY_PORT", "GATEWAY_READ_TIMEOUT_SEC",
			"GATEWAY_WRITE_TIMEOUT_SEC", "GATEWAY_LOG_LEVEL",
		} {
			t.Setenv(env, "")
		}
		cfg := base()
		applyServerEnvOverrides(cfg)
		if cfg.Server.Host != "0.0.0.0" || cfg.Server.Port != 8080 ||
			cfg.Server.ReadTimeoutSec != 600 || cfg.Server.WriteTimeoutSec != 600 ||
			cfg.Server.LogLevel != "info" {
			t.Fatalf("unset env changed the file values: %+v", cfg.Server)
		}
	})

	t.Run("invalid values are ignored", func(t *testing.T) {
		for env, value := range map[string]string{
			"GATEWAY_PORT":              "not-a-port",
			"GATEWAY_READ_TIMEOUT_SEC":  "-5",
			"GATEWAY_WRITE_TIMEOUT_SEC": "0",
		} {
			t.Setenv(env, value)
		}
		cfg := base()
		applyServerEnvOverrides(cfg)
		if cfg.Server.Port != 8080 || cfg.Server.ReadTimeoutSec != 600 || cfg.Server.WriteTimeoutSec != 600 {
			t.Fatalf("invalid env values were applied: %+v", cfg.Server)
		}
	})

	t.Run("nil config is a no-op", func(t *testing.T) {
		applyServerEnvOverrides(nil) // must not panic
	})
}
