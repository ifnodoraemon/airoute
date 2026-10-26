package router

import (
	"context"
	"testing"

	"github.com/ifnodoraemon/airoute/internal/telemetry"
)

// A routing decision must carry the request trace id so it can be joined with
// the access line. Before this, "attempt -> 429 -> breaker skip -> 502" produced
// unrelated lines and one client retrying N times was indistinguishable from N
// clients failing once.
func TestRouterLogCarriesTraceID(t *testing.T) {
	const traceID = "tr-router-corr-001"
	ctx := context.WithValue(context.Background(), telemetry.ContextKeyTraceID, traceID)

	routerLog(ctx).Warn("channel execution failed, triggering fallback",
		"channel", "ch-x", "attempt", 2, "error", "upstream 429")

	entries := telemetry.GlobalRingBuffer.GetRecentLogs(200, "", "", "router", traceID)
	if len(entries) == 0 {
		t.Fatal("no router log captured for the trace id: routing decisions are not correlatable")
	}
	found := false
	for _, entry := range entries {
		if entry.Message != "channel execution failed, triggering fallback" {
			continue
		}
		found = true
		if entry.TraceID != traceID {
			t.Fatalf("entry trace id = %q, want %q", entry.TraceID, traceID)
		}
		if got := entry.Attrs["attempt"]; got != int64(2) && got != 2 {
			t.Fatalf("attempt attribute = %v, want 2", got)
		}
	}
	if !found {
		t.Fatalf("expected the fallback warning in the ring buffer, got %+v", entries)
	}
}

// Without a trace id in the context the logger must still work (startup paths,
// background workers).
func TestRouterLogWithoutTraceID(t *testing.T) {
	routerLog(context.Background()).Debug("attempting chat completion", "channel", "ch-y")
}
