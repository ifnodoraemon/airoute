//go:build integration

package distributed

import (
	"context"
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
)

// testRedisURL returns the DSN for the Redis integration suite.
//
// A dedicated variable is used instead of REDIS_URL so that running the suite
// can never be pointed at a live cluster by accident: these tests write keys and
// publish on the real invalidation channel.
func testRedisURL(t *testing.T) string {
	t.Helper()
	url := os.Getenv("AIROUTE_TEST_REDIS_URL")
	if url == "" {
		t.Skip("AIROUTE_TEST_REDIS_URL is not set; skipping Redis integration test")
	}
	return url
}

// TestRedisClusterReloadAndSessionAffinity exercises the real Redis cluster
// paths against a live server: the reload broadcast that keeps replicas'
// in-memory configuration in sync, the config-version counter used for
// divergence reconciliation, and the sticky-session pinning used for provider
// affinity.
//
// These paths were previously reachable only by hand-starting the docker
// compose cluster, so no automated run ever executed them.
func TestRedisClusterReloadAndSessionAffinity(t *testing.T) {
	url := testRedisURL(t)

	client := InitRedis(url)
	if client == nil {
		t.Fatalf("InitRedis(%q) returned nil; expected an active cluster client", url)
	}
	// InitRedis writes a package-level global, so restore local-memory mode for
	// anything that runs after this test.
	defer InitRedis("")

	if !client.IsActive() {
		t.Fatal("client reports inactive against a reachable Redis server")
	}

	ctx := context.Background()

	// --- Sticky session affinity round trip -------------------------------
	if err := client.SetSessionChannel(ctx, "it-model", "it-session-1", "channel-a", 30*time.Minute); err != nil {
		t.Fatalf("SetSessionChannel: %v", err)
	}
	got, err := client.GetSessionChannel(ctx, "it-model", "it-session-1")
	if err != nil {
		t.Fatalf("GetSessionChannel: %v", err)
	}
	if got != "channel-a" {
		t.Fatalf("session pin = %q, want %q", got, "channel-a")
	}

	// An unpinned session must resolve to empty rather than an error.
	missing, err := client.GetSessionChannel(ctx, "it-model", "it-session-absent")
	if err != nil {
		t.Fatalf("GetSessionChannel(absent): %v", err)
	}
	if missing != "" {
		t.Fatalf("unpinned session resolved to %q, want empty", missing)
	}

	// --- Cluster reload broadcast and config version ----------------------
	before, err := client.GetRDB().Get(ctx, ConfigVersionKey).Int64()
	if err != nil && err != redis.Nil {
		t.Fatalf("read config version: %v", err)
	}

	// The callback receives the *reason*, and that distinction is the whole
	// point: the subscriber also has a 15s self-healing ticker that fires a
	// reload when it notices the config version moved. Asserting only "a reload
	// happened" therefore passes even with the pub/sub broadcast completely
	// broken, which is exactly what an earlier version of this test did.
	reloads := make(chan string, 16)
	stop := make(chan struct{})
	defer close(stop)
	client.SubscribeReload(func(reason string) {
		select {
		case reloads <- reason:
		default:
		}
	}, stop)

	const publishReason = "integration-test"

	// SubscribeReload establishes the subscription asynchronously, so a single
	// publish races with SUBSCRIBE and can be missed; keep publishing until the
	// subscriber reacts. The 6s window is deliberately far below the 15s
	// reconciliation ticker, so a delivery can only have come from the broadcast.
	//
	// The publisher impersonates a *peer* node: a node's own broadcast is ignored by
	// its own subscriber (it has already reloaded before publishing), so publishing
	// through PublishReload here would never be delivered to this subscriber.
	deadline := time.After(6 * time.Second)
	ticker := time.NewTicker(250 * time.Millisecond)
	defer ticker.Stop()

	publishAsPeer := func() {
		// A real peer advances the shared version before publishing (PublishReload
		// does the same); the version is what heals subscribers that missed a
		// message, and the assertions below check that it moved.
		if err := client.GetRDB().Incr(ctx, ConfigVersionKey).Err(); err != nil {
			t.Fatalf("advance config version: %v", err)
		}
		payload, err := json.Marshal(ClusterEvent{
			Event:     "reload",
			Reason:    publishReason,
			NodeID:    "integration-test-peer-node",
			Timestamp: time.Now().UnixMicro(),
		})
		if err != nil {
			t.Fatalf("marshal cluster event: %v", err)
		}
		if err := client.GetRDB().Publish(ctx, ReloadChannel, string(payload)).Err(); err != nil {
			t.Fatalf("publish peer reload: %v", err)
		}
	}

	var gotReason string
	for gotReason == "" {
		publishAsPeer()
		select {
		case gotReason = <-reloads:
		case <-ticker.C:
		case <-deadline:
			t.Fatal("cluster reload broadcast was not delivered within 6s; only the 15s reconciliation ticker could have delivered it later")
		}
	}
	if gotReason != publishReason {
		t.Fatalf("reload reason = %q, want %q: a reconcile-triggered reload means the pub/sub broadcast was not delivered", gotReason, publishReason)
	}

	after, err := client.GetRDB().Get(ctx, ConfigVersionKey).Int64()
	if err != nil {
		t.Fatalf("read config version after publish: %v", err)
	}
	if after <= before {
		t.Fatalf("config version did not advance: before=%d after=%d", before, after)
	}

	// A node's own broadcast must be ignored by its own subscriber: it already
	// reloaded before publishing, and acting on the message again would double the
	// database load per write. The 15s reconciliation ticker cannot interfere with
	// this 700ms window.
	whileDraining := time.NewTimer(200 * time.Millisecond)
	<-whileDraining.C
	for {
		select {
		case <-reloads:
			continue
		default:
		}
		break
	}
	if ownErr := client.PublishReload(ctx, "own-node-broadcast"); ownErr != nil {
		t.Fatalf("PublishReload: %v", ownErr)
	}
	select {
	case reason := <-reloads:
		t.Fatalf("own broadcast %q was delivered back to the publishing node", reason)
	case <-time.After(700 * time.Millisecond):
		// Expected: ignored.
	}
}

// TestRedisUnreachableFallsBackToLocalMode pins the documented graceful
// degradation: an unreachable Redis must not fail startup, it must leave the
// gateway running in local in-memory mode.
func TestRedisUnreachableFallsBackToLocalMode(t *testing.T) {
	// Deliberately not gated on AIROUTE_TEST_REDIS_URL: this case needs no
	// server, and it is the behaviour every single-node deployment relies on.
	client := InitRedis("redis://127.0.0.1:1/0")
	defer InitRedis("")

	if client != nil {
		t.Fatal("InitRedis returned a client for an unreachable server; expected nil")
	}
	if GetClient() != nil {
		t.Fatal("global client was left set after an unreachable-server init")
	}
}
