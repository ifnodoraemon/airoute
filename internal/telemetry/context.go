package telemetry

import (
	"context"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"
)

// ContextKeyTraceID is the context key under which the HTTP trace middleware
// stores the request trace id.
//
// It is a plain string constant on purpose: low-level packages (router, billing,
// dispatcher) need to attach the trace id to their log lines, and they must be
// able to do so without importing the middleware package (which would invert the
// dependency and risk an import cycle).
const ContextKeyTraceID = "trace_id"

// TraceIDFromContext returns the request trace id carried by ctx, or "" when the
// context has none (background jobs, startup paths). Callers pass it as a
// structured attribute so one request's lines can be joined together.
func TraceIDFromContext(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	if value := ctx.Value(ContextKeyTraceID); value != nil {
		if traceID, ok := value.(string); ok {
			return traceID
		}
	}
	return ""
}

// warnGate remembers, per throttling key, when a warning was last emitted.
var warnGate sync.Map // key -> *atomic.Int64 (unix nanos)

// WarnLogger emits msg as WARN at most once per interval for the given key.
//
// Several call sites are per-request but describe conditions that last for
// minutes: an open circuit breaker was logged once per bypassed request (127
// lines in a two-second burst), and a Redis hiccup logged once per request and
// once per usage-log record. The result is a log storm exactly when the system
// is least healthy. Throttling keeps the first occurrence (the diagnostic one)
// and a periodic reminder, so the condition stays visible without hiding the
// rest of the log.
//
// The logger parameter preserves the caller's scoped category.
func WarnLogger(logger *slog.Logger, key string, interval time.Duration, msg string, args ...any) {
	if logger == nil {
		logger = ScopedLogger("system")
	}
	now := time.Now().UnixNano()
	slot, _ := warnGate.LoadOrStore(key, new(atomic.Int64))
	last := slot.(*atomic.Int64)
	previous := last.Load()
	if previous != 0 && now-previous < int64(interval) {
		return
	}
	// Only one goroutine wins the emit, so a burst cannot produce a small burst
	// of duplicate lines either.
	if !last.CompareAndSwap(previous, now) {
		return
	}
	logger.Warn(msg, args...)
}

// ResetWarnGate clears the throttle state. Tests use it to keep cases
// independent; production code never needs it.
func ResetWarnGate() {
	warnGate.Range(func(key, _ any) bool {
		warnGate.Delete(key)
		return true
	})
}
