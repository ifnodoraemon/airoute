package storage

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/ifnodoraemon/airoute/internal/telemetry"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
)

func TestRedisStoppedLoggerSpillsWithoutQueueing(t *testing.T) {
	client := redisBugprobeClient(t)
	dir := spillTestConfig(t)
	al := &AsyncLogger{spill: newUsageSpill(), stopCh: make(chan struct{})}
	al.Stop()
	var wg sync.WaitGroup
	for i := 0; i < 32; i++ {
		wg.Go(func() { al.Record(&UsageLogRecord{Model: "m", APIKey: "sk-test-credential"}) })
	}
	wg.Wait()
	length, err := client.GetRDB().XLen(context.Background(), RedisLogStreamKey).Result()
	require.NoError(t, err)
	require.Zero(t, length, "stopped logger must not write to Redis")
	files := spillFiles(t, dir)
	require.Len(t, files, 1)
	data, err := os.ReadFile(filepath.Join(dir, files[0]))
	require.NoError(t, err)
	scanner := bufio.NewScanner(bytes.NewReader(data))
	count := 0
	for scanner.Scan() {
		var rec UsageLogRecord
		require.NoError(t, json.Unmarshal(scanner.Bytes(), &rec))
		require.NotEmpty(t, rec.TraceID)
		require.NotEmpty(t, rec.BillingID)
		require.NotEqual(t, "sk-test-credential", rec.APIKey)
		count++
	}
	require.NoError(t, scanner.Err())
	require.Equal(t, 32, count)
}

func TestRedisTrimAcknowledgedWhilePending(t *testing.T) {
	client := redisBugprobeClient(t)
	ctx := context.Background()
	cases := []struct{ name, acked, pending string }{
		{"pending older", "10-0", "9-10"},
		{"pending newer", "9-10", "10-0"},
		{"sequence ordering", "10-10", "10-9"},
		{"sequence ordering reverse", "10-9", "10-10"},
		{"no pending", "10-10", ""},
		{"no acknowledged cursor", "", "10-10"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cleanupRedisStreams(t, client)
			require.NoError(t, client.EnsureConsumerGroup(ctx, RedisLogStreamKey, RedisLogGroup))
			ids := []string{"1-0", "9-9", "9-10", "10-0", "10-9", "10-10", "11-0"}
			for _, id := range ids {
				require.NoError(t, client.GetRDB().XAdd(ctx, &redis.XAddArgs{Stream: RedisLogStreamKey, ID: id, Values: map[string]any{"data": "{}"}}).Err())
			}
			_, err := client.ReadGroupLogs(ctx, RedisLogStreamKey, RedisLogGroup, "peer", 7, 0)
			require.NoError(t, err)
			for _, id := range ids {
				if id != tc.pending {
					require.NoError(t, client.AckLogs(ctx, RedisLogStreamKey, RedisLogGroup, id))
				}
			}
			c := &logStreamConsumer{client: client, ctx: ctx, ackedUpTo: tc.acked, trimCounter: 99}
			c.trimAcknowledged()
			rows, err := client.GetRDB().XRange(ctx, RedisLogStreamKey, "-", "+").Result()
			require.NoError(t, err)
			want := "9-10"
			if tc.name == "sequence ordering" || tc.name == "sequence ordering reverse" {
				want = "10-9"
			}
			if tc.pending == "" {
				want = "10-10"
			}
			if tc.acked == "" {
				want = "1-0"
			}
			require.Equal(t, want, rows[0].ID)
			if tc.pending != "" {
				pending, err := client.GetRDB().XRange(ctx, RedisLogStreamKey, tc.pending, tc.pending).Result()
				require.NoError(t, err)
				require.Len(t, pending, 1, "pending billing record must survive trimming")
			}
		})
	}
}

func TestRedisClaimOrphansWarnsOnFailure(t *testing.T) {
	client := redisBugprobeClient(t)
	ctx := context.Background()
	require.NoError(t, client.GetRDB().Set(ctx, RedisLogStreamKey, "wrong type", 0).Err())
	var logs bytes.Buffer
	previous := telemetry.Logger
	telemetry.Logger = slog.New(slog.NewJSONHandler(&logs, nil))
	t.Cleanup(func() { telemetry.Logger = previous })
	c := &logStreamConsumer{client: client, ctx: ctx, consumerID: "audit-peer", logger: &AsyncLogger{batchSize: 1}, claimStart: "0-0"}
	c.claimOrphans()
	for _, part := range []string{`"level":"WARN"`, `"consumer":"audit-peer"`, "WRONGTYPE"} {
		require.True(t, strings.Contains(logs.String(), part), fmt.Sprintf("missing %s in %s", part, logs.String()))
	}
}
