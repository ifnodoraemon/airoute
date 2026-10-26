package main

import (
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/ifnodoraemon/airoute/internal/model"
	"github.com/ifnodoraemon/airoute/internal/router"
	"github.com/ifnodoraemon/airoute/internal/storage"
	"github.com/ifnodoraemon/airoute/internal/telemetry"
)

// defaultEdgeTimeoutBudget mirrors nginx's proxy_read_timeout in the shipped
// deployment. A channel timeout larger than this can never take effect: the edge
// cuts the connection first, so the larger number is a lie in the configuration.
const defaultEdgeTimeoutBudget = 600 * time.Second

// edgeTimeoutBudget reads GATEWAY_EDGE_TIMEOUT_SEC (default 600s).
func edgeTimeoutBudget() time.Duration {
	raw := strings.TrimSpace(os.Getenv("GATEWAY_EDGE_TIMEOUT_SEC"))
	if raw == "" {
		return defaultEdgeTimeoutBudget
	}
	seconds, err := strconv.Atoi(raw)
	if err != nil || seconds <= 0 {
		return defaultEdgeTimeoutBudget
	}
	return time.Duration(seconds) * time.Second
}

// reportTimeoutChain logs the effective timeout budget for each active channel and
// warns when the configured numbers contradict each other.
//
// The three knobs form a chain: the first-token wait bounds how long a single
// attempt may stay silent, the channel timeout bounds the whole upstream call, and
// the edge proxy bounds everything. When they are set independently they drift
// apart - the live cluster had a 3000s channel timeout behind a 600s nginx, so the
// configuration promised a budget that the edge silently truncated.
func reportTimeoutChain(repo *storage.Repository, dispatcher *router.Dispatcher) {
	if repo == nil || dispatcher == nil {
		return
	}
	edge := edgeTimeoutBudget()
	channels, err := repo.ListChannels()
	if err != nil {
		telemetry.ScopedLogger("router").Warn("timeout chain check skipped", "error", err.Error())
		return
	}
	for _, record := range channels {
		if record == nil || record.Status != "active" {
			continue
		}
		channelTimeout := time.Duration(record.TimeoutSeconds) * time.Second
		firstToken := dispatcher.GetFirstTokenTimeout(&model.ChannelConfig{
			Name:           record.Name,
			TimeoutSeconds: record.TimeoutSeconds,
		}, "")
		telemetry.ScopedLogger("router").Info("timeout chain",
			"channel", record.Name,
			"first_token", firstToken.String(),
			"channel_timeout", channelTimeout.String(),
			"edge_budget", edge.String())

		if channelTimeout > edge {
			telemetry.ScopedLogger("router").Warn("channel timeout exceeds the edge budget; the proxy will cut requests first",
				"channel", record.Name,
				"channel_timeout", channelTimeout.String(),
				"edge_budget", edge.String())
		}
		if firstToken >= channelTimeout {
			telemetry.ScopedLogger("router").Warn("first-token wait is not shorter than the channel timeout; it cannot take effect",
				"channel", record.Name,
				"first_token", firstToken.String(),
				"channel_timeout", channelTimeout.String())
		}
	}
}
