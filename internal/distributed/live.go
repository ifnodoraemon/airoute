package distributed

import (
	"context"
	"encoding/json"
	"math"
	"time"

	"github.com/ifnodoraemon/airoute/internal/telemetry"
)

const (
	// LiveSnapshotKeyPrefix namespaces the per-node live telemetry snapshots.
	LiveSnapshotKeyPrefix = "airoute:live:"
	// liveSnapshotTTL bounds how stale a peer snapshot may be. Publishing once
	// per second with a 4s TTL tolerates one or two lost publishes without
	// dropping a healthy node from the cluster view.
	liveSnapshotTTL = 4 * time.Second
	// liveAggregateTimeout bounds the optional Redis read on the request path so
	// a slow cluster never stalls the dashboard.
	liveAggregateTimeout = 250 * time.Millisecond
	// livePublishInterval is the snapshot publish cadence.
	livePublishInterval = time.Second
)

// StartLiveStatsPublisher publishes this node's in-memory live telemetry to
// Redis once per second so every replica can serve a cluster-wide aggregate of
// the dashboard's real-time throughput. It is a no-op when Redis is inactive:
// the gateway then simply serves node-local values.
func StartLiveStatsPublisher(client *Client, stop <-chan struct{}) {
	if client == nil || !client.IsActive() {
		return
	}
	rdb := client.GetRDB()
	if rdb == nil {
		return
	}
	key := LiveSnapshotKeyPrefix + telemetry.LiveNodeID()

	go func() {
		ticker := time.NewTicker(livePublishInterval)
		defer ticker.Stop()

		var lastErr string
		publish := func() {
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			payload, err := json.Marshal(telemetry.GlobalLiveMetrics.Snapshot())
			if err != nil {
				return
			}
			err = rdb.Set(ctx, key, payload, liveSnapshotTTL).Err()
			// Log a failure once rather than every second while Redis is down.
			if err != nil && err.Error() != lastErr {
				telemetry.Logger.Warn("failed to publish live telemetry snapshot to redis", "error", err.Error())
				lastErr = err.Error()
			}
			if err == nil {
				lastErr = ""
			}
		}

		// Publish immediately so a freshly started node is visible to peers
		// within the first second instead of the first tick.
		publish()
		for {
			select {
			case <-stop:
				// Remove the key on shutdown so peers stop counting this node
				// immediately instead of waiting for liveSnapshotTTL to expire.
				ctx, cancel := context.WithTimeout(context.Background(), time.Second)
				_ = rdb.Del(ctx, key).Err()
				cancel()
				return
			case <-ticker.C:
				publish()
			}
		}
	}()
}

// AggregateLiveSnapshot merges the local snapshot with every peer snapshot
// currently published under airoute:live:*. The Redis read is bounded by
// liveAggregateTimeout and the function silently falls back to the node-local
// snapshot when Redis is unavailable, slow, or empty, so the dashboard endpoint
// never depends on the cluster being healthy.
func AggregateLiveSnapshot(ctx context.Context, local telemetry.LiveSnapshot) telemetry.LiveSnapshot {
	client := GetClient()
	if client == nil || !client.IsActive() {
		return local
	}
	rdb := client.GetRDB()
	if rdb == nil {
		return local
	}

	queryCtx, cancel := context.WithTimeout(ctx, liveAggregateTimeout)
	defer cancel()

	// KEYS is deliberate here: the namespace under this prefix is bounded by the
	// number of gateway replicas, never by request traffic.
	keys, err := rdb.Keys(queryCtx, LiveSnapshotKeyPrefix+"*").Result()
	if err != nil || len(keys) == 0 {
		return local
	}

	localKey := LiveSnapshotKeyPrefix + telemetry.LiveNodeID()
	peerKeys := make([]string, 0, len(keys))
	for _, key := range keys {
		if key != localKey {
			peerKeys = append(peerKeys, key)
		}
	}

	peers := make([]telemetry.LiveSnapshot, 0, len(peerKeys))
	if len(peerKeys) > 0 {
		values, err := rdb.MGet(queryCtx, peerKeys...).Result()
		if err != nil {
			return local
		}
		for _, value := range values {
			raw, ok := value.(string)
			if !ok || raw == "" {
				// The key expired between KEYS and MGET; skip it.
				continue
			}
			var snapshot telemetry.LiveSnapshot
			if err := json.Unmarshal([]byte(raw), &snapshot); err != nil {
				// Ignore a malformed/foreign payload rather than failing the endpoint.
				continue
			}
			peers = append(peers, snapshot)
		}
	}

	return mergeLiveSnapshots(time.Now(), local, peers)
}

// mergeLiveSnapshots folds peer snapshots into the local one: gauges and token
// rates are summed, the most recent token stamp wins, and idle time is derived
// from that stamp so a busy peer immediately resets the cluster idle time.
func mergeLiveSnapshots(now time.Time, local telemetry.LiveSnapshot, peers []telemetry.LiveSnapshot) telemetry.LiveSnapshot {
	merged := local
	merged.Nodes = 1

	latestTokenAt, _ := time.Parse(time.RFC3339, local.LastTokenAt) // zero when ""

	for _, peer := range peers {
		merged.ActiveRequests += peer.ActiveRequests
		merged.ActiveStreams += peer.ActiveStreams
		merged.TokensPerSec1s += peer.TokensPerSec1s
		merged.TokensPerSec10s += peer.TokensPerSec10s
		merged.TokensPerSec60s += peer.TokensPerSec60s
		merged.TokensLast60s += peer.TokensLast60s
		merged.Nodes++

		if peer.LastTokenAt == "" {
			continue
		}
		peerTokenAt, err := time.Parse(time.RFC3339, peer.LastTokenAt)
		if err != nil {
			continue
		}
		if peerTokenAt.After(latestTokenAt) {
			latestTokenAt = peerTokenAt
		}
	}

	// Re-round after summing so the aggregate reads like a single-node snapshot.
	merged.TokensPerSec1s = roundLiveRate(merged.TokensPerSec1s)
	merged.TokensPerSec10s = roundLiveRate(merged.TokensPerSec10s)
	merged.TokensPerSec60s = roundLiveRate(merged.TokensPerSec60s)
	merged.ServerTime = now.UTC().Format(time.RFC3339)

	if latestTokenAt.IsZero() {
		merged.LastTokenAt = ""
		merged.IdleSeconds = -1
	} else {
		merged.LastTokenAt = latestTokenAt.UTC().Format(time.RFC3339)
		idle := int64(now.Sub(latestTokenAt) / time.Second)
		if idle < 0 {
			idle = 0
		}
		merged.IdleSeconds = idle
	}
	return merged
}

// roundLiveRate keeps the aggregated payload free of binary float noise.
func roundLiveRate(rate float64) float64 {
	return math.Round(rate*10) / 10
}
