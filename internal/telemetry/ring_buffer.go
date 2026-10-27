package telemetry

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"sync"
	"time"
)

// SystemLogEntry represents a single structured system runtime log line.
type SystemLogEntry struct {
	ID        int64          `json:"id"`
	Timestamp time.Time      `json:"timestamp"`
	Level     string         `json:"level"`
	Message   string         `json:"message"`
	NodeID    string         `json:"node_id"`
	TraceID   string         `json:"trace_id,omitempty"`
	Category  string         `json:"category,omitempty"` // "breaker", "prober", "router", "sync", "auth", "billing", "system"
	Attrs     map[string]any `json:"attrs,omitempty"`
}

// LogRingBuffer stores the latest N runtime logs in memory for troubleshooting.
type LogRingBuffer struct {
	mu          sync.RWMutex
	capacity    int
	entries     []*SystemLogEntry
	nextID      int64
	subscribers map[int64]chan *SystemLogEntry
	nextSubID   int64
	nodeID      string
}

// defaultNodeID resolves the node identity shown in the console's log view.
func defaultNodeID() string {
	if nodeID := os.Getenv("NODE_ID"); nodeID != "" {
		return nodeID
	}
	if hostname := os.Getenv("HOSTNAME"); hostname != "" {
		return hostname
	}
	return "airoute-gateway"
}

// GlobalRingBuffer holds the in-memory circular log buffer.
//
// It is a variable initializer rather than an init() function on purpose:
// init() functions run only after every package variable has been initialized,
// but the logger's own init() builds the BroadcastHandler that captures into
// this buffer. When this was assigned in init(), the logger bound a nil buffer
// and every line logged before the first explicit InitLogger call (config load,
// security warnings, startup failures) silently never reached the console.
//
// 5000 entries of real events: the buffer used to share its space with one
// http_access line per request, so during a 300 req/s burst it held about six
// seconds of history and an incident was evicted before anyone opened the
// console.
var GlobalRingBuffer = NewLogRingBuffer(5000, defaultNodeID())

// NewLogRingBuffer creates a thread-safe circular log buffer with subscriber broadcasting.
func NewLogRingBuffer(capacity int, nodeID string) *LogRingBuffer {
	if capacity <= 0 {
		capacity = 2000
	}
	return &LogRingBuffer{
		capacity:    capacity,
		entries:     make([]*SystemLogEntry, 0, capacity),
		subscribers: make(map[int64]chan *SystemLogEntry),
		nodeID:      nodeID,
	}
}

// Append adds a new log entry to the buffer and broadcasts to live streaming subscribers.
func (rb *LogRingBuffer) Append(entry *SystemLogEntry) {
	if entry == nil {
		return
	}
	if entry.NodeID == "" {
		entry.NodeID = rb.nodeID
	}

	rb.mu.Lock()
	rb.nextID++
	entry.ID = rb.nextID

	if len(rb.entries) >= rb.capacity {
		// Shift ring buffer
		rb.entries = append(rb.entries[1:], entry)
	} else {
		rb.entries = append(rb.entries, entry)
	}

	// Broadcast while still holding the write lock.
	//
	// Subscribe's cancel func closes the subscriber channel under this same
	// lock. Copying the subscriber list, unlocking and only then sending left a
	// window in which a cancel could close a channel that this loop had already
	// captured, panicking the logging goroutine with "send on closed channel"
	// (and taking the whole process down when the call site was a background
	// worker). Sends are non-blocking, so holding the lock costs nothing.
	for _, ch := range rb.subscribers {
		select {
		case ch <- entry:
		default:
			// Subscriber buffer full; drop to avoid blocking logger
		}
	}
	rb.mu.Unlock()
}

// GetRecentLogs returns matching entries from newest to oldest.
func (rb *LogRingBuffer) GetRecentLogs(limit int, levelFilter, queryFilter, categoryFilter, traceIDFilter string) []*SystemLogEntry {
	rb.mu.RLock()
	defer rb.mu.RUnlock()

	if limit <= 0 || limit > 1000 {
		limit = 100
	}

	matched := make([]*SystemLogEntry, 0, limit)
	levelFilter = strings.ToUpper(strings.TrimSpace(levelFilter))
	queryFilter = strings.ToLower(strings.TrimSpace(queryFilter))
	categoryFilter = strings.ToLower(strings.TrimSpace(categoryFilter))
	traceIDFilter = strings.TrimSpace(traceIDFilter)

	// Scan backwards from newest to oldest
	for i := len(rb.entries) - 1; i >= 0 && len(matched) < limit; i-- {
		entry := rb.entries[i]

		// Level check
		if levelFilter != "" && levelFilter != "ALL" {
			if entry.Level != levelFilter {
				continue
			}
		}

		// Category check
		if categoryFilter != "" && categoryFilter != "all" {
			if entry.Category != categoryFilter {
				continue
			}
		}

		// TraceID check
		if traceIDFilter != "" {
			if entry.TraceID != traceIDFilter {
				continue
			}
		}

		// Query text check
		if queryFilter != "" {
			msgLower := strings.ToLower(entry.Message)
			hasMatch := strings.Contains(msgLower, queryFilter)
			if !hasMatch {
				// Search in attrs
				for k, v := range entry.Attrs {
					if strings.Contains(strings.ToLower(k), queryFilter) ||
						strings.Contains(strings.ToLower(fmt.Sprintf("%v", v)), queryFilter) {
						hasMatch = true
						break
					}
				}
			}
			if !hasMatch {
				continue
			}
		}

		matched = append(matched, entry)
	}

	return matched
}

// Subscribe opens a stream channel for live SSE push. Returns channel and cancel func.
func (rb *LogRingBuffer) Subscribe() (<-chan *SystemLogEntry, func()) {
	rb.mu.Lock()
	rb.nextSubID++
	subID := rb.nextSubID
	ch := make(chan *SystemLogEntry, 200)
	rb.subscribers[subID] = ch
	rb.mu.Unlock()

	cancel := func() {
		rb.mu.Lock()
		defer rb.mu.Unlock()
		if existing, ok := rb.subscribers[subID]; ok {
			delete(rb.subscribers, subID)
			close(existing)
		}
	}

	return ch, cancel
}

// BroadcastHandler wraps an underlying slog.Handler, intercepting records to feed the RingBuffer.
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
