package main

import (
	"context"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/ifnodoraemon/nano-gateway/internal/api"
	"github.com/ifnodoraemon/nano-gateway/internal/billing"
	"github.com/ifnodoraemon/nano-gateway/internal/config"
	"github.com/ifnodoraemon/nano-gateway/internal/controlplane"
	"github.com/ifnodoraemon/nano-gateway/internal/distributed"
	"github.com/ifnodoraemon/nano-gateway/internal/router"
	"github.com/ifnodoraemon/nano-gateway/internal/storage"
	"github.com/ifnodoraemon/nano-gateway/internal/telemetry"
)

func main() {
	defaultConfig := "configs/config.yaml"
	if env := os.Getenv("GATEWAY_CONFIG"); env != "" {
		defaultConfig = env
	}
	defaultDB := "data/gateway.db"
	if env := os.Getenv("GATEWAY_DB_DSN"); env != "" {
		defaultDB = env
	} else if env := os.Getenv("GATEWAY_DB"); env != "" {
		defaultDB = env
	}

	configPath := flag.String("config", defaultConfig, "Path to YAML configuration file")
	dbPath := flag.String("db", defaultDB, "Path to SQLite database file")
	flag.Parse()

	// Load file configuration
	cfg, err := config.LoadConfig(*configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to load config: %v\n", err)
		os.Exit(1)
	}

	if portEnv := os.Getenv("GATEWAY_PORT"); portEnv != "" {
		if p, err := strconv.Atoi(portEnv); err == nil && p > 0 {
			cfg.Server.Port = p
		}
	}
	if hostEnv := os.Getenv("GATEWAY_HOST"); hostEnv != "" {
		cfg.Server.Host = hostEnv
	}

	// Initialize Logger
	telemetry.InitLogger(cfg.Server.LogLevel)
	telemetry.Logger.Info("starting nano-gateway",
		"version", "0.1.0",
		"config", *configPath,
		"db", *dbPath,
	)

	dataSource := *dbPath
	if dbEnv := os.Getenv("DATABASE_URL"); dbEnv != "" {
		dataSource = dbEnv
	}

	// Initialize Storage Layer (SQLite or Distributed PostgreSQL)
	db, err := storage.OpenDB(dataSource)
	if err != nil {
		telemetry.Logger.Error("failed to open database", "error", err.Error())
		os.Exit(1)
	}
	defer db.Close()

	repo := storage.NewRepository(db)

	// Ensure default admin user account exists
	controlplane.InitDefaultAdmin(repo)

	// Seed DB from YAML config if DB is currently empty
	existingChannels, _ := repo.ListChannels()
	if len(existingChannels) == 0 && len(cfg.Channels) > 0 {
		telemetry.Logger.Info("seeding database from initial config file", "channels", len(cfg.Channels))
		for _, ch := range cfg.Channels {
			_ = repo.CreateChannel(&storage.ChannelRecord{
				Name:           ch.Name,
				Type:           ch.Type,
				BaseURL:        ch.BaseURL,
				APIKey:         ch.APIKey,
				Models:         ch.Models,
				ModelMapping:   ch.ModelMapping,
				Protocols:      ch.Protocols,
				Priority:       ch.Priority,
				Weight:         ch.Weight,
				TimeoutSeconds: ch.TimeoutSeconds,
			})
		}
	}

	existingKeys, _ := repo.ListVirtualKeys()
	if len(existingKeys) == 0 && len(cfg.VirtualKeys) > 0 {
		for _, vk := range cfg.VirtualKeys {
			_ = repo.CreateVirtualKey(&storage.VirtualKeyRecord{
				Key:           vk.Key,
				TenantID:      vk.TenantID,
				AllowedModels: vk.AllowedModels,
				RPM:           vk.RPM,
				TPM:           vk.TPM,
				Budget:        vk.Budget,
			})
		}
	}

	// Initialize Data Plane Dispatcher
	dispatcher := router.NewDispatcher(nil)

	// Initialize Async Usage Logger for zero-latency audit logs
	asyncLogger := storage.InitAsyncLogger(repo, 10000, 100, 500*time.Millisecond)
	defer asyncLogger.Stop()

	// Initialize Real-Time Model Pricing & Prompt-Cache Billing Engine
	billing.InitGlobalEngine(repo)

	// Initialize Control Plane Synchronizer & load state into Data Plane memory
	synchronizer := controlplane.NewSynchronizer(repo, dispatcher)
	if err := synchronizer.ReloadFromDB(); err != nil {
		telemetry.Logger.Warn("initial sync from db failed, using config file defaults", "error", err.Error())
		dispatcher.UpdateChannels(cfg.Channels)
	}

	// Initialize Enterprise Distributed Redis Layer (if REDIS_URL configured)
	redisClient := distributed.InitRedis(cfg.Server.RedisURL)
	if redisClient != nil && redisClient.IsActive() {
		stopRedis := make(chan struct{})
		defer close(stopRedis)
		redisClient.SubscribeReload(func(reason string) {
			telemetry.Logger.Info("handling cluster-wide reload broadcast (<1ms latency)", "reason", reason)
			if err := synchronizer.ReloadFromDB(); err != nil {
				telemetry.Logger.Error("failed to reload data plane after cluster broadcast", "error", err.Error())
			}
		}, stopRedis)
	}

	// HA Multi-Replica Periodic Auto-Sync (10s interval as baseline fallback)
	stopSync := make(chan struct{})
	defer close(stopSync)
	synchronizer.StartPeriodicSync(10*time.Second, stopSync)

	// Initialize Admin Handler
	adminHandler := controlplane.NewAdminHandler(repo, synchronizer, dispatcher)

	// Setup HTTP Engine (Data Plane + Control Plane Admin API + Embedded Web UI)
	engine := api.SetupRouter(dispatcher, adminHandler)

	addr := fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.Port)
	srv := &http.Server{
		Addr:         addr,
		Handler:      engine,
		ReadTimeout:  time.Duration(cfg.Server.ReadTimeoutSec) * time.Second,
		WriteTimeout: time.Duration(cfg.Server.WriteTimeoutSec) * time.Second,
	}

	// Run server in background goroutine
	go func() {
		telemetry.Logger.Info(fmt.Sprintf("🚀 Nano-Gateway listening on http://%s", addr))
		telemetry.Logger.Info(fmt.Sprintf("🌐 Web UI available at: http://%s/ui/", addr))
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			telemetry.Logger.Error("server fatal error", "error", err.Error())
			os.Exit(1)
		}
	}()

	// Graceful shutdown on SIGINT or SIGTERM
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	telemetry.Logger.Info("shutting down nano-gateway gracefully...")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		telemetry.Logger.Error("server forced to shutdown", "error", err.Error())
	}

	telemetry.Logger.Info("nano-gateway exited smoothly.")
}
