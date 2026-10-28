package telemetry

import (
	"context"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"
)

// ContextKeyTraceID is the gin.Context key under which the HTTP trace middleware
// stores the request trace id. gin only accepts string keys, so the value is a
// plain constant; readers of a Go context.Context must use WithTraceID instead.
const ContextKeyTraceID = "trace_id"

// traceIDContextKey is the context.Context key for the trace id. It is a private
// struct type rather than a string so that a value stored by another package
// under a "trace_id" string key can never be mistaken for the request trace id.
type traceIDContextKey struct{}

// WithTraceID returns a child context carrying traceID for trace-aware log lines.
func WithTraceID(ctx context.Context, traceID string) context.Context {
	return context.WithValue(ctx, traceIDContextKey{}, traceID)
}

// TraceIDFromContext returns the request trace id carried by ctx, or "" when the
// context has none (background jobs, startup paths). Callers pass it as a
// structured attribute so one request's lines can be joined together.
func TraceIDFromContext(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	traceID, _ := ctx.Value(traceIDContextKey{}).(string)
	return traceID
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
