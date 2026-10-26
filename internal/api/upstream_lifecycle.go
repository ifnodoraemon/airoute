package api

import (
	"context"
	"os"
	"strconv"
	"strings"
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
// With draining enabled it is detached from the request's cancellation, so a
// client disconnect no longer aborts the upstream call: the response still
// arrives and its usage can be billed exactly. The context is always bounded by
// the drain timeout, which is what stops a forgotten request from running
// forever.
func upstreamRequestContext(c *gin.Context) (context.Context, context.CancelFunc) {
	if c == nil {
		return context.WithTimeout(context.Background(), upstreamDrainTimeout())
	}
	base := context.WithoutCancel(c.Request.Context())
	if !upstreamDrainingEnabled() {
		// Keep the original context so an abort cancels the upstream as before.
		return context.WithTimeout(c.Request.Context(), upstreamDrainTimeout())
	}
	return context.WithTimeout(base, upstreamDrainTimeout())
}

// markAbortedDrain records that a request was drained after the downstream left.
func markAbortedDrain(completionTokens int) {
	telemetry.GlobalMetrics.AbortedDrainedRequests.Add(1)
	if completionTokens > 0 {
		telemetry.GlobalMetrics.AbortedDrainedCompletionTokens.Add(uint64(completionTokens))
	}
}
