package router

import (
	"context"
	"log/slog"

	"github.com/ifnodoraemon/airoute/internal/telemetry"
)

// routerLog returns the router-category logger with the request trace id bound.
//
// Every routing line used to be emitted without the trace id, so a request that
// went "attempt on channel A -> 429 -> breaker skip -> attempt on channel B ->
// 502" produced several unrelated log lines and an operator could not join them
// back into one request (nor tell one client retrying 43 times apart from 43
// clients failing once). With the trace id attached, the access-log line and
// every routing decision share one key.
//
// The trace id is also propagated into the log ring buffer (BroadcastHandler
// reads the "trace_id" attribute), so the console's system-log search by
// trace_id works for routing decisions too.
func routerLog(ctx context.Context) *slog.Logger {
	traceID := telemetry.TraceIDFromContext(ctx)
	if traceID == "" {
		return telemetry.ScopedLogger("router")
	}
	return telemetry.ScopedLogger("router").With("trace_id", traceID)
}
