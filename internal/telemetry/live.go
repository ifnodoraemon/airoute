package telemetry

import (
	"math"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

const (
	// liveBucketCount is the number of 1-second buckets retained by the live
	// token ring. 180s gives the 60s rate window three times the headroom, so a
	// slot is never overwritten while it is still inside an active window.
	liveBucketCount = 180

	// liveCharsPerToken mirrors model.CharsPerToken (internal/model/tokens.go).
	// telemetry deliberately does not import the business `model` package to
	// keep the observability layer dependency-free; keep the two values in sync
	// whenever the estimation heuristic changes.
	liveCharsPerToken = 4
)

// LiveSnapshot is the point-in-time in-flight telemetry payload behind
// GET /api/v1/admin/stats/live. The JSON shape is a frozen frontend contract:
// field names and semantics must not change.
type LiveSnapshot struct {
	ActiveRequests  int64   `json:"active_requests"`
	ActiveStreams   int64   `json:"active_streams"`
	TokensPerSec1s  float64 `json:"tokens_per_sec_1s"`
	TokensPerSec10s float64 `json:"tokens_per_sec_10s"`
	TokensPerSec60s float64 `json:"tokens_per_sec_60s"`
	TokensLast60s   int64   `json:"tokens_last_60s"`
	LastTokenAt     string  `json:"last_token_at"` // RFC3339 UTC; "" until the first token
	IdleSeconds     int64   `json:"idle_seconds"`  // -1 until the first token
	Nodes           int64   `json:"nodes"`
	ServerTime      string  `json:"server_time"`
}

// LiveMetrics tracks in-flight (in-memory) request/stream gauges plus a
// second-resolution ring of generated tokens. It answers the dashboard's
// real-time throughput question without reading usage_logs, which only contains
// already-finished requests and therefore stays at zero during generation.
//
// Every method is safe for concurrent use by arbitrary request goroutines.
type LiveMetrics struct {
	activeStreams atomic.Int64

	// mu guards the bucket ring, the character remainder and the last-token stamp.
	mu            sync.Mutex
	bucketTokens  [liveBucketCount]int64
	bucketSeconds [liveBucketCount]int64 // epoch second each slot currently belongs to
	charRemainder int                    // sub-token characters carried between deltas
	lastTokenAt   time.Time              // zero until the first token
}

// GlobalLiveMetrics is the process-wide in-flight telemetry collector.
var GlobalLiveMetrics = &LiveMetrics{}

// LiveNodeID returns this process' stable node identifier, used to scope the
// per-replica keys under airoute:live:<node>. It follows the same NODE_ID /
// HOSTNAME scheme as the system log ring buffer so operators can correlate the
// two views.
func LiveNodeID() string {
	if id := strings.TrimSpace(os.Getenv("NODE_ID")); id != "" {
		return id
	}
	if host := strings.TrimSpace(os.Getenv("HOSTNAME")); host != "" {
		return host
	}
	if host, err := os.Hostname(); err == nil {
		if trimmed := strings.TrimSpace(host); trimmed != "" {
			return trimmed
		}
	}
	return "airoute-gateway"
}

// BeginStream marks a streaming response as in flight.
func (l *LiveMetrics) BeginStream() {
	l.activeStreams.Add(1)
}

// EndStream marks a streaming response as finished and flushes any sub-token
// character remainder: once the stream is gone its remaining characters can no
// longer accumulate into a whole token, so an approximate token is emitted now.
func (l *LiveMetrics) EndStream() {
	// Clamp at zero so an unbalanced pair can never make the gauge negative.
	for {
		current := l.activeStreams.Load()
		if current <= 0 {
			break
		}
		if l.activeStreams.CompareAndSwap(current, current-1) {
			break
		}
	}

	l.mu.Lock()
	if l.charRemainder > 0 {
		l.addTokensLocked(time.Now(), 1)
		l.charRemainder = 0
	}
	l.mu.Unlock()
}

// ActiveStreams returns the current number of in-flight streams on this node.
func (l *LiveMetrics) ActiveStreams() int64 {
	return l.activeStreams.Load()
}

// AddTokens records n freshly generated tokens arriving now.
func (l *LiveMetrics) AddTokens(tokens int) {
	if tokens <= 0 {
		return
	}
	l.mu.Lock()
	l.addTokensLocked(time.Now(), tokens)
	l.mu.Unlock()
}

// AddChars converts streamed delta text into approximate tokens and records
// them; chars is the byte length of the delta text.
//
// The character remainder is carried across calls so tiny deltas (1-3 chars
// each) still add up to whole tokens instead of being rounded down to zero
// individually: four 1-char deltas produce exactly one token.
func (l *LiveMetrics) AddChars(chars int) {
	if chars <= 0 {
		return
	}
	l.mu.Lock()
	l.addCharsLocked(time.Now(), chars)
	l.mu.Unlock()
}

func (l *LiveMetrics) addCharsLocked(now time.Time, chars int) {
	l.charRemainder += chars
	tokens := l.charRemainder / liveCharsPerToken
	if tokens <= 0 {
		return
	}
	l.charRemainder -= tokens * liveCharsPerToken
	l.addTokensLocked(now, tokens)
}

func (l *LiveMetrics) addTokensLocked(now time.Time, tokens int) {
	sec := now.Unix()
	idx := int(sec % liveBucketCount)
	if idx < 0 {
		idx += liveBucketCount
	}
	// A slot is only valid for the second it was stamped with; when the ring
	// wraps onto an old second the stale count is discarded here instead of
	// requiring a background clearing pass.
	if l.bucketSeconds[idx] != sec {
		l.bucketSeconds[idx] = sec
		l.bucketTokens[idx] = 0
	}
	l.bucketTokens[idx] += int64(tokens)
	l.lastTokenAt = now
}

// Snapshot returns the current in-flight telemetry for this node. It performs
// no I/O besides an in-process lock, so the dashboard may poll it freely.
func (l *LiveMetrics) Snapshot() LiveSnapshot {
	return l.snapshotAt(time.Now())
}

func (l *LiveMetrics) snapshotAt(now time.Time) LiveSnapshot {
	nowUnix := now.Unix()

	l.mu.Lock()
	tokens1s := l.windowTokensLocked(nowUnix, 1)
	tokens10s := l.windowTokensLocked(nowUnix, 10)
	tokens60s := l.windowTokensLocked(nowUnix, 60)
	lastTokenAt := l.lastTokenAt
	l.mu.Unlock()

	snapshot := LiveSnapshot{
		// active_requests reuses the gauge every protocol handler already
		// maintains (chat/completions/responses/gemini/anthropic).
		ActiveRequests:  GlobalMetrics.ActiveConnections.Load(),
		ActiveStreams:   l.activeStreams.Load(),
		TokensPerSec1s:  roundLiveRate(float64(tokens1s)),
		TokensPerSec10s: roundLiveRate(float64(tokens10s) / 10),
		TokensPerSec60s: roundLiveRate(float64(tokens60s) / 60),
		TokensLast60s:   tokens60s,
		Nodes:           1,
		ServerTime:      now.UTC().Format(time.RFC3339),
	}
	if lastTokenAt.IsZero() {
		snapshot.LastTokenAt = ""
		snapshot.IdleSeconds = -1
	} else {
		snapshot.LastTokenAt = lastTokenAt.UTC().Format(time.RFC3339)
		idle := int64(now.Sub(lastTokenAt) / time.Second)
		if idle < 0 {
			idle = 0
		}
		snapshot.IdleSeconds = idle
	}
	return snapshot
}

// windowTokensLocked sums the token buckets belonging to the last `window`
// seconds, ending at nowUnix inclusive. Slots whose epoch second does not match
// are simply stale and contribute zero.
func (l *LiveMetrics) windowTokensLocked(nowUnix, window int64) int64 {
	var sum int64
	for i := int64(0); i < window; i++ {
		sec := nowUnix - i
		idx := int(sec % liveBucketCount)
		if idx < 0 {
			idx += liveBucketCount
		}
		if l.bucketSeconds[idx] == sec {
			sum += l.bucketTokens[idx]
		}
	}
	return sum
}

// roundLiveRate trims binary float noise so the JSON payload stays readable
// (e.g. 38.666666 -> 38.7).
func roundLiveRate(rate float64) float64 {
	return math.Round(rate*10) / 10
}
