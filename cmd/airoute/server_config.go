package main

import (
	"flag"
	"fmt"
	"net/url"
	"os"
	"strings"

	"github.com/ifnodoraemon/airoute/internal/config"
	"github.com/ifnodoraemon/airoute/internal/telemetry"
)

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

func serverFlags(args []string) (string, string) {
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

	return *configPath, *dbPath
}

func loadServerSettings(args []string) (*config.Config, string) {
	configPath, dbPath := serverFlags(args)
	// Load file configuration
	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "加载配置文件失败: %v\n", err)
		os.Exit(1)
	}

	// Apply the env overrides for the values the config file also carries (see
	// applyServerEnvOverrides for why the timeout/log-level knobs need one).
	applyServerEnvOverrides(cfg)

	// Initialize Logger
	telemetry.InitLogger(cfg.Server.LogLevel)

	// The effective store is DATABASE_URL when set, otherwise the SQLite -db path.
	// Log the redacted effective source: the old line always printed the SQLite
	// flag (so a PostgreSQL deployment logged a SQLite path), and logging
	// DATABASE_URL verbatim would leak the database password into the log stream.
	dataSource := dbPath
	if dbEnv := os.Getenv("DATABASE_URL"); dbEnv != "" {
		dataSource = dbEnv
	}
	storageKind, storageTarget := describeDataSource(dataSource)
	telemetry.ScopedLogger("system").Info("starting airoute",
		"version", Version,
		"config", configPath,
		"storage", storageKind,
		"data_source", storageTarget,
	)

	return cfg, dataSource
}

func warnServerConfiguration(cfg *config.Config, dataSource string) {
	// Security and deployment warnings on startup.
	//
	// The credential check is fail-closed: a forgeable admin signing key or the
	// shipped admin123 password is never acceptable in a running gateway. Earlier
	// versions only refused in "release mode" (GIN_MODE=release or log_level
	// warn/error), which the shipped Compose/Swarm files never set, so the guard
	// silently did nothing on the deployments it was written for. Local
	// development can opt out explicitly with GATEWAY_ALLOW_INSECURE_DEFAULTS.
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

}
