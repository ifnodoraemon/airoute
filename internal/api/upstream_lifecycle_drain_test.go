package api

import (
	"context"
	"net/http"
	"testing"
	"time"
)

// R-02: GATEWAY_DRAIN_TIMEOUT_SEC used to be applied as an absolute deadline to
// every chat/responses/messages request, so channel timeouts above it were
// unreachable and streams were cut at the drain value.
func TestUpstreamContextIsNotABlanketDeadline(t *testing.T) {
	t.Setenv("GATEWAY_DRAIN_ABORTED_UPSTREAM", "true")
	t.Setenv("GATEWAY_DRAIN_TIMEOUT_SEC", "1")

	c, _ := newTestContext(http.MethodPost, "/v1/chat/completions")
	ctx, cancel := upstreamRequestContext(c)
	defer cancel()

	if deadline, ok := ctx.Deadline(); ok {
		t.Fatalf("drain window applied as a request deadline (%v remaining): channel timeouts above it are unreachable", time.Until(deadline))
	}

	// The old implementation expired the provider context after the drain window
	// even though the customer was still connected.
	time.Sleep(1200 * time.Millisecond)
	if err := ctx.Err(); err != nil {
		t.Fatalf("provider context expired after the drain window while the client was still connected: %v", err)
	}
}

// R-02: the drain window starts only when the downstream goes away.
func TestUpstreamContextDrainsOnlyAfterDisconnect(t *testing.T) {
	t.Setenv("GATEWAY_DRAIN_ABORTED_UPSTREAM", "true")
	t.Setenv("GATEWAY_DRAIN_TIMEOUT_SEC", "1")

	c, _ := newTestContext(http.MethodPost, "/v1/chat/completions")
	clientCtx, cancelClient := context.WithCancel(c.Request.Context())
	c.Request = c.Request.WithContext(clientCtx)

	ctx, cancel := upstreamRequestContext(c)
	defer cancel()

	// Stay connected for longer than half the drain window first.
	time.Sleep(600 * time.Millisecond)
	if err := ctx.Err(); err != nil {
		t.Fatalf("provider context expired before any disconnect: %v", err)
	}

	cancelClient()
	cancelledAt := time.Now()

	// Draining means the provider call survives the disconnect for the configured
	// window instead of being cancelled immediately.
	time.Sleep(300 * time.Millisecond)
	if err := ctx.Err(); err != nil {
		t.Fatalf("provider context was cancelled with the downstream (%v), not drained", err)
	}

	deadline := time.After(3 * time.Second)
	for {
		if ctx.Err() != nil {
			break
		}
		select {
		case <-deadline:
			t.Fatalf("provider context never ended after the drain window")
		case <-time.After(10 * time.Millisecond):
		}
	}

	// With a 1s drain window and a disconnect 600ms into the request, the old
	// absolute deadline would have fired ~400ms after the disconnect.
	if elapsed := time.Since(cancelledAt); elapsed < 700*time.Millisecond {
		t.Fatalf("provider context ended %v after the disconnect: the drain window did not start at disconnect", elapsed)
	}
}

// Draining disabled keeps the old cancel-with-the-client behaviour.
func TestUpstreamContextFollowsDownstreamWhenDrainingDisabled(t *testing.T) {
	t.Setenv("GATEWAY_DRAIN_ABORTED_UPSTREAM", "false")

	c, _ := newTestContext(http.MethodPost, "/v1/chat/completions")
	clientCtx, cancelClient := context.WithCancel(c.Request.Context())
	c.Request = c.Request.WithContext(clientCtx)

	ctx, cancel := upstreamRequestContext(c)
	defer cancel()

	cancelClient()
	deadline := time.After(2 * time.Second)
	for {
		if ctx.Err() != nil {
			return
		}
		select {
		case <-deadline:
			t.Fatalf("provider context outlived the downstream although draining is disabled")
		case <-time.After(10 * time.Millisecond):
		}
	}
}
