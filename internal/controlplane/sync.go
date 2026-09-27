package controlplane

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/ifnodoraemon/nano-gateway/internal/billing"
	"github.com/ifnodoraemon/nano-gateway/internal/config"
	"github.com/ifnodoraemon/nano-gateway/internal/distributed"
	"github.com/ifnodoraemon/nano-gateway/internal/router"
	"github.com/ifnodoraemon/nano-gateway/internal/storage"
	"github.com/ifnodoraemon/nano-gateway/internal/telemetry"
)

// Synchronizer synchronizes persistent database state to the in-memory Data Plane.
type Synchronizer struct {
	mu         sync.Mutex
	repo       *storage.Repository
	dispatcher *router.Dispatcher
}

// NewSynchronizer creates a new Synchronizer.
func NewSynchronizer(repo *storage.Repository, dispatcher *router.Dispatcher) *Synchronizer {
	return &Synchronizer{
		repo:       repo,
		dispatcher: dispatcher,
	}
}

// ReloadFromDB pulls active channels and virtual keys from DB and atomically updates the Data Plane.
func (s *Synchronizer) ReloadFromDB() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// 1. Sync Channels to Dispatcher
	channels, err := s.repo.ToModelChannels()
	if err != nil {
		return fmt.Errorf("load channels from db error: %w", err)
	}
	s.dispatcher.UpdateChannels(channels)

	// 2. Sync Virtual Keys to Global Config
	keys, err := s.repo.ToModelVirtualKeys()
	if err != nil {
		return fmt.Errorf("load virtual keys from db error: %w", err)
	}

	allKeys, _ := s.repo.ListVirtualKeys()

	cfg := config.GetGlobalConfig()
	cfg.VirtualKeys = keys
	cfg.HasConfiguredKeys = len(allKeys) > 0
	config.SetGlobalConfig(cfg)

	// 3. Sync Model Fallbacks to Dispatcher
	if fallbacks, err := s.repo.GetModelFallbacks(); err == nil {
		s.dispatcher.SetModelFallbacks(fallbacks)
	}

	// 4. Sync Model Prices to Billing Engine
	if billing.GlobalEngine != nil {
		_ = billing.GlobalEngine.ReloadPrices()
	}

	telemetry.Logger.Info("hot reloaded data plane memory state from database",
		"active_channels", len(channels),
		"active_keys", len(keys),
	)

	return nil
}

// ReloadAndBroadcast reloads local data plane state and immediately broadcasts
// an invalidation event across the Redis cluster (<1ms latency peer synchronization).
func (s *Synchronizer) ReloadAndBroadcast(ctx context.Context, reason string) error {
	err := s.ReloadFromDB()
	if client := distributed.GetClient(); client != nil && client.IsActive() {
		if pubErr := client.PublishReload(ctx, reason); pubErr != nil {
			telemetry.Logger.Warn("failed to broadcast reload event to redis cluster", "error", pubErr.Error())
		} else {
			telemetry.Logger.Info("broadcast cluster reload notification via Redis Pub/Sub", "reason", reason)
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
		for {
			select {
			case <-ticker.C:
				_ = s.ReloadFromDB()
			case <-stopCh:
				return
			}
		}
	}()
}

