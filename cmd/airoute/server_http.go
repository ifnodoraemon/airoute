package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/ifnodoraemon/airoute/internal/api"
	"github.com/ifnodoraemon/airoute/internal/config"
	"github.com/ifnodoraemon/airoute/internal/controlplane"
	"github.com/ifnodoraemon/airoute/internal/middleware"
	"github.com/ifnodoraemon/airoute/internal/router"
	"github.com/ifnodoraemon/airoute/internal/storage"
	"github.com/ifnodoraemon/airoute/internal/telemetry"
)

func runServerHTTP(cfg *config.Config, repo *storage.Repository, dispatcher *router.Dispatcher, synchronizer *controlplane.Synchronizer, quit <-chan os.Signal) {
	adminHandler := newServerAdminHandler(cfg, repo, dispatcher, synchronizer)
	engine := api.SetupRouter(dispatcher, adminHandler)
	jobsCtx, stopJobs := context.WithCancel(context.Background())
	defer stopJobs()
	api.StartNativeJobReconciler(jobsCtx, dispatcher)
	paymentsDone := make(chan struct{})
	go func() {
		defer close(paymentsDone)
		adminHandler.RunPaymentReconciler(jobsCtx)
	}()
	defer func() { stopJobs(); <-paymentsDone }()
	configureServerProxies(engine)
	select {
	case <-quit:
		telemetry.ScopedLogger("system").Info("shutdown requested during startup; exiting before accepting connections")
		return
	default:
	}
	addr := fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.Port)
	srv := newGatewayHTTPServer(cfg, addr, engine)
	startGatewayListener(srv, cfg, addr)
	<-quit
	shutdownGatewayHTTPServer(srv)
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

func newServerAdminHandler(cfg *config.Config, repo *storage.Repository, dispatcher *router.Dispatcher, synchronizer *controlplane.Synchronizer) *controlplane.AdminHandler {
	artifactStorage := initServerArtifactStorage(cfg)
	// Initialize Admin Handler
	adminHandler := controlplane.NewAdminHandler(repo, synchronizer, dispatcher)
	adminHandler.SetArtifactStorage(artifactStorage)
	if err := adminHandler.ValidatePaymentConfiguration(); err != nil {
		telemetry.ScopedLogger("system").Error("invalid payment configuration; refusing to start", "error", err.Error())
		os.Exit(1)
	}

	return adminHandler
}

func initServerArtifactStorage(cfg *config.Config) storage.ArtifactStorage {
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

	return artifactStorage
}

func configureServerProxies(engine *gin.Engine) {
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

}

func newGatewayHTTPServer(cfg *config.Config, addr string, handler http.Handler) *http.Server {
	srv := &http.Server{
		Addr:              addr,
		Handler:           handler,
		ReadTimeout:       time.Duration(cfg.Server.ReadTimeoutSec) * time.Second,
		ReadHeaderTimeout: 15 * time.Second,
		WriteTimeout:      serverWriteTimeout(cfg),
		IdleTimeout:       120 * time.Second,
		MaxHeaderBytes:    1 << 20, // 1 MiB of headers is far beyond any legitimate request
	}

	return srv
}

func startGatewayListener(srv *http.Server, cfg *config.Config, addr string) {

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

}

func shutdownGatewayHTTPServer(srv *http.Server) {
	grace := shutdownTimeout()
	telemetry.ScopedLogger("system").Info("shutting down airoute gracefully", "timeout", grace.String())

	ctx, cancel := context.WithTimeout(context.Background(), grace)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		telemetry.ScopedLogger("system").Error("server forced to shutdown", "error", err.Error())
	}

	telemetry.ScopedLogger("system").Info("airoute exited smoothly")
}
