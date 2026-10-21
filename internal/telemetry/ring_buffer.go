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

// GlobalRingBuffer holds the in-memory circular log buffer.
var GlobalRingBuffer *LogRingBuffer

func init() {
	nodeID := os.Getenv("NODE_ID")
	if nodeID == "" {
		nodeID = os.Getenv("HOSTNAME")
	}
	if nodeID == "" {
		nodeID = "airoute-gateway"
	}
	GlobalRingBuffer = NewLogRingBuffer(2000, nodeID)
}

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

	// Copy subscribers list to avoid holding lock while broadcasting
	subs := make([]chan *SystemLogEntry, 0, len(rb.subscribers))
	for _, ch := range rb.subscribers {
		subs = append(subs, ch)
	}
	rb.mu.Unlock()

	// Non-blocking broadcast
	for _, ch := range subs {
		select {
		case ch <- entry:
		default:
			// Subscriber buffer full; drop to avoid blocking logger
		}
	}
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
		attrs := make(map[string]any)
		var traceID string
		r.Attrs(func(a slog.Attr) bool {
			k := a.Key
			v := a.Value.Any()
			attrs[k] = v
			if strings.EqualFold(k, "trace_id") || strings.EqualFold(k, "traceId") {
				traceID = fmt.Sprintf("%v", v)
			}
			return true
		})

		msg := r.Message
		category := categorizeLog(msg, attrs)

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

func (h *BroadcastHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	var next slog.Handler
	if h.next != nil {
		next = h.next.WithAttrs(attrs)
	}
	return &BroadcastHandler{
		next:   next,
		buffer: h.buffer,
		level:  h.level,
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
	}
}

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
