package telemetry

import (
	"encoding/json"
	"sync"
	"testing"
	"time"
)

// TestLiveMetricsCharRemainder pins the "带余数累积" conversion: every four
// characters yield one approximate token, and the sub-token remainder is carried
// between deltas instead of being rounded away per call.
func TestLiveMetricsCharRemainder(t *testing.T) {
	m := &LiveMetrics{}

	// Three 1-char deltas are still short of one token.
	m.AddChars(1)
	m.AddChars(1)
	m.AddChars(1)
	if got := m.Snapshot().TokensLast60s; got != 0 {
		t.Fatalf("expected no token for 3 chars, got %d", got)
	}

	// The fourth character completes a token.
	m.AddChars(1)
	if got := m.Snapshot().TokensLast60s; got != 1 {
		t.Fatalf("expected 1 token for 4 chars, got %d", got)
	}

	// 9 chars = 2 tokens + 1 char remainder (already 1 token recorded above).
	m.AddChars(9)
	if got := m.Snapshot().TokensLast60s; got != 3 {
		t.Fatalf("expected 3 tokens after 13 chars total, got %d", got)
	}
}

// TestLiveMetricsEndStreamFlushesRemainder verifies that a stream ending on a
// sub-token boundary still records its final partial token.
func TestLiveMetricsEndStreamFlushesRemainder(t *testing.T) {
	m := &LiveMetrics{}
	m.BeginStream()
	m.AddChars(3)
	m.EndStream()

	if got := m.Snapshot().TokensLast60s; got != 1 {
		t.Fatalf("expected the partial token to be flushed at stream end, got %d", got)
	}
}

// TestLiveMetricsWindowBuckets verifies the 1s/10s/60s sliding windows and that
// tokens older than 60s are excluded.
func TestLiveMetricsWindowBuckets(t *testing.T) {
	m := &LiveMetrics{}
	base := time.Now().Truncate(time.Second)

	record := func(secondsAgo int64, tokens int) {
		m.mu.Lock()
		m.addTokensLocked(base.Add(-time.Duration(secondsAgo)*time.Second), tokens)
		m.mu.Unlock()
	}
	record(120, 5) // outside every window
	record(61, 4)  // outside the 60s window
	record(55, 20)
	record(5, 30)
	record(0, 10)

	snapshot := m.snapshotAt(base)

	if snapshot.TokensPerSec1s != 10 {
		t.Fatalf("expected 10 tokens in the 1s window, got %v", snapshot.TokensPerSec1s)
	}
	if snapshot.TokensPerSec10s != 4 {
		t.Fatalf("expected 40 tokens / 10s in the 10s window, got %v", snapshot.TokensPerSec10s)
	}
	if snapshot.TokensLast60s != 60 {
		t.Fatalf("expected 60 tokens in the 60s window, got %d", snapshot.TokensLast60s)
	}
	if snapshot.TokensPerSec60s != 1 {
		t.Fatalf("expected 60 tokens / 60s in the 60s window, got %v", snapshot.TokensPerSec60s)
	}
}

// TestLiveMetricsRingWrap checks that a ring slot reused by a newer second
// discards the stale count instead of leaking it into the new window.
func TestLiveMetricsRingWrap(t *testing.T) {
	m := &LiveMetrics{}
	base := time.Now().Truncate(time.Second)

	m.mu.Lock()
	m.addTokensLocked(base.Add(-liveBucketCount*time.Second), 9) // same slot as base
	m.mu.Unlock()
	m.AddTokens(2)

	snapshot := m.snapshotAt(base)
	if snapshot.TokensLast60s != 2 {
		t.Fatalf("expected stale wrapped bucket to be discarded, got %d", snapshot.TokensLast60s)
	}
}

// TestLiveMetricsIdleAndLastTokenAt covers the "never generated" sentinel (-1,
// empty timestamp) and idle derivation from the last token.
func TestLiveMetricsIdleAndLastTokenAt(t *testing.T) {
	m := &LiveMetrics{}
	now := time.Now().Truncate(time.Second)

	empty := m.snapshotAt(now)
	if empty.LastTokenAt != "" {
		t.Fatalf("expected empty last_token_at before any token, got %q", empty.LastTokenAt)
	}
	if empty.IdleSeconds != -1 {
		t.Fatalf("expected idle_seconds -1 before any token, got %d", empty.IdleSeconds)
	}

	lastTokenAt := now.Add(-42 * time.Second)
	m.mu.Lock()
	m.addTokensLocked(lastTokenAt, 3)
	m.mu.Unlock()

	snapshot := m.snapshotAt(now)
	if snapshot.IdleSeconds != 42 {
		t.Fatalf("expected idle_seconds 42, got %d", snapshot.IdleSeconds)
	}
	if snapshot.LastTokenAt != lastTokenAt.UTC().Format(time.RFC3339) {
		t.Fatalf("expected last_token_at %q, got %q", lastTokenAt.UTC().Format(time.RFC3339), snapshot.LastTokenAt)
	}
}

// TestLiveMetricsStreamGauge checks that BeginStream/EndStream stay balanced and
// never go negative.
func TestLiveMetricsStreamGauge(t *testing.T) {
	m := &LiveMetrics{}

	m.BeginStream()
	m.BeginStream()
	if got := m.ActiveStreams(); got != 2 {
		t.Fatalf("expected 2 active streams, got %d", got)
	}

	m.EndStream()
	if got := m.ActiveStreams(); got != 1 {
		t.Fatalf("expected 1 active stream after one end, got %d", got)
	}

	// An unbalanced end must clamp at zero rather than go negative.
	m.EndStream()
	m.EndStream()
	if got := m.ActiveStreams(); got != 0 {
		t.Fatalf("expected 0 active streams, got %d", got)
	}
}

// TestLiveMetricsSnapshotActiveRequests confirms active_requests reuses the
// existing in-flight connection gauge.
func TestLiveMetricsSnapshotActiveRequests(t *testing.T) {
	m := &LiveMetrics{}
	GlobalMetrics.IncActiveConns()
	defer GlobalMetrics.DecActiveConns()

	if snapshot := m.Snapshot(); snapshot.ActiveRequests < 1 {
		t.Fatalf("expected active_requests >= 1, got %d", snapshot.ActiveRequests)
	}
}

// TestLiveSnapshotJSONContract pins the frozen response field names.
func TestLiveSnapshotJSONContract(t *testing.T) {
	snapshot := GlobalLiveMetrics.Snapshot()
	raw, err := json.Marshal(snapshot)
	if err != nil {
		t.Fatalf("marshal snapshot: %v", err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("unmarshal snapshot: %v", err)
	}
	for _, field := range []string{
		"active_requests", "active_streams",
		"tokens_per_sec_1s", "tokens_per_sec_10s", "tokens_per_sec_60s",
		"tokens_last_60s", "last_token_at", "idle_seconds", "nodes", "server_time",
	} {
		if _, ok := decoded[field]; !ok {
			t.Fatalf("live snapshot JSON is missing contract field %q: %s", field, raw)
		}
	}
	if _, err := time.Parse(time.RFC3339, snapshot.ServerTime); err != nil {
		t.Fatalf("server_time is not RFC3339: %q", snapshot.ServerTime)
	}
}

// TestLiveMetricsConcurrentAccess is meaningful under -race: every streaming
// goroutine updates the same collector while the dashboard polls snapshots.
func TestLiveMetricsConcurrentAccess(t *testing.T) {
	m := &LiveMetrics{}
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 500; j++ {
				m.BeginStream()
				m.AddChars(7)
				m.AddTokens(2)
				_ = m.Snapshot()
				m.EndStream()
			}
		}()
	}
	wg.Wait()

	if got := m.ActiveStreams(); got != 0 {
		t.Fatalf("expected all streams to be closed, got %d", got)
	}
	if got := m.Snapshot().TokensLast60s; got <= 0 {
		t.Fatalf("expected positive token count after concurrent updates, got %d", got)
	}
}
