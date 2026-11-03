package main

import (
	"time"

	"github.com/ifnodoraemon/airoute/internal/api"
	"github.com/ifnodoraemon/airoute/internal/config"
	"github.com/ifnodoraemon/airoute/internal/model"
	"github.com/ifnodoraemon/airoute/internal/router"
	"github.com/ifnodoraemon/airoute/internal/storage"
	"github.com/ifnodoraemon/airoute/internal/telemetry"
)

// edgeProxyTimeout is the proxy budget the deployment declares. It comes from the
// model constant that also bounds every channel timeout, so there is one number in
// the system rather than three that have to agree by hand.
var edgeProxyTimeout = time.Duration(model.EdgeProxyTimeoutSeconds) * time.Second

// serverWriteTimeout is the Go http.Server write deadline derived from the loaded
// config. It is the layer that actually ends a non-streaming request (streaming
// responses clear the deadline in InitSSEStream), yet the startup self-check used to
// ignore it: it printed an "edge 86400s, channel 3600s" all-clear while the gateway
// itself cut the response at write_timeout_sec (600s in the shipped config, 180s in
// the Helm chart).
//
// server.go takes the http.Server value from this function too, so the number the
// report prints cannot drift from the number the server enforces.
func serverWriteTimeout(cfg *config.Config) time.Duration {
	if cfg == nil || cfg.Server.WriteTimeoutSec <= 0 {
		return 0
	}
	return time.Duration(cfg.Server.WriteTimeoutSec) * time.Second
}

// writeTimeoutCutsChannel reports the non-streaming truncation risk: a channel
// budget larger than the server's write deadline means the gateway ends the request
// before the upstream or the edge proxy ever gets the chance to.
func writeTimeoutCutsChannel(channelTimeout, writeTimeout time.Duration) bool {
	return writeTimeout > 0 && channelTimeout > writeTimeout
}

// reportTimeoutChain logs the effective timeout budget for each active channel and
// warns when the configured numbers contradict each other.
//
// There is one operator-facing knob: the channel timeout. It bounds the whole
// provider attempt for every protocol (streaming, non-streaming and modality
// calls), the first-token window is derived from it (GATEWAY_FIRST_TOKEN_TIMEOUT
// can only shorten it), and the edge proxy's timeout is set far above the maximum
// accepted channel timeout so it never truncates a request. The drain window
// (GATEWAY_DRAIN_TIMEOUT_SEC) is reported too, but it is NOT part of this budget:
// it only starts after the downstream has disconnected, so it cannot cap a
// request. This check exists to prove that invariant at startup instead of
// assuming it: the live cluster once ran a 3000s channel timeout behind a 600s
// nginx, so the configured budget never decided anything.
//
// writeTimeout is the second layer that can end a request: the Go server's own
// write deadline, which bounds non-streaming responses. It is compared here
// because a channel timeout above it means the gateway - not the upstream and not
// the edge - is the component that cuts the request, and nothing else reports it.
func reportTimeoutChain(repo *storage.Repository, dispatcher *router.Dispatcher, writeTimeout time.Duration) {
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
			"server_write_timeout", writeTimeout.String(),
			"edge_timeout", edge.String(),
			"abort_drain", api.UpstreamDrainTimeout().String(),
			"abort_drain_note", "post-disconnect only; not a request deadline")

		if channelTimeout > time.Hour {
			// Not an error: the operator asked for it and the proxy allows it. It is
			// worth saying out loud though, because a silent upstream will hold the
			// request (and the client connection) for that whole window.
			telemetry.ScopedLogger("router").Warn("channel budget is unusually large",
				"channel", record.Name,
				"channel_timeout", channelTimeout.String())
		}
		if writeTimeoutCutsChannel(channelTimeout, writeTimeout) {
			telemetry.ScopedLogger("router").Warn("channel timeout exceeds the server write timeout; non-streaming requests will be cut by the gateway first",
				"channel", record.Name,
				"channel_timeout", channelTimeout.String(),
				"server_write_timeout", writeTimeout.String(),
				"note", "raise GATEWAY_WRITE_TIMEOUT_SEC / config.server.write_timeout_sec, or lower the channel timeout")
		}
		if channelTimeout > edge {
			telemetry.ScopedLogger("router").Warn("channel timeout exceeds the edge proxy timeout; the proxy will cut requests first",
				"channel", record.Name,
				"channel_timeout", channelTimeout.String(),
				"edge_timeout", edge.String())
		}
	}
}
