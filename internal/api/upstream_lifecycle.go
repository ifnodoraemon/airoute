package api

import (
	"context"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/ifnodoraemon/airoute/internal/telemetry"
)

const (
	// defaultUpstreamDrainTimeout bounds how long an upstream call may continue
	// after the downstream has gone away.
	defaultUpstreamDrainTimeout = 2 * time.Minute
)

// upstreamDrainingEnabled reports whether an aborted downstream keeps the
// upstream request alive to capture its real usage.
//
// Billing follows the provider invoice, and the only way to know what the
// provider charged for an aborted request is to let it finish and read its usage
// report. The trade-off is real: the provider keeps generating (and billing) the
// completion the client walked away from. GATEWAY_DRAIN_ABORTED_UPSTREAM=false
// restores the cheaper cancel-and-estimate behaviour.
func upstreamDrainingEnabled() bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv("GATEWAY_DRAIN_ABORTED_UPSTREAM"))) {
	case "0", "false", "no", "off":
		return false
	}
	return true
}

// upstreamDrainTimeout reads GATEWAY_DRAIN_TIMEOUT_SEC (default 120s).
func upstreamDrainTimeout() time.Duration {
	raw := strings.TrimSpace(os.Getenv("GATEWAY_DRAIN_TIMEOUT_SEC"))
	if raw == "" {
		return defaultUpstreamDrainTimeout
	}
	seconds, err := strconv.Atoi(raw)
	if err != nil || seconds <= 0 {
		return defaultUpstreamDrainTimeout
	}
	return time.Duration(seconds) * time.Second
}

// upstreamRequestContext returns the context used for the provider call.
//
// The request budget belongs to the channel, not to this helper: the dispatcher
// bounds every provider attempt with the channel's timeout_seconds (and the
// documented default when a channel sets none). This context therefore carries
// no absolute deadline of its own — the previous implementation applied
// GATEWAY_DRAIN_TIMEOUT_SEC (120s in the shipped .env) as a hard deadline to
// every chat/responses/messages request, so any channel timeout above 120s was
// unreachable and every stream was cut at 120s.
//
// With draining enabled the context is detached from the downstream, so a client
// disconnect does not abort a provider call that has already started: the usage
// report still arrives and the charge matches the invoice. GATEWAY_DRAIN_TIMEOUT_SEC
// is applied only as that post-disconnect drain window. With draining disabled the
// old cancel-with-the-client behaviour is kept.
func upstreamRequestContext(c *gin.Context) (context.Context, context.CancelFunc) {
	if c == nil {
		return context.WithCancel(context.Background())
	}
	clientCtx := c.Request.Context()
	if !upstreamDrainingEnabled() {
		// Keep the original cancellation semantics: an abort cancels the upstream.
		return context.WithCancel(clientCtx)
	}
	detached, cancel := context.WithCancel(context.WithoutCancel(clientCtx))
	return withPostDisconnectDrain(clientCtx, detached, cancel, upstreamDrainTimeout())
}

// withPostDisconnectDrain returns ctx (an already-detached context) plus the
// combined cancel func. When the downstream goes away, the provider call is
// allowed to continue for drain before it is cancelled; before that point the
// caller's own budget (the channel timeout, applied by the dispatcher) decides.
//
// The watchdog goroutine exits as soon as the caller invokes the returned cancel
// (normal completion), so it does not outlive the request.
func withPostDisconnectDrain(clientCtx, detached context.Context, cancel context.CancelFunc, drain time.Duration) (context.Context, context.CancelFunc) {
	var once sync.Once
	done := make(chan struct{})
	cleanup := func() {
		once.Do(func() {
			close(done)
			cancel()
		})
	}
	go func() {
		select {
		case <-clientCtx.Done():
		case <-done:
			return
		}
		if drain <= 0 {
			cancel()
			return
		}
		timer := time.NewTimer(drain)
		defer timer.Stop()
		select {
		case <-timer.C:
			cancel()
		case <-done:
		case <-detached.Done():
		}
	}()
	return detached, cleanup
}

// UpstreamDrainTimeout exposes the configured post-disconnect drain window so the
// startup timeout-chain report can state the real budget (it is not a request
// deadline: the channel timeout is).
func UpstreamDrainTimeout() time.Duration { return upstreamDrainTimeout() }

// markAbortedDrain records that a request was drained after the downstream left.
func markAbortedDrain(completionTokens int) {
	telemetry.GlobalMetrics.AbortedDrainedRequests.Add(1)
	if completionTokens > 0 {
		telemetry.GlobalMetrics.AbortedDrainedCompletionTokens.Add(uint64(completionTokens))
	}
}
