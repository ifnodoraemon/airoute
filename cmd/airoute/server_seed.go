package main

import (
	"fmt"
	"strings"

	"github.com/ifnodoraemon/airoute/internal/config"
	"github.com/ifnodoraemon/airoute/internal/controlplane"
	"github.com/ifnodoraemon/airoute/internal/storage"
	"github.com/ifnodoraemon/airoute/internal/telemetry"
)

// seedChannelsFromConfig inserts the channels from the YAML config file when the
// database has none yet.
//
// It seeds only a genuinely empty database (the historical behaviour: an operator
// who deleted a channel must not have it re-created on the next restart) and it
// never seeds after a failed read, because that turns a database hiccup into
// duplicated channels. Duplicate names inside the config file itself are collapsed
// to the first entry.
func seedChannelsFromConfig(repo *storage.Repository, cfg *config.Config) error {
	if repo == nil || cfg == nil || len(cfg.Channels) == 0 {
		return nil
	}
	existing, err := repo.ListChannels()
	if err != nil {
		return fmt.Errorf("cannot list channels: %w", err)
	}
	if len(existing) > 0 {
		return nil
	}
	telemetry.ScopedLogger("system").Info("seeding database from initial config file", "channels", len(cfg.Channels))
	seen := make(map[string]struct{}, len(cfg.Channels))
	for _, ch := range cfg.Channels {
		name := strings.ToLower(strings.TrimSpace(ch.Name))
		if _, duplicate := seen[name]; duplicate {
			telemetry.ScopedLogger("system").Warn("ignoring duplicate channel name in the config file", "channel", ch.Name)
			continue
		}
		seen[name] = struct{}{}
		if err := repo.CreateChannel(&storage.ChannelRecord{
			Name:             ch.Name,
			Type:             ch.Type,
			BaseURL:          ch.BaseURL,
			APIKey:           ch.APIKey,
			Models:           ch.Models,
			ModelMapping:     ch.ModelMapping,
			Protocols:        ch.Protocols,
			Priority:         ch.Priority,
			Weight:           ch.Weight,
			TimeoutSeconds:   ch.TimeoutSeconds,
			MaxConcurrency:   ch.MaxConcurrency,
			QueueLimit:       ch.QueueLimit,
			QueueWaitSeconds: ch.QueueWaitSeconds,
		}); err != nil {
			return fmt.Errorf("cannot create channel %q: %w", ch.Name, err)
		}
	}
	return nil
}

// seedAPIKeysFromConfig is seedChannelsFromConfig for the YAML api_keys list: it
// seeds only an empty database and never after a failed read.
func seedAPIKeysFromConfig(repo *storage.Repository, cfg *config.Config) error {
	if repo == nil || cfg == nil || len(cfg.APIKeys) == 0 {
		return nil
	}
	existing, err := repo.ListAPIKeys()
	if err != nil {
		return fmt.Errorf("cannot list api keys: %w", err)
	}
	if len(existing) > 0 {
		return nil
	}
	telemetry.ScopedLogger("system").Info("seeding database api keys from initial config file", "keys", len(cfg.APIKeys))
	seen := make(map[string]struct{}, len(cfg.APIKeys))
	for _, k := range cfg.APIKeys {
		if _, duplicate := seen[k.Key]; duplicate {
			telemetry.ScopedLogger("system").Warn("ignoring duplicate api key in the config file")
			continue
		}
		seen[k.Key] = struct{}{}
		if err := repo.CreateAPIKey(&storage.APIKeyRecord{
			Key:           k.Key,
			TenantID:      k.TenantID,
			AllowedModels: k.AllowedModels,
			RPM:           k.RPM,
			TPM:           k.TPM,
			Budget:        k.Budget,
		}); err != nil {
			return fmt.Errorf("cannot create api key: %w", err)
		}
	}
	return nil
}

func seedServerRepository(repo *storage.Repository, cfg *config.Config) {
	// Ensure default admin user account exists
	controlplane.InitDefaultAdmin(repo, cfg)

	// Seed DB from YAML config if DB is currently empty.
	//
	// Both reads used to discard their error, so a transient database failure at
	// boot was indistinguishable from "no rows": the seeder then inserted every
	// configured channel and key a second time. Nothing enforces uniqueness on
	// channels.name or api_keys.key, so that produced duplicate routing entries
	// (the same upstream receiving the traffic of two channels) and duplicate rows
	// for one credential. A failed read now means "do not seed": the operator sees a
	// warning and the periodic synchroniser installs the real configuration.
	if err := seedChannelsFromConfig(repo, cfg); err != nil {
		telemetry.ScopedLogger("system").Warn("skipping channel seeding from the config file", "error", err.Error())
	}
	if err := seedAPIKeysFromConfig(repo, cfg); err != nil {
		telemetry.ScopedLogger("system").Warn("skipping api key seeding from the config file", "error", err.Error())
	}

}
