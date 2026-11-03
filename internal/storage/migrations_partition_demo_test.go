package storage

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"
)

// TestPartitionDemoLargeTable is a manual probe (not part of the normal suite): it
// seeds a large plain usage_logs, runs the opt-in migration, and reports copy time,
// row/unit preservation, partition layout and pruning behaviour.
//
// Run it against a throwaway database to size the maintenance window for a real
// deployment:
//
//	docker run -d --rm --name pg-partcheck -e POSTGRES_PASSWORD=x -e POSTGRES_USER=airoute \
//	  -e POSTGRES_DB=airoute -p 55432:5432 postgres:16-alpine
//	AIROUTE_PARTITION_DEMO_DSN='postgres://airoute:x@127.0.0.1:55432/airoute?sslmode=disable' \
//	  go test ./internal/storage/ -run TestPartitionDemoLargeTable -v -timeout 15m
func TestPartitionDemoLargeTable(t *testing.T) {
	dsn := os.Getenv("AIROUTE_PARTITION_DEMO_DSN")
	if dsn == "" {
		t.Skip("AIROUTE_PARTITION_DEMO_DSN is not set")
	}
	const totalRows = 1_000_000

	t.Setenv(enableTablePartitioningEnv, "false")
	plain, err := OpenDB(dsn)
	if err != nil {
		t.Fatalf("open plain demo db: %v", err)
	}
	seedStart := time.Now()
	if _, err := plain.Exec(`INSERT INTO usage_logs
		(trace_id, api_key, tenant_id, model, channel, prompt_tokens, completion_tokens, total_tokens,
		 cost, cost_units, status_code, created_at, billing_id)
		SELECT 'tr-' || g, 'sk-demo-key', 'tenant-demo', 'demo-model', 'demo-channel', 500, 500, 1000,
		       0.001, 1000000, 200, now() - ((g % 400) || ' days')::interval, 'demo-' || g
		  FROM generate_series(1, ` + fmt.Sprint(totalRows) + `) g`); err != nil {
		t.Fatalf("seed rows: %v", err)
	}
	var rows, units int64
	if err := plain.QueryRow(`SELECT COUNT(*), COALESCE(SUM(cost_units), 0) FROM usage_logs`).Scan(&rows, &units); err != nil {
		t.Fatalf("stats after seed: %v", err)
	}
	t.Logf("seeded rows=%d units=%d in %s", rows, units, time.Since(seedStart).Round(time.Millisecond))
	var dbSize string
	_ = plain.QueryRow(`SELECT pg_size_pretty(pg_total_relation_size('usage_logs'))`).Scan(&dbSize)
	t.Logf("plain usage_logs total size: %s", dbSize)
	_ = plain.Close()

	t.Setenv(enableTablePartitioningEnv, "true")
	migrateStart := time.Now()
	db, err := OpenDB(dsn)
	if err != nil {
		t.Fatalf("open partitioned demo db: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	t.Logf("startup with migration 43 took %s", time.Since(migrateStart).Round(time.Millisecond))

	relkind, err := usageLogsRelkind(db)
	if err != nil {
		t.Fatalf("relkind: %v", err)
	}
	var afterRows, afterUnits int64
	if err := db.QueryRow(`SELECT COUNT(*), COALESCE(SUM(cost_units), 0) FROM usage_logs`).Scan(&afterRows, &afterUnits); err != nil {
		t.Fatalf("stats after migration: %v", err)
	}
	repo := NewRepository(db)
	partitions, err := repo.usageLogPartitions()
	if err != nil {
		t.Fatalf("partitions: %v", err)
	}
	monthly, hasDefault := 0, false
	for _, p := range partitions {
		if p.isDefault {
			hasDefault = true
			continue
		}
		monthly++
	}
	var newSize, legacyName string
	_ = db.QueryRow(`SELECT pg_size_pretty(pg_total_relation_size('usage_logs'))`).Scan(&newSize)
	_ = db.QueryRow(`SELECT c.relname FROM pg_class c JOIN pg_namespace n ON n.oid = c.relnamespace
		WHERE n.nspname = current_schema() AND c.relkind = 'r' AND c.relname LIKE 'usage_logs_legacy_%'
		ORDER BY c.relname LIMIT 1`).Scan(&legacyName)
	var legacySize string
	if legacyName != "" {
		_ = db.QueryRow(`SELECT pg_size_pretty(pg_total_relation_size($1))`, legacyName).Scan(&legacySize)
	}
	t.Logf("after migration: relkind=%q rows=%d units=%d (want %d/%d) monthly_partitions=%d default=%v",
		relkind, afterRows, afterUnits, rows, units, monthly, hasDefault)
	t.Logf("sizes: partitioned=%s legacy=%s (%s)", newSize, legacyName, legacySize)

	// Pruning: a single-month window must touch one partition.
	from, to := usageLogMonthStart(time.Now()), usageLogMonthStart(time.Now()).AddDate(0, 1, 0)
	planRows, err := db.Query(`EXPLAIN (COSTS OFF) SELECT COUNT(*) FROM usage_logs WHERE created_at >= $1 AND created_at < $2`, from, to)
	if err != nil {
		t.Fatalf("explain: %v", err)
	}
	t.Log("single-month plan:")
	for planRows.Next() {
		var line string
		if err := planRows.Scan(&line); err != nil {
			t.Fatal(err)
		}
		t.Log("  " + line)
	}
	planRows.Close()
	t.Logf("partitions scanned for one month: %d", scannedPartitionCount(t, db, from, to))

	// A year-wide range must still prune to the months in that range.
	yearStart := usageLogMonthStart(time.Now().AddDate(0, 0, -400))
	yearEnd := usageLogMonthStart(time.Now()).AddDate(0, 1, 0)
	t.Logf("partitions scanned for a 400-day range: %d of %d monthly",
		scannedPartitionCount(t, db, yearStart, yearEnd), monthly)

	// The old ON CONFLICT form must be gone from the partitioned table (proving the
	// writer path needs the registry).
	var conflictErr error
	_, conflictErr = db.Exec(`INSERT INTO usage_logs (billing_id) VALUES ('conflict-probe') ON CONFLICT (billing_id) DO NOTHING`)
	t.Logf("ON CONFLICT (billing_id) on partitioned table: %v", conflictErr)

	// Dedupe via the registry: two deliveries, one row.
	record := &UsageLogRecord{TraceID: "tr-demo-dedupe", APIKey: "sk-demo-key", TenantID: "tenant-demo",
		Model: "demo-model", Cost: 0.001, TotalTokens: 1000, BillingID: "demo-dedupe", CreatedAt: time.Now()}
	dedupeStart := time.Now()
	for i := 0; i < 2; i++ {
		if err := repo.insertUsageLogsContext(context.Background(), []*UsageLogRecord{record}); err != nil {
			t.Fatalf("dedupe delivery %d: %v", i+1, err)
		}
	}
	var dedupeRows int
	if err := db.QueryRow(`SELECT COUNT(*) FROM usage_logs WHERE billing_id = 'demo-dedupe'`).Scan(&dedupeRows); err != nil {
		t.Fatalf("count dedupe rows: %v", err)
	}
	t.Logf("registry dedupe: %d row(s) after two deliveries in %s", dedupeRows, time.Since(dedupeStart).Round(time.Millisecond))
}
