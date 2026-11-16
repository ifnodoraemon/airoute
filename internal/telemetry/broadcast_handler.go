package telemetry

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
)

// BroadcastHandler is the slog handler that fans records out to stdout and the
// in-memory system-log ring buffer. Extracted from ring_buffer.go to keep the
// buffer implementation under the size red line.
type BroadcastHandler struct {
	next   slog.Handler
	buffer *LogRingBuffer
	level  slog.Level
	// attrs carries attributes attached via With/WithAttrs (a scoped logger's
	// category, for example). slog handlers are expected to remember them; this
	// wrapper used to forward them only to the stdout handler, so they never
	// reached the ring buffer.
	attrs []slog.Attr
}

// NewBroadcastHandler wraps stdout slog handler with memory ring buffer capture.
func NewBroadcastHandler(next slog.Handler, buffer *LogRingBuffer, level slog.Level) *BroadcastHandler {
	return &BroadcastHandler{
		next:   next,
		buffer: buffer,
		level:  level,
	}
}

func (h *BroadcastHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return level >= h.level
}

func (h *BroadcastHandler) Handle(ctx context.Context, r slog.Record) error {
	// First forward to underlying stdout JSON handler
	var err error
	if h.next != nil {
		err = h.next.Handle(ctx, r)
	}

	if h.buffer != nil {
		// Per-request access lines are already the audit log (usage_logs) and are
		// the single highest-volume source in the system. Keeping them in the
		// in-memory troubleshooting buffer evicted the rare, important events
		// (breaker trips, billing reloads, sync failures) within seconds of a
		// traffic burst. The console has a dedicated audit page for requests.
		if _, skip := ringBufferSkipMessages[r.Message]; skip {
			return err
		}
		attrs := make(map[string]any, len(h.attrs)+8)
		for _, a := range h.attrs {
			attrs[a.Key] = a.Value.Any()
		}
		r.Attrs(func(a slog.Attr) bool {
			attrs[a.Key] = a.Value.Any()
			return true
		})

		msg := r.Message
		// A call site that knows its subsystem attaches the category explicitly;
		// the message heuristic below is only a fallback for unlabelled sites.
		category := normalizeCategory(attrs["category"])
		if category == "" {
			category = categorizeLog(msg, attrs)
		}
		delete(attrs, "category")

		traceID := ""
		for _, key := range []string{"trace_id", "traceId"} {
			if v, ok := attrs[key]; ok {
				traceID = fmt.Sprintf("%v", v)
				break
			}
		}

		entry := &SystemLogEntry{
			Timestamp: r.Time,
			Level:     r.Level.String(),
			Message:   msg,
			TraceID:   traceID,
			Category:  category,
			Attrs:     attrs,
		}

		h.buffer.Append(entry)
	}

	return err
}

// ringBufferSkipMessages lists per-request messages that must not consume the
// in-memory system-log budget. They are persisted in usage_logs instead.
var ringBufferSkipMessages = map[string]struct{}{
	"http_access": {},
}

// knownLogCategories is the set the console renders. An explicit category outside
// it is ignored so a typo cannot invent a filter nobody can select.
var knownLogCategories = map[string]struct{}{
	"breaker": {},
	"prober":  {},
	"router":  {},
	"sync":    {},
	"auth":    {},
	"billing": {},
	"system":  {},
}

// normalizeCategory validates an explicit category attribute.
func normalizeCategory(raw any) string {
	value, ok := raw.(string)
	if !ok {
		return ""
	}
	category := strings.ToLower(strings.TrimSpace(value))
	if _, known := knownLogCategories[category]; !known {
		return ""
	}
	return category
}

func (h *BroadcastHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	var next slog.Handler
	if h.next != nil {
		next = h.next.WithAttrs(attrs)
	}
	merged := make([]slog.Attr, 0, len(h.attrs)+len(attrs))
	merged = append(merged, h.attrs...)
	merged = append(merged, attrs...)
	return &BroadcastHandler{
		next:   next,
		buffer: h.buffer,
		level:  h.level,
		attrs:  merged,
	}
}

func (h *BroadcastHandler) WithGroup(name string) slog.Handler {
	var next slog.Handler
	if h.next != nil {
		next = h.next.WithGroup(name)
	}
	return &BroadcastHandler{
		next:   next,
		buffer: h.buffer,
		level:  h.level,
		attrs:  h.attrs,
	}
}

// categorizeLog is the fallback classifier for call sites that do not attach an
// explicit category. Guessing a subsystem from message text is brittle (the same
// smell as recovering a channel name from an error string), so subsystem loggers
// should use ScopedLogger instead.
func categorizeLog(msg string, attrs map[string]any) string {
	msgLower := strings.ToLower(msg)
	if strings.Contains(msgLower, "breaker") || strings.Contains(msgLower, "circuit") {
		return "breaker"
	}
	if strings.Contains(msgLower, "probe") || strings.Contains(msgLower, "health") || strings.Contains(msgLower, "ping") {
		return "prober"
	}
	if strings.Contains(msgLower, "redis") || strings.Contains(msgLower, "sync") || strings.Contains(msgLower, "cluster") || strings.Contains(msgLower, "reload") {
		return "sync"
	}
	if strings.Contains(msgLower, "auth") || strings.Contains(msgLower, "token") || strings.Contains(msgLower, "permission") || strings.Contains(msgLower, "jwt") {
		return "auth"
	}
	if strings.Contains(msgLower, "billing") || strings.Contains(msgLower, "cost") || strings.Contains(msgLower, "balance") || strings.Contains(msgLower, "wallet") {
		return "billing"
	}
	if strings.Contains(msgLower, "route") || strings.Contains(msgLower, "failover") || strings.Contains(msgLower, "dispatch") || strings.Contains(msgLower, "channel") {
		return "router"
	}
	return "system"
}