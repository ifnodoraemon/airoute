package telemetry

import (
	"fmt"
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
