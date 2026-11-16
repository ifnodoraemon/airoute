package controlplane

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/ifnodoraemon/airoute/internal/config"
	"github.com/ifnodoraemon/airoute/internal/distributed"
	"github.com/ifnodoraemon/airoute/internal/model"
	"github.com/ifnodoraemon/airoute/internal/telemetry"
)

// Snapshot loading/applying, cluster broadcast and periodic sync, extracted from
// sync.go to keep the synchronizer core under the size red line.

func (s *Synchronizer) loadSnapshot() (*ConfigSnapshot, error) {
	if s.source == nil {
		return nil, fmt.Errorf("configuration source is not configured")
	}

	channels, err := s.source.ToModelChannels()
	if err != nil {
		return nil, fmt.Errorf("load channels from db error: %w", err)
	}

	keys, err := s.source.ToModelAPIKeys()
	if err != nil {
		return nil, fmt.Errorf("load api keys from db error: %w", err)
	}

	hasKeys, err := s.source.HasRegisteredAPIKeys()
	if err != nil {
		return nil, fmt.Errorf("count registered api keys error: %w", err)
	}

	fallbacks, err := s.source.GetModelFallbacks()
	if err != nil {
		return nil, fmt.Errorf("load model fallbacks error: %w", err)
	}

	return &ConfigSnapshot{
		Channels:  channels,
		APIKeys:   keys,
		HasKeys:   hasKeys,
		Fallbacks: fallbacks,
	}, nil
}

// applySnapshot installs a complete snapshot. The in-memory setters cannot fail,
// so once this returns the new configuration is live.
func (s *Synchronizer) applySnapshot(snap *ConfigSnapshot) {
	if s.sink != nil {
		s.sink.UpdateChannels(snap.Channels)
	}
	if s.keys != nil {
		s.keys.UpdateAPIKeys(snap.APIKeys, snap.HasKeys)
	}
	if s.sink != nil {
		s.sink.SetModelFallbacks(snap.Fallbacks)
	}
}

// ReloadAndBroadcast reloads local data plane state and immediately broadcasts
// an invalidation event across the Redis cluster (<1ms latency peer synchronization).
func (s *Synchronizer) ReloadAndBroadcast(ctx context.Context, reason string) error {
	err := s.ReloadFromDB()
	if client := distributed.GetClient(); client != nil && client.IsActive() {
		if pubErr := client.PublishReload(ctx, reason); pubErr != nil {
			telemetry.ScopedLogger("sync").Warn("failed to broadcast reload event to redis cluster", "error", pubErr.Error())
			err = errors.Join(err, &HotReloadError{Stage: "集群广播", Err: pubErr})
		} else {
			telemetry.ScopedLogger("sync").Info("broadcast cluster reload notification via Redis Pub/Sub", "reason", reason)
		}
	}
	return err
}

// StartPeriodicSync runs a background worker to periodically reload configuration from DB,
// enabling automatic hot sync across multi-replica HA clusters.
func (s *Synchronizer) StartPeriodicSync(interval time.Duration, stopCh <-chan struct{}) {
	if interval <= 0 {
		interval = 10 * time.Second
	}
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		var consecutiveFailures int
		for {
			select {
			case <-ticker.C:
				if err := s.ReloadFromDB(); err != nil {
					consecutiveFailures++
					// Log the first failure and then only every 30th (5 minutes at
					// the default interval) so a persistent DB outage does not
					// flood the log while staying observable.
					if consecutiveFailures == 1 || consecutiveFailures%30 == 0 {
						telemetry.ScopedLogger("sync").Error("periodic data plane sync from database failed",
							"error", err.Error(),
							"consecutive_failures", consecutiveFailures,
						)
					}
				} else if consecutiveFailures > 0 {
					telemetry.ScopedLogger("sync").Info("periodic data plane sync recovered",
						"after_failures", consecutiveFailures)
					consecutiveFailures = 0
				}
			case <-stopCh:
				return
			}
		}
	}()
}

// globalKeyConfigSink writes the key set into the package-level runtime config.
// The config is resolved per call because it is a global that may be installed
// after the synchronizer is constructed.
type globalKeyConfigSink struct{}

func (globalKeyConfigSink) UpdateAPIKeys(keys []model.APIKeyConfig, hasConfiguredKeys bool) {
	if cfg := config.GetGlobalConfig(); cfg != nil {
		cfg.UpdateAPIKeys(keys, hasConfiguredKeys)
	}
}

// snapshotFingerprint is a stable digest of the configuration a reload would
// install. It is used only to decide whether the periodic reload log is worth
// writing, never to decide whether to apply a snapshot.
//
// Usage counters are part of the key record but are not configuration: they move
// with every billed request, so hashing them unchanged would make the digest
// differ on every reload and the log would keep firing at INFO every 10s — the
// exact noise this gate exists to remove.
func snapshotFingerprint(snap *ConfigSnapshot) string {
	if snap == nil {
		return ""
	}
	configOnly := make([]model.APIKeyConfig, len(snap.APIKeys))
	copy(configOnly, snap.APIKeys)
	for i := range configOnly {
		configOnly[i].UsedCost = 0
		configOnly[i].UsedCostUnits = 0
		configOnly[i].UserBalance = 0
	}

	hasher := sha256.New()
	encoder := json.NewEncoder(hasher)
	_ = encoder.Encode(snap.Channels)
	// The repository attaches one shared route snapshot to every active channel.
	// RouteRules is runtime-only in ChannelConfig JSON, so hash it explicitly.
	if len(snap.Channels) > 0 {
		_ = encoder.Encode(snap.Channels[0].RouteRules)
	}
	_ = encoder.Encode(configOnly)
	_ = encoder.Encode(snap.HasKeys)
	_ = encoder.Encode(snap.Fallbacks)
	return hex.EncodeToString(hasher.Sum(nil))
}