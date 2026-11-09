package main

import (
	"context"
	"os"
	"time"

	"github.com/ifnodoraemon/airoute/internal/api"
	"github.com/ifnodoraemon/airoute/internal/billing"
	"github.com/ifnodoraemon/airoute/internal/config"
	"github.com/ifnodoraemon/airoute/internal/controlplane"
	"github.com/ifnodoraemon/airoute/internal/distributed"
	"github.com/ifnodoraemon/airoute/internal/middleware"
	"github.com/ifnodoraemon/airoute/internal/router"
	"github.com/ifnodoraemon/airoute/internal/storage"
	"github.com/ifnodoraemon/airoute/internal/telemetry"
)

func runServerWithBilling(cfg *config.Config, repo *storage.Repository, dispatcher *router.Dispatcher, redisClient *distributed.Client, quit <-chan os.Signal) {
	stopCalibration := startServerCalibration(repo)
	defer close(stopCalibration)
	traceCapture := storage.InitTraceCapture(repo)
	defer traceCapture.Stop()
	usageLogRetention := storage.InitUsageLogRetention(repo)
	defer usageLogRetention.Stop()
	billingReconciler := storage.InitBillingReconcile(repo)
	defer billingReconciler.Stop()
	ledgerRetention := storage.InitBillingLedgerRetention(repo)
	defer ledgerRetention.Stop()
	synchronizer := loadServerSnapshot(cfg, repo, dispatcher)
	if stopRedis := subscribeServerReload(redisClient, synchronizer); stopRedis != nil {
		defer close(stopRedis)
	}
	stopSync := make(chan struct{})
	defer close(stopSync)
	synchronizer.StartPeriodicSync(10*time.Second, stopSync)
	runServerHTTP(cfg, repo, dispatcher, synchronizer, quit)
}

func openServerRepository(dataSource string) (*storage.DB, *storage.Repository) {
	// Initialize Storage Layer (SQLite or Distributed PostgreSQL)
	db, err := storage.OpenDB(dataSource)
	if err != nil {
		telemetry.ScopedLogger("system").Error("failed to open database", "error", err.Error())
		os.Exit(1)
	}

	repo := storage.NewRepository(db)

	// Readiness signal for orchestrators and scripts/docker-rebuild.sh:
	// /health stays a cheap process-liveness probe, while /health/ready also
	// verifies that storage answers (and, below, that Redis answers) so a replica
	// with a dead database never reports itself ready for traffic.
	api.RegisterReadinessCheck("database", func(ctx context.Context) error {
		return db.PingContext(ctx)
	})

	return db, repo
}

func startServerCalibration(repo *storage.Repository) chan struct{} {
	// Token-estimate calibration describes the upstream tokenizers, so it must
	// survive restarts and be shared by both replicas.
	if err := billing.LoadCalibration(repo, billing.GlobalCalibrator); err != nil {
		telemetry.ScopedLogger("billing").Warn("failed to load token estimate calibration", "error", err.Error())
	}
	billing.GlobalCalibrator.SetPersistHook(func() error {
		return billing.SaveCalibration(repo, billing.GlobalCalibrator)
	})
	stopCalibration := make(chan struct{})
	go func() {
		ticker := time.NewTicker(5 * time.Minute)
		defer ticker.Stop()
		for {
			select {
			case <-stopCalibration:
				return
			case <-ticker.C:
				if err := billing.SaveCalibration(repo, billing.GlobalCalibrator); err != nil {
					telemetry.ScopedLogger("billing").Warn("failed to persist token estimate calibration", "error", err.Error())
				}
			}
		}
	}()

	return stopCalibration
}

func loadServerSnapshot(cfg *config.Config, repo *storage.Repository, dispatcher *router.Dispatcher) *controlplane.Synchronizer {
	// Initialize Control Plane Synchronizer & load state into Data Plane memory.
	//
	// The data plane starts fail-closed: until the first snapshot is installed the
	// in-memory API key set is empty, and the auth middleware would treat that as
	// "no key was ever registered" (the open-dev-mode bypass). Serving 503 until
	// storage answers keeps a database outage at boot from turning the gateway
	// into an anonymous proxy.
	middleware.SetDataPlaneReady(false)
	synchronizer := controlplane.NewSynchronizer(repo, dispatcher)
	synchronizer.SetOnReady(func() { middleware.SetDataPlaneReady(true) })
	if err := synchronizer.ReloadFromDB(); err != nil {
		telemetry.ScopedLogger("system").Warn("initial sync from db failed; data plane stays closed until a reload succeeds", "error", err.Error())
		dispatcher.UpdateChannels(cfg.Channels)
	}

	// Warn once at startup about models that served traffic recently but have no
	// price rule: they are billed at the hardcoded fallback rate, which is a guess
	// in both directions. Runs in the background so a large audit table cannot
	// delay readiness.
	//
	// The two checks are independent on purpose. They were one function whose
	// `len(unpriced)==0` return also skipped the served-model check, so "the alias
	// is priced but the upstream model is not" - the case by-served-model billing
	// exists for - was never reported.
	go func() {
		warnUnpricedModels(repo, pricingAuditWindow)
		warnUnpricedServedModels(repo, dispatcher, pricingAuditWindow)
	}()

	return synchronizer
}

func subscribeServerReload(redisClient *distributed.Client, synchronizer *controlplane.Synchronizer) chan struct{} {
	// Subscribe to cluster-wide reload broadcasts via Redis Pub/Sub
	if redisClient != nil && redisClient.IsActive() {
		stopRedis := make(chan struct{})
		redisClient.SubscribeReload(func(reason string) {
			telemetry.ScopedLogger("system").Info("handling cluster-wide reload broadcast (<1ms latency)", "reason", reason)
			if err := synchronizer.ReloadFromDB(); err != nil {
				telemetry.ScopedLogger("system").Error("failed to reload data plane after cluster broadcast", "error", err.Error())
			}
		}, stopRedis)
		return stopRedis
	}

	return nil
}
