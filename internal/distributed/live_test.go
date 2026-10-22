package distributed

import (
	"context"
	"testing"
	"time"

	"github.com/ifnodoraemon/airoute/internal/telemetry"
)

// TestAggregateLiveSnapshotFallsBackToLocal pins the no-Redis degradation: the
// endpoint must serve node-local values when the cluster layer is inactive.
func TestAggregateLiveSnapshotFallsBackToLocal(t *testing.T) {
	if GetClient() != nil {
		t.Skip("redis client is initialized in this process; fallback not exercised")
	}
	local := telemetry.LiveSnapshot{
		ActiveRequests: 2,
		ActiveStreams:  1,
		TokensPerSec1s: 3.5,
		TokensLast60s:  21,
		LastTokenAt:    "2026-10-22T02:11:01Z",
		IdleSeconds:    2,
		Nodes:          1,
		ServerTime:     "2026-10-22T02:11:03Z",
	}
	got := AggregateLiveSnapshot(context.Background(), local)
	if got != local {
		t.Fatalf("expected local snapshot fallback, got %+v", got)
	}
}

// TestMergeLiveSnapshots covers the cluster merge rules: gauges and rates sum,
// the freshest token stamp wins, malformed peer stamps are ignored and the node
// count reflects every valid peer.
func TestMergeLiveSnapshots(t *testing.T) {
	now := time.Now().Truncate(time.Second)

	local := telemetry.LiveSnapshot{
		ActiveRequests:  2,
		ActiveStreams:   1,
		TokensPerSec1s:  10,
		TokensPerSec10s: 5.5,
		TokensPerSec60s: 2.2,
		TokensLast60s:   100,
		LastTokenAt:     now.Add(-30 * time.Second).UTC().Format(time.RFC3339),
		IdleSeconds:     30,
		Nodes:           1,
		ServerTime:      now.UTC().Format(time.RFC3339),
	}
	peers := []telemetry.LiveSnapshot{
		{
			ActiveRequests:  1,
			ActiveStreams:   2,
			TokensPerSec1s:  3.3,
			TokensPerSec10s: 1.1,
			TokensPerSec60s: 0.6,
			TokensLast60s:   40,
			LastTokenAt:     now.Add(-2 * time.Second).UTC().Format(time.RFC3339),
		},
		{}, // a peer that has not generated a token yet
		{
			ActiveRequests: 4,
			TokensLast60s:  7,
			LastTokenAt:    "not-a-timestamp", // malformed stamp must not break aggregation
		},
	}

	got := mergeLiveSnapshots(now, local, peers)

	if got.Nodes != 4 {
		t.Fatalf("expected 4 nodes, got %d", got.Nodes)
	}
	if got.ActiveRequests != 7 {
		t.Fatalf("expected active_requests 7, got %d", got.ActiveRequests)
	}
	if got.ActiveStreams != 3 {
		t.Fatalf("expected active_streams 3, got %d", got.ActiveStreams)
	}
	if got.TokensLast60s != 147 {
		t.Fatalf("expected tokens_last_60s 147, got %d", got.TokensLast60s)
	}
	if got.TokensPerSec1s != 13.3 {
		t.Fatalf("expected tokens_per_sec_1s 13.3, got %v", got.TokensPerSec1s)
	}
	if got.TokensPerSec10s != 6.6 {
		t.Fatalf("expected tokens_per_sec_10s 6.6, got %v", got.TokensPerSec10s)
	}
	if got.TokensPerSec60s != 2.8 {
		t.Fatalf("expected tokens_per_sec_60s 2.8, got %v", got.TokensPerSec60s)
	}
	if got.IdleSeconds != 2 {
		t.Fatalf("expected idle_seconds 2 from the freshest peer, got %d", got.IdleSeconds)
	}
	if got.LastTokenAt != now.Add(-2*time.Second).UTC().Format(time.RFC3339) {
		t.Fatalf("expected last_token_at from the freshest peer, got %q", got.LastTokenAt)
	}
	if got.ServerTime != now.UTC().Format(time.RFC3339) {
		t.Fatalf("expected server_time %q, got %q", now.UTC().Format(time.RFC3339), got.ServerTime)
	}
}

// TestMergeLiveSnapshotsWithoutTokens keeps the "never streamed" sentinels intact.
func TestMergeLiveSnapshotsWithoutTokens(t *testing.T) {
	now := time.Now()
	got := mergeLiveSnapshots(now, telemetry.LiveSnapshot{}, nil)
	if got.Nodes != 1 {
		t.Fatalf("expected 1 node, got %d", got.Nodes)
	}
	if got.LastTokenAt != "" || got.IdleSeconds != -1 {
		t.Fatalf("expected empty last_token_at and idle -1, got %q / %d", got.LastTokenAt, got.IdleSeconds)
	}
}
