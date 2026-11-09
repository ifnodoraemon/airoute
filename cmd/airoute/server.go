package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/ifnodoraemon/airoute/internal/api"
	"github.com/ifnodoraemon/airoute/internal/billing"
	"github.com/ifnodoraemon/airoute/internal/config"
	"github.com/ifnodoraemon/airoute/internal/distributed"
	"github.com/ifnodoraemon/airoute/internal/router"
	"github.com/ifnodoraemon/airoute/internal/storage"
)

// runServer starts the authoritative AI Gateway server and control plane.
func runServer(args []string) {
	// Cover slow bootstrap too, so a signal cannot bypass deferred audit/DB cleanup.
	quit := make(chan os.Signal, 2)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	defer signal.Stop(quit)
	cfg, dataSource := loadServerSettings(args)
	warnServerConfiguration(cfg, dataSource)
	db, repo := openServerRepository(dataSource)
	defer db.Close()
	seedServerRepository(repo, cfg)
	runServerWithStorage(cfg, db, repo, quit)
}

func runServerWithStorage(cfg *config.Config, db *storage.DB, repo *storage.Repository, quit <-chan os.Signal) {
	dispatcher := router.NewDispatcher(nil)
	// Redis must precede AsyncLogger, which consumes its stream.
	redisClient := distributed.InitRedis(cfg.Server.RedisURL)
	if redisClient != nil && redisClient.IsActive() {
		api.RegisterReadinessCheck("redis", func(ctx context.Context) error {
			return redisClient.GetRDB().Ping(ctx).Err()
		})
	}
	stopLiveStats := make(chan struct{})
	defer close(stopLiveStats)
	distributed.StartLiveStatsPublisher(redisClient, stopLiveStats)
	asyncLogger := storage.InitAsyncLogger(repo, 10000, 100, 500*time.Millisecond)
	defer asyncLogger.Stop()
	billing.InitGlobalEngine(repo)
	storage.InitBillingSettlement(repo)
	settlementBatcher := storage.InitSettlementBatcher(repo)
	defer settlementBatcher.Stop()
	if stopCheckpointer := db.StartSQLiteCheckpointer(0); stopCheckpointer != nil {
		defer stopCheckpointer()
	}
	api.SetServedModelResolver(dispatcher.ResolveServedModel)
	reportTimeoutChain(repo, dispatcher, serverWriteTimeout(cfg))
	runServerWithBilling(cfg, repo, dispatcher, redisClient, quit)
}
