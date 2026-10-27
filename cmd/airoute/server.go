package main

import (
	"context"
	"flag"
	"fmt"
	"net/http"
	"net/url"
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
	"github.com/ifnodoraemon/airoute/internal/middleware"
	"github.com/ifnodoraemon/airoute/internal/router"
	"github.com/ifnodoraemon/airoute/internal/storage"
	"github.com/ifnodoraemon/airoute/internal/telemetry"
)

// runServer starts the authoritative AI Gateway server and control plane.
func runServer(args []string) {
	// Install the signal handler before the potentially slow initialization (DB
	// open, migrations, seeding, Redis bootstrap). Previously it was registered
	// only after the listener started, so a SIGTERM during startup killed the
	// process immediately: deferred cleanup (audit flush, DB close) never ran.
	// The channel has room for a second signal so a repeated Ctrl-C cannot block.
	quit := make(chan os.Signal, 2)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	defer signal.Stop(quit)
	shutdownRequested := func() bool {
		select {
		case <-quit:
			return true
		default:
			return false
		}
	}

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

	// The effective store is DATABASE_URL when set, otherwise the SQLite -db path.
	// Log the redacted effective source: the old line always printed the SQLite
	// flag (so a PostgreSQL deployment logged a SQLite path), and logging
	// DATABASE_URL verbatim would leak the database password into the log stream.
	dataSource := *dbPath
	if dbEnv := os.Getenv("DATABASE_URL"); dbEnv != "" {
		dataSource = dbEnv
	}
	storageKind, storageTarget := describeDataSource(dataSource)
	telemetry.ScopedLogger("system").Info("starting airoute",
		"version", Version,
		"config", *configPath,
		"storage", storageKind,
		"data_source", storageTarget,
	)

	// Security and deployment warnings on startup.
	//
	// The credential check is fail-closed: a forgeable admin signing key or the
	// shipped admin123 password is never acceptable in a running gateway. Earlier
	// versions only refused in "release mode" (GIN_MODE=release or log_level
	// warn/error), which the shipped Compose/Swarm files never set, so the guard
	// silently did nothing on the deployments it was written for. Local
	// development can opt out explicitly with GATEWAY_ALLOW_INSECURE_DEFAULTS.
	isRelease := os.Getenv("GIN_MODE") == "release" || cfg.Server.LogLevel == "warn" || cfg.Server.LogLevel == "error"
	if issues := productionSecurityIssues(cfg); len(issues) > 0 {
		for _, issue := range issues {
			telemetry.ScopedLogger("system").Warn("SECURITY WARNING: "+issue, "remediation", "set GATEWAY_ADMIN_SECRET and GATEWAY_ADMIN_PASSWORD to strong random values")
		}
		if !allowInsecureDefaults() {
			telemetry.ScopedLogger("system").Error("refusing to start: insecure default credentials detected; set GATEWAY_ADMIN_SECRET and GATEWAY_ADMIN_PASSWORD",
				"escape_hatch", "GATEWAY_ALLOW_INSECURE_DEFAULTS=true (local development only)")
			os.Exit(1)
		}
		telemetry.ScopedLogger("system").Warn("starting with insecure default credentials because GATEWAY_ALLOW_INSECURE_DEFAULTS is enabled; never use this in production")
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

	// Readiness signal for orchestrators and scripts/docker-rebuild.sh:
	// /health stays a cheap process-liveness probe, while /health/ready also
	// verifies that storage answers (and, below, that Redis answers) so a replica
	// with a dead database never reports itself ready for traffic.
	api.RegisterReadinessCheck("database", func(ctx context.Context) error {
		return db.PingContext(ctx)
	})

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
	if redisClient != nil && redisClient.IsActive() {
		api.RegisterReadinessCheck("redis", func(ctx context.Context) error {
			return redisClient.GetRDB().Ping(ctx).Err()
		})
	}

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

	// Enable synchronous charge settlement: money moves at request completion and
	// no longer waits for the audit log to be persisted.
	storage.InitBillingSettlement(repo)

	// Bill by the model the upstream actually served (channel model mapping),
	// not by the client-facing alias.
	api.SetServedModelResolver(dispatcher.ResolveServedModel)

	// Verify the timeout numbers agree with each other and with the edge proxy.
	reportTimeoutChain(repo, dispatcher)

	// Token-estimate calibration describes the upstream tokenizers, so it must
	// survive restarts and be shared by both replicas.
	if err := billing.LoadCalibration(repo, billing.GlobalCalibrator); err != nil {
		telemetry.ScopedLogger("billing").Warn("failed to load token estimate calibration", "error", err.Error())
	}
	billing.GlobalCalibrator.SetPersistHook(func() error {
		return billing.SaveCalibration(repo, billing.GlobalCalibrator)
	})
	stopCalibration := make(chan struct{})
	defer close(stopCalibration)
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
	go func() {
		unpriced, err := billing.UnpricedModelsInUse(repo, billing.GlobalEngine, 7*24*time.Hour)
		if err != nil {
			telemetry.ScopedLogger("billing").Warn("pricing coverage check failed", "error", err.Error())
			return
		}
		if len(unpriced) == 0 {
			return
		}
		shown := unpriced
		if len(shown) > 20 {
			shown = shown[:20]
		}
		telemetry.ScopedLogger("billing").Warn("models served without a configured price; the fallback rate card is in use",
			"count", len(unpriced),
			"models", strings.Join(shown, ","))

		// Billing aligns with the provider invoice only if the model the provider
		// actually served is priced. Channel model mappings can turn a priced alias
		// into an unpriced upstream model.
		servedUnpriced, servedErr := billing.UnpricedServedModelsInUse(repo, billing.GlobalEngine, 7*24*time.Hour, dispatcher.ResolveServedModel)
		if servedErr != nil {
			telemetry.ScopedLogger("billing").Warn("served-model pricing check failed", "error", servedErr.Error())
			return
		}
		if len(servedUnpriced) == 0 {
			return
		}
		servedShown := servedUnpriced
		if len(servedShown) > 10 {
			servedShown = servedShown[:10]
		}
		telemetry.ScopedLogger("billing").Warn("upstream models served without their own price; charges fall back to the alias rate",
			"count", len(servedUnpriced),
			"served_models", strings.Join(servedShown, "; "))
	}()

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

	// Initialize Pluggable Artifact Storage (Local or Distributed RustFS / S3).
	// An unusable configuration (unknown driver, or s3/rustfs without explicit
	// endpoint/credentials) refuses to start instead of silently falling back to a
	// local directory like it used to.
	storageCfg := cfg.GetStorageConfig()
	if err := storageCfg.Validate(); err != nil {
		telemetry.ScopedLogger("system").Error("invalid storage configuration; refusing to start",
			"error", err.Error(),
			"hint", "local needs no credentials; s3/rustfs need STORAGE_S3_ENDPOINT and STORAGE_S3_ACCESS_KEY/STORAGE_S3_SECRET_KEY")
		os.Exit(1)
	}
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

	// Only configured proxy hops may declare the client address. Gin trusts every
	// peer by default, so an unset GATEWAY_TRUSTED_PROXIES would let any caller
	// choose the IP that the login lockout, the per-IP rate limiters and the
	// access log use.
	if err := middleware.ConfigureTrustedProxies(engine); err != nil {
		telemetry.ScopedLogger("system").Error("invalid GATEWAY_TRUSTED_PROXIES; refusing to start with forwarding headers trusted", "error", err.Error())
		os.Exit(1)
	}
	if !middleware.TrustedProxyConfigured() {
		telemetry.ScopedLogger("system").Warn("GATEWAY_TRUSTED_PROXIES is unset: X-Forwarded-For/X-Real-IP are ignored and every request is attributed to its TCP peer. Behind a reverse proxy set GATEWAY_TRUSTED_PROXIES to the proxy CIDRs, otherwise client IPs, per-IP rate limits and access logs show the proxy address.")
	}

	if shutdownRequested() {
		telemetry.ScopedLogger("system").Info("shutdown requested during startup; exiting before accepting connections")
		return
	}

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

	// Graceful shutdown on SIGINT or SIGTERM (handler installed at the top of
	// this function so startup is covered too).
	<-quit

	grace := shutdownTimeout()
	telemetry.ScopedLogger("system").Info("shutting down airoute gracefully", "timeout", grace.String())

	ctx, cancel := context.WithTimeout(context.Background(), grace)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		telemetry.ScopedLogger("system").Error("server forced to shutdown", "error", err.Error())
	}

	telemetry.ScopedLogger("system").Info("airoute exited smoothly")
}

// defaultDrainTimeoutSeconds mirrors the aborted-request drain budget default in
// internal/api (GATEWAY_DRAIN_TIMEOUT_SEC). Keeping it here makes the shutdown
// budget derivable from the drain it has to allow.
const defaultDrainTimeoutSeconds = 120

// drainBudget is the maximum time one aborted upstream request may keep draining
// after the downstream client disconnected.
func drainBudget() time.Duration {
	if raw := strings.TrimSpace(os.Getenv("GATEWAY_DRAIN_TIMEOUT_SEC")); raw != "" {
		if seconds, err := strconv.Atoi(raw); err == nil && seconds > 0 {
			return time.Duration(seconds) * time.Second
		}
	}
	return defaultDrainTimeoutSeconds * time.Second
}

// shutdownTimeout bounds http.Server.Shutdown after SIGTERM.
//
// The previous hardcoded 10s was shorter than the 120s aborted-drain budget and
// raced Docker's default 10s SIGKILL, so every deploy cut long generations and
// drain work mid-flight. GATEWAY_SHUTDOWN_TIMEOUT_SEC overrides the value; when
// unset it is derived from the drain budget plus a 15s audit-flush margin.
func shutdownTimeout() time.Duration {
	if raw := strings.TrimSpace(os.Getenv("GATEWAY_SHUTDOWN_TIMEOUT_SEC")); raw != "" {
		if seconds, err := strconv.Atoi(raw); err == nil && seconds > 0 {
			return time.Duration(seconds) * time.Second
		}
		telemetry.ScopedLogger("system").Warn("invalid GATEWAY_SHUTDOWN_TIMEOUT_SEC; deriving the shutdown budget from the drain budget",
			"value", raw)
	}
	return drainBudget() + 15*time.Second
}

// describeDataSource returns the storage kind ("postgres" or "sqlite") and a
// redacted target for the effective data source. DATABASE_URL contains the
// database password, so it must never be logged verbatim.
func describeDataSource(dsn string) (string, string) {
	if strings.HasPrefix(dsn, "postgres://") || strings.HasPrefix(dsn, "postgresql://") {
		parsed, err := url.Parse(dsn)
		if err != nil {
			return "postgres", "configured (unparsable DSN)"
		}
		target := parsed.Host
		if dbName := strings.TrimPrefix(parsed.Path, "/"); dbName != "" {
			target += "/" + dbName
		}
		return "postgres", target
	}
	return "sqlite", dsn
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
