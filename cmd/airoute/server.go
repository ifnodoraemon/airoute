package main

import (
	"context"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/ifnodoraemon/airoute/internal/api"
	"github.com/ifnodoraemon/airoute/internal/billing"
	"github.com/ifnodoraemon/airoute/internal/config"
	"github.com/ifnodoraemon/airoute/internal/controlplane"
	"github.com/ifnodoraemon/airoute/internal/distributed"
	"github.com/ifnodoraemon/airoute/internal/router"
	"github.com/ifnodoraemon/airoute/internal/storage"
	"github.com/ifnodoraemon/airoute/internal/telemetry"
)

// runServer starts the authoritative AI Gateway server and control plane.
func runServer(args []string) {
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

	fs := flag.NewFlagSet("server", flag.ExitOnError)
	configPath := fs.String("config", defaultConfig, "Path to YAML configuration file")
	dbPath := fs.String("db", defaultDB, "Path to SQLite database file")
	_ = fs.Parse(args)

	// Load file configuration
	cfg, err := config.LoadConfig(*configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "加载配置文件失败: %v\n", err)
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
	telemetry.ScopedLogger("system").Info("starting airoute",
		"version", Version,
		"config", *configPath,
		"db", *dbPath,
	)

	dataSource := *dbPath
	if dbEnv := os.Getenv("DATABASE_URL"); dbEnv != "" {
		dataSource = dbEnv
	}

	// Security and deployment warnings on startup
	isRelease := os.Getenv("GIN_MODE") == "release" || cfg.Server.LogLevel == "warn" || cfg.Server.LogLevel == "error"
	if issues := productionSecurityIssues(cfg); len(issues) > 0 {
		for _, issue := range issues {
			telemetry.ScopedLogger("system").Warn("SECURITY WARNING: "+issue, "remediation", "set GATEWAY_ADMIN_SECRET and GATEWAY_ADMIN_PASSWORD to strong random values")
		}
		if isRelease {
			telemetry.ScopedLogger("system").Error("refusing to start: insecure default credentials detected in release mode; set GATEWAY_ADMIN_SECRET and GATEWAY_ADMIN_PASSWORD")
			os.Exit(1)
		}
	}
	if isRelease && os.Getenv("STRIPE_WEBHOOK_SECRET") == "" {
		telemetry.ScopedLogger("system").Warn("SECURITY WARNING: STRIPE_WEBHOOK_SECRET is unset in production release mode. Stripe payment webhooks will be rejected.")
	}
	// Without a shared secret the HMAC signing key is generated randomly per
	// process: every restart invalidates all issued tokens, and in a multi-replica
	// cluster a token minted by one node is rejected by every other node.
	if os.Getenv("GATEWAY_ADMIN_SECRET") == "" {
		telemetry.ScopedLogger("system").Warn("CONFIGURATION WARNING: GATEWAY_ADMIN_SECRET is unset; using a per-process random signing key. " +
			"User sessions will not survive a restart and will not be valid across cluster replicas.")
	}
	if !strings.HasPrefix(dataSource, "postgres://") && !strings.HasPrefix(dataSource, "postgresql://") {
		if os.Getenv("KUBERNETES_SERVICE_HOST") != "" {
			telemetry.ScopedLogger("system").Warn("STORAGE WARNING: Running in Kubernetes with SQLite file storage. For multi-replica deployments (replicas > 1), configure PostgreSQL via DATABASE_URL to avoid database lock contention.")
		}
	}

	// Initialize Storage Layer (SQLite or Distributed PostgreSQL)
	db, err := storage.OpenDB(dataSource)
	if err != nil {
		telemetry.ScopedLogger("system").Error("failed to open database", "error", err.Error())
		os.Exit(1)
	}
	defer db.Close()

	repo := storage.NewRepository(db)

	// Ensure default admin user account exists
	controlplane.InitDefaultAdmin(repo, cfg)

	// Seed DB from YAML config if DB is currently empty
	existingChannels, _ := repo.ListChannels()
	if len(existingChannels) == 0 && len(cfg.Channels) > 0 {
		telemetry.ScopedLogger("system").Info("seeding database from initial config file", "channels", len(cfg.Channels))
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

	existingKeys, _ := repo.ListAPIKeys()
	if len(existingKeys) == 0 && len(cfg.APIKeys) > 0 {
		for _, k := range cfg.APIKeys {
			_ = repo.CreateAPIKey(&storage.APIKeyRecord{
				Key:           k.Key,
				TenantID:      k.TenantID,
				AllowedModels: k.AllowedModels,
				RPM:           k.RPM,
				TPM:           k.TPM,
				Budget:        k.Budget,
			})
		}
	}

	// Initialize Data Plane Dispatcher
	dispatcher := router.NewDispatcher(nil)

	// Initialize Enterprise Distributed Redis Layer FIRST (required by AsyncLogger for stream consumption)
	redisClient := distributed.InitRedis(cfg.Server.RedisURL)

	// Publish this node's in-flight telemetry snapshot every second so the
	// dashboard's live throughput endpoint can aggregate across replicas.
	// No-op without Redis; the endpoint then serves node-local values.
	stopLiveStats := make(chan struct{})
	defer close(stopLiveStats)
	distributed.StartLiveStatsPublisher(redisClient, stopLiveStats)

	// Initialize Async Usage Logger for zero-latency audit logs
	// NOTE: Must be after Redis init so redisStreamWorker can start consuming the stream
	asyncLogger := storage.InitAsyncLogger(repo, 10000, 100, 500*time.Millisecond)
	defer asyncLogger.Stop()

	// Initialize Real-Time Model Pricing & Prompt-Cache Billing Engine
	billing.InitGlobalEngine(repo)

	// Initialize redacted request/response trace archiving for troubleshooting
	traceCapture := storage.InitTraceCapture(repo)
	defer traceCapture.Stop()

	// Prune audit rows beyond the retention window (GATEWAY_USAGE_LOG_RETENTION_DAYS,
	// 0 disables). Without this usage_logs only ever grew.
	usageLogRetention := storage.InitUsageLogRetention(repo)
	defer usageLogRetention.Stop()

	// Compare the billing ledgers periodically so drift becomes a metric/alert
	// instead of something an operator has to discover by hand.
	billingReconciler := storage.InitBillingReconcile(repo)
	defer billingReconciler.Stop()

	// Initialize Control Plane Synchronizer & load state into Data Plane memory
	synchronizer := controlplane.NewSynchronizer(repo, dispatcher)
	if err := synchronizer.ReloadFromDB(); err != nil {
		telemetry.ScopedLogger("system").Warn("initial sync from db failed, using config file defaults", "error", err.Error())
		dispatcher.UpdateChannels(cfg.Channels)
	}

	// Subscribe to cluster-wide reload broadcasts via Redis Pub/Sub
	if redisClient != nil && redisClient.IsActive() {
		stopRedis := make(chan struct{})
		defer close(stopRedis)
		redisClient.SubscribeReload(func(reason string) {
			telemetry.ScopedLogger("system").Info("handling cluster-wide reload broadcast (<1ms latency)", "reason", reason)
			if err := synchronizer.ReloadFromDB(); err != nil {
				telemetry.ScopedLogger("system").Error("failed to reload data plane after cluster broadcast", "error", err.Error())
			}
		}, stopRedis)
	}

	// HA Multi-Replica Periodic Auto-Sync (10s interval as baseline fallback)
	stopSync := make(chan struct{})
	defer close(stopSync)
	synchronizer.StartPeriodicSync(10*time.Second, stopSync)

	// Initialize Pluggable Artifact Storage (Local or Distributed RustFS / S3)
	storageCfg := cfg.GetStorageConfig()
	artifactStorage, err := storage.NewArtifactStorage(storageCfg)
	if err != nil {
		telemetry.ScopedLogger("system").Warn("failed to initialize configured artifact storage, falling back to local storage", "error", err.Error())
		artifactStorage, _ = storage.NewLocalStorage(storageCfg.LocalPath)
	}
	telemetry.ScopedLogger("system").Info("artifact storage initialized", "driver", artifactStorage.Driver())

	// Initialize Admin Handler
	adminHandler := controlplane.NewAdminHandler(repo, synchronizer, dispatcher)
	adminHandler.SetArtifactStorage(artifactStorage)

	// Setup HTTP Engine (Data Plane + Control Plane Admin API + Embedded Web UI)
	engine := api.SetupRouter(dispatcher, adminHandler)

	addr := fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.Port)
	srv := &http.Server{
		Addr:              addr,
		Handler:           engine,
		ReadTimeout:       time.Duration(cfg.Server.ReadTimeoutSec) * time.Second,
		ReadHeaderTimeout: 15 * time.Second,
		WriteTimeout:      time.Duration(cfg.Server.WriteTimeoutSec) * time.Second,
		IdleTimeout:       120 * time.Second,
		MaxHeaderBytes:    1 << 20, // 1 MiB of headers is far beyond any legitimate request
	}

	// Run server in background goroutine
	go func() {
		telemetry.ScopedLogger("system").Info("gateway server listening", "addr", addr)
		if pub := cfg.GetPublicURL(); pub != "" {
			telemetry.ScopedLogger("system").Info("gateway public endpoint configured", "url", pub)
		}

		printStartupBanner(addr, cfg.GetPublicURL())

		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			telemetry.ScopedLogger("system").Error("server fatal error", "error", err.Error())
			os.Exit(1)
		}
	}()

	// Graceful shutdown on SIGINT or SIGTERM
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	telemetry.ScopedLogger("system").Info("shutting down airoute gracefully")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		telemetry.ScopedLogger("system").Error("server forced to shutdown", "error", err.Error())
	}

	telemetry.ScopedLogger("system").Info("airoute exited smoothly")
}

// printStartupBanner prints the human-facing readiness banner, but only when
// stdout is a terminal.
//
// Under Docker/Compose/Kubernetes stdout is a pipe, and the colored multi-line
// banner is not JSON: it breaks `jq`, log-shipper JSON parsing and line-oriented
// alerting rules. Operators still see everything they need — "gateway server
// listening" and "gateway public endpoint configured" are emitted as structured
// logs — so the banner is only worth printing for an interactive `airoute run`.
func printStartupBanner(addr, publicURL string) {
	info, err := os.Stdout.Stat()
	if err != nil || info.Mode()&os.ModeCharDevice == 0 {
		return
	}
	fmt.Printf("\n%s🚀 Airoute 网关已成功就绪%s\n", colorBold+colorGreen, colorReset)
	fmt.Printf("  • 网关服务端口: %shttp://%s%s\n", colorCyan, addr, colorReset)
	fmt.Printf("  • 控制台工作台: %shttp://%s/app/%s (或 http://%s/)\n", colorCyan, addr, colorReset, addr)
	if publicURL != "" {
		fmt.Printf("  • 外部公共端点: %s%s%s\n", colorCyan, publicURL, colorReset)
	}
	fmt.Println()
}
