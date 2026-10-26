package main

import (
	"time"

	"github.com/ifnodoraemon/airoute/internal/model"
	"github.com/ifnodoraemon/airoute/internal/router"
	"github.com/ifnodoraemon/airoute/internal/storage"
	"github.com/ifnodoraemon/airoute/internal/telemetry"
)

// edgeProxyTimeout is the proxy budget the deployment declares. It comes from the
// model constant that also bounds every channel timeout, so there is one number in
// the system rather than three that have to agree by hand.
var edgeProxyTimeout = time.Duration(model.EdgeProxyTimeoutSeconds) * time.Second

// reportTimeoutChain logs the effective timeout budget for each active channel and
// warns when the configured numbers contradict each other.
//
// There is one operator-facing knob: the channel timeout. The first-token window is
// derived from it (they are the same number), and the edge proxy's timeout is set
// far above the maximum accepted channel timeout so it never truncates a request.
// This check exists to prove that invariant at startup instead of assuming it: the
// live cluster once ran a 3000s channel timeout behind a 600s nginx, so the
// configured budget never decided anything.
func reportTimeoutChain(repo *storage.Repository, dispatcher *router.Dispatcher) {
	if repo == nil || dispatcher == nil {
		return
	}
	edge := edgeProxyTimeout
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
			"edge_timeout", edge.String())

		if channelTimeout > time.Hour {
			// Not an error: the operator asked for it and the proxy allows it. It is
			// worth saying out loud though, because a silent upstream will hold the
			// request (and the client connection) for that whole window.
			telemetry.ScopedLogger("router").Warn("channel budget is unusually large",
				"channel", record.Name,
				"channel_timeout", channelTimeout.String())
		}
		if channelTimeout > edge {
			telemetry.ScopedLogger("router").Warn("channel timeout exceeds the edge proxy timeout; the proxy will cut requests first",
				"channel", record.Name,
				"channel_timeout", channelTimeout.String(),
				"edge_timeout", edge.String())
		}
	}
}
