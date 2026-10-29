package storage

// Regression tests for JOBS-01 / JOBS-02 (2026-10-29 bughunt round 3), gated on a
// throwaway Redis because they exercise the real XREADGROUP/XADD/DLQ paths:
//
//	docker run -d --rm -p 16379:6379 redis:7-alpine
//	AIROUTE_TEST_REDIS_URL=redis://127.0.0.1:16379/0 \
//	  go test ./internal/storage/ -run 'TestRedis' -v
//
// Without AIROUTE_TEST_REDIS_URL they skip, which keeps `go test ./...` green on a
// machine with no Redis.
//
// JOBS-01: a consumer group that disappears must be recreated by the running
// consumer (the old code created it once at start and swallowed the error), so
// ingestion resumes without a restart.
// JOBS-02: an undecodable entry is dead-lettered and counted, never silently
// acknowledged and forgotten.

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/ifnodoraemon/airoute/internal/distributed"
	"github.com/ifnodoraemon/airoute/internal/telemetry"
	"github.com/redis/go-redis/v9"
)

func redisBugprobeClient(t *testing.T) *distributed.Client {
	t.Helper()
	url := strings.TrimSpace(os.Getenv("AIROUTE_TEST_REDIS_URL"))
	if url == "" {
		t.Skip("set AIROUTE_TEST_REDIS_URL to a disposable redis to run the real-Redis integration tests")
	}
	client := distributed.InitRedis(url)
	if client == nil || !client.IsActive() {
		t.Fatalf("redis at %s is not usable", url)
	}
	cleanupRedisStreams(t, client)
	t.Cleanup(func() {
		cleanupRedisStreams(t, client)
		distributed.InitRedis("")
	})
	return client
}

func cleanupRedisStreams(t *testing.T, client *distributed.Client) {
	t.Helper()
	ctx := context.Background()
	for _, key := range []string{RedisLogStreamKey, RedisLogDeadLetterStream} {
		_ = client.GetRDB().Del(ctx, key).Err()
	}
}

func redisTestRepo(t *testing.T) *Repository {
	t.Helper()
	db, err := OpenDB(":memory:")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return NewRepository(db)
}

func waitGroupPresent(t *testing.T, client *distributed.Client, timeout time.Duration) {
	t.Helper()
	ctx := context.Background()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if _, err := client.GetRDB().XInfoGroups(ctx, RedisLogStreamKey).Result(); err == nil {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("consumer group %q was not created", RedisLogGroup)
}

// waitUsageLogBillingID waits for want rows with billingID and returns the last
// observed count.
func waitUsageLogBillingID(t *testing.T, repo *Repository, billingID string, want int, timeout time.Duration) int {
	t.Helper()
	deadline := time.Now().Add(timeout)
	got := -1
	for time.Now().Before(deadline) {
		var n int
		if err := repo.db.QueryRow(`SELECT COUNT(*) FROM usage_logs WHERE billing_id = ?`, billingID).Scan(&n); err == nil {
			got = n
			if n == want {
				return n
			}
		}
		time.Sleep(25 * time.Millisecond)
	}
	return got
}

// TestRedisConsumerRecoversFromMissingGroup is the flipped JOBS-01 probe: after the
// group is deleted, the running consumer recreates it and persists new records.
func TestRedisConsumerRecoversFromMissingGroup(t *testing.T) {
	client := redisBugprobeClient(t)
	repo := redisTestRepo(t)

	prevLogger := GlobalAsyncLogger
	defer func() { GlobalAsyncLogger = prevLogger }()

	al := InitAsyncLogger(repo, 64, 8, 40*time.Millisecond)
	defer al.Stop()
	waitGroupPresent(t, client, 3*time.Second)

	// Model a Redis that came back without the group (restart from a snapshot that
	// predates it, a failover to a replica that lacks it, or an operator deleting
	// it). Before the fix the consumer retried a failing read forever, silently.
	ctx := context.Background()
	if err := client.GetRDB().XGroupDestroy(ctx, RedisLogStreamKey, RedisLogGroup).Err(); err != nil {
		t.Fatalf("destroy group: %v", err)
	}
	recreatedBefore := telemetry.GlobalMetrics.UsageLogGroupRecreated.Load()

	al.Record(&UsageLogRecord{TraceID: "tr-nogroup", BillingID: "bill-nogroup", StatusCode: 500})

	if got := waitUsageLogBillingID(t, repo, "bill-nogroup", 1, 6*time.Second); got != 1 {
		groups, _ := client.GetRDB().XInfoGroups(ctx, RedisLogStreamKey).Result()
		t.Fatalf("ingestion did not resume after the group went missing: rows=%d groups=%d", got, len(groups))
	}
	if recreated := telemetry.GlobalMetrics.UsageLogGroupRecreated.Load() - recreatedBefore; recreated == 0 {
		t.Fatal("the consumer must record that it recreated the missing consumer group")
	}
	// The recreated group must be usable: pending may hold nothing once the record
	// is persisted and ACKed.
	if pending, err := client.GetRDB().XPending(ctx, RedisLogStreamKey, RedisLogGroup).Result(); err == nil && pending.Count != 0 {
		t.Fatalf("expected the recovered record to be acknowledged, pending=%d", pending.Count)
	}
}

// TestRedisPoisonEntryIsDeadLettered is the flipped JOBS-02 probe: an undecodable
// entry is acknowledged (cannot wedge the PEL), counted, and copied to the
// bounded dead-letter stream — not silently dropped.
func TestRedisPoisonEntryIsDeadLettered(t *testing.T) {
	client := redisBugprobeClient(t)
	repo := redisTestRepo(t)

	prevLogger := GlobalAsyncLogger
	defer func() { GlobalAsyncLogger = prevLogger }()

	al := InitAsyncLogger(repo, 64, 8, 40*time.Millisecond)
	defer al.Stop()
	waitGroupPresent(t, client, 3*time.Second)

	ctx := context.Background()
	poisonedBefore := telemetry.GlobalMetrics.UsageLogStreamPoisoned.Load()

	poisonJSON, err := client.GetRDB().XAdd(ctx, &redis.XAddArgs{
		Stream: RedisLogStreamKey, Values: map[string]any{"data": "{not-json"},
	}).Result()
	if err != nil {
		t.Fatalf("xadd poison json: %v", err)
	}
	poisonMissing, err := client.GetRDB().XAdd(ctx, &redis.XAddArgs{
		Stream: RedisLogStreamKey, Values: map[string]any{"wrong": "field"},
	}).Result()
	if err != nil {
		t.Fatalf("xadd poison missing data: %v", err)
	}
	healthyPayload, _ := json.Marshal(&UsageLogRecord{TraceID: "tr-poison-healthy", BillingID: "bill-poison-healthy", StatusCode: 500})
	if _, err := client.GetRDB().XAdd(ctx, &redis.XAddArgs{
		Stream: RedisLogStreamKey, Values: map[string]any{"data": string(healthyPayload)},
	}).Result(); err != nil {
		t.Fatalf("xadd healthy: %v", err)
	}

	if got := waitUsageLogBillingID(t, repo, "bill-poison-healthy", 1, 4*time.Second); got != 1 {
		t.Fatalf("healthy record in the same batch must be persisted, got rows=%d", got)
	}

	// Wait for the poison entries to be acknowledged and dead-lettered.
	deadline := time.Now().Add(4 * time.Second)
	var dlqEntries []redis.XMessage
	for time.Now().Before(deadline) {
		dlqEntries, _ = client.GetRDB().XRange(ctx, RedisLogDeadLetterStream, "-", "+").Result()
		if len(dlqEntries) >= 2 {
			break
		}
		time.Sleep(25 * time.Millisecond)
	}
	if len(dlqEntries) != 2 {
		t.Fatalf("expected both poison entries in %q, got %d", RedisLogDeadLetterStream, len(dlqEntries))
	}
	seen := map[string]bool{}
	for _, entry := range dlqEntries {
		id, _ := entry.Values["original_id"].(string)
		reason, _ := entry.Values["reason"].(string)
		seen[id] = true
		if reason == "" {
			t.Fatalf("dead-letter entry %s has no reason", entry.ID)
		}
	}
	if !seen[poisonJSON] || !seen[poisonMissing] {
		t.Fatalf("dead-letter stream does not reference both poison entries: %v (want %s, %s)", seen, poisonJSON, poisonMissing)
	}

	if poisoned := telemetry.GlobalMetrics.UsageLogStreamPoisoned.Load() - poisonedBefore; poisoned != 2 {
		t.Fatalf("UsageLogStreamPoisoned must count both poison entries, got %d", poisoned)
	}

	pending, err := client.GetRDB().XPending(ctx, RedisLogStreamKey, RedisLogGroup).Result()
	if err != nil {
		t.Fatalf("xpending: %v", err)
	}
	if pending.Count != 0 {
		t.Fatalf("poison entries must be ACKed after dead-lettering, pending=%d", pending.Count)
	}
}

// TestRedisDeadLetterStreamIsBounded proves the DLQ cannot grow without limit: the
// MAXLEN trim keeps only the newest entries.
func TestRedisDeadLetterStreamIsBounded(t *testing.T) {
	client := redisBugprobeClient(t)
	ctx := context.Background()
	limit := int64(5)
	for i := 0; i < 12; i++ {
		if err := client.DeadLetterLog(ctx, RedisLogDeadLetterStream, "1-0", "test", "payload", limit); err != nil {
			t.Fatalf("append dead letter %d: %v", i, err)
		}
	}
	length, err := client.GetRDB().XLen(ctx, RedisLogDeadLetterStream).Result()
	if err != nil {
		t.Fatalf("xlen: %v", err)
	}
	if length > limit {
		t.Fatalf("dead-letter stream is not bounded: length=%d limit=%d", length, limit)
	}
}
