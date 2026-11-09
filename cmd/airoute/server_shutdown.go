package main

import (
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/ifnodoraemon/airoute/internal/telemetry"
)

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
