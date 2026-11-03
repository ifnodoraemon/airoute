package distributed

import (
	"context"
	"strings"
	"testing"
	"time"
)

// TestRPMWindowIsStrictSixtySecondSlidingWindow pins the window semantics that
// both limiters now share.
//
// The limiter admits at most `rpm` requests per rolling 60s window: each admission
// is timestamped, timestamps older than the window are evicted before the count is
// taken, and the count is compared against the limit before the request is
// recorded. internal/middleware keeps the same (now-window, now] window locally
// (rpmWindowSpan mirrors rpmWindowMs), so the fail-open path is as strict as the
// cluster one - a Redis outage costs the cross-replica view, not quota.
func TestRPMWindowIsStrictSixtySecondSlidingWindow(t *testing.T) {
	if rpmWindowMs != 60_000 {
		t.Fatalf("rpmWindowMs = %d, want 60000 (one minute)", rpmWindowMs)
	}

	// Boundary: the window is (now-window, now], because the script evicts
	// everything at or before now-window and ZREMRANGEBYSCORE is inclusive. An
	// admission exactly one window old therefore no longer counts, which is what
	// makes it a sliding window rather than a fixed bucket that resets in bursts.
	if got, want := time.Duration(rpmWindowMs)*time.Millisecond, time.Minute; got != want {
		t.Fatalf("window = %v, want %v", got, want)
	}

	// The window has to reach the script as an argument; a hardcoded literal there
	// is exactly how the Go side and the Lua side drift apart.
	script := slidingWindowLuaSrc
	for _, fragment := range []string{
		"local window = tonumber(ARGV[3])",
		"local clearBefore = now - window",
		"ZREMRANGEBYSCORE",
		"ZCARD",
		"ZADD",
	} {
		if !strings.Contains(script, fragment) {
			t.Fatalf("sliding window script no longer contains %q:\n%s", fragment, script)
		}
	}
	if strings.Contains(script, "60000") {
		t.Fatalf("window must come from ARGV, not a literal in the script:\n%s", script)
	}

	// Evicting before counting, then counting before admitting, is what makes the
	// limit apply to the current window instead of the previous one.
	evict := strings.Index(script, "ZREMRANGEBYSCORE")
	count := strings.Index(script, "ZCARD")
	add := strings.Index(script, "ZADD")
	if !(evict < count && count < add) {
		t.Fatalf("expected evict(%d) < count(%d) < admit(%d) ordering", evict, count, add)
	}
}

// With no Redis client the limiter must not reject: RPM is enforced by the local
// tracker in that mode, and AllowRPM's own contract is fail-open.
func TestAllowRPMWithoutRedisFailsOpen(t *testing.T) {
	var nilClient *Client
	allowed, err := nilClient.AllowRPM(context.Background(), "sk-test", 60)
	if !allowed || err != nil {
		t.Fatalf("nil client: allowed=%v err=%v, want true/nil", allowed, err)
	}

	inactive := &Client{}
	if allowed, err = inactive.AllowRPM(context.Background(), "sk-test", 60); !allowed || err != nil {
		t.Fatalf("inactive client: allowed=%v err=%v, want true/nil", allowed, err)
	}

	// An unmetered key (rpm <= 0) is never rate limited, with or without Redis.
	if allowed, err = inactive.AllowRPM(context.Background(), "sk-test", 0); !allowed || err != nil {
		t.Fatalf("unmetered key: allowed=%v err=%v, want true/nil", allowed, err)
	}
}
