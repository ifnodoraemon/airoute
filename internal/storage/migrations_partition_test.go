package storage

import (
	"context"
	"database/sql"
	"fmt"
	"net/url"
	"os"
	"regexp"
	"strings"
	"testing"
	"time"
)

// partitionScratchDSN creates a throwaway PostgreSQL database from
// AIROUTE_TEST_DATABASE_URL and returns its DSN.
//
// Migration 43 rewrites the live usage_logs table, so it must never run against a
// database other tests share. A dedicated database also lets the test seed a plain
// table, enable the flag, and observe the copy-switch exactly as an operator would.
// When the configured role cannot CREATE DATABASE the test skips instead of
// mutating the shared database.
func partitionScratchDSN(t *testing.T) string {
	t.Helper()
	base := strings.TrimSpace(os.Getenv("AIROUTE_TEST_DATABASE_URL"))
	if base == "" {
		t.Skip("AIROUTE_TEST_DATABASE_URL is not set; skipping PostgreSQL partition integration test")
	}
	parsed, err := url.Parse(base)
	if err != nil || (parsed.Scheme != "postgres" && parsed.Scheme != "postgresql") {
		t.Skipf("AIROUTE_TEST_DATABASE_URL is not a postgres:// URL DSN: %v", err)
	}

	name := fmt.Sprintf("airoute_partcheck_%d", time.Now().UnixNano())
	admin, err := sql.Open("postgres", base)
	if err != nil {
		t.Fatalf("open admin connection: %v", err)
	}
	if _, err := admin.Exec(`CREATE DATABASE ` + quotePGIdentifier(name)); err != nil {
		_ = admin.Close()
		t.Skipf("cannot create a scratch database (role needs CREATEDB): %v", err)
	}
	_ = admin.Close()

	scratch := *parsed
	scratch.Path = "/" + name
	t.Cleanup(func() {
		admin, err := sql.Open("postgres", base)
		if err != nil {
			return
		}
		defer func() { _ = admin.Close() }()
		_, _ = admin.Exec(`DROP DATABASE IF EXISTS ` + quotePGIdentifier(name) + ` WITH (FORCE)`)
	})
	return scratch.String()
}

// seedPartitionTestUsageLog inserts one audit row at an explicit age.
func seedPartitionTestUsageLog(t *testing.T, db *DB, billingID string, createdAt time.Time, costUnits int64) {
	t.Helper()
	_, err := db.Exec(`INSERT INTO usage_logs
		(trace_id, model, cost, cost_units, total_tokens, created_at, billing_id)
		VALUES (?, ?, ?, ?, ?, ?, ?)`,
		"tr-"+billingID, "partition-test-model", float64(costUnits)/1e9, costUnits, 1000,
		createdAt.UTC(), billingID)
	if err != nil {
		t.Fatalf("seed usage log %s: %v", billingID, err)
	}
}

// usageLogTestStats returns the audit row count and the charged-unit sum.
func usageLogTestStats(t *testing.T, db *DB) (rows, costUnits int64) {
	t.Helper()
	if err := db.QueryRow(`SELECT COUNT(*), COALESCE(SUM(cost_units), 0) FROM usage_logs`).Scan(&rows, &costUnits); err != nil {
		t.Fatalf("usage_logs stats: %v", err)
	}
	return rows, costUnits
}

// usageLogTableIndexes returns the index names attached to the live usage_logs
// table (the partitioned parent, when partitioning is enabled).
func usageLogTableIndexes(t *testing.T, db *DB) map[string]struct{} {
	t.Helper()
	rows, err := db.Query(`SELECT indexname FROM pg_indexes
		WHERE schemaname = current_schema() AND tablename = 'usage_logs'`)
	if err != nil {
		t.Fatalf("list usage_logs indexes: %v", err)
	}
	defer rows.Close()
	indexes := make(map[string]struct{})
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			t.Fatalf("scan index name: %v", err)
		}
		indexes[name] = struct{}{}
	}
	return indexes
}

// scannedPartitionCount runs EXPLAIN (no execution) and counts how many
// usage_logs_YYYY_MM partitions the plan touches for a created_at range.
func scannedPartitionCount(t *testing.T, db *DB, from, to time.Time) int {
	t.Helper()
	rows, err := db.Query(`EXPLAIN (FORMAT JSON) SELECT COUNT(*) FROM usage_logs
		WHERE created_at >= $1 AND created_at < $2`, from.UTC(), to.UTC())
	if err != nil {
		t.Fatalf("explain partitioned query: %v", err)
	}
	defer rows.Close()
	var plan strings.Builder
	for rows.Next() {
		var line string
		if err := rows.Scan(&line); err != nil {
			t.Fatalf("scan explain output: %v", err)
		}
		plan.WriteString(line)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("explain rows: %v", err)
	}
	relation := regexp.MustCompile(`"Relation Name": "usage_logs_\d{4}_\d{2}"`)
	return len(relation.FindAllString(plan.String(), -1))
}

// TestUsageLogsPartitionMigration drives the whole opt-in migration against a
// scratch PostgreSQL database: seed a plain table, enable the flag, and verify the
// copy-switch preserved the data, replaced the dedupe gate, re-provisioned the
// indexes, kept the id sequence moving, and is idempotent on re-run.
func TestUsageLogsPartitionMigration(t *testing.T) {
	dsn := partitionScratchDSN(t)
	ctx := context.Background()
	now := time.Now().UTC()

	// Phase 1: the default schema (v1..v42), with the partitioning flag off.
	t.Setenv(enableTablePartitioningEnv, "false")
	plain, err := OpenDB(dsn)
	if err != nil {
		t.Fatalf("open unpartitioned scratch database: %v", err)
	}
	if partitioned := NewRepository(plain).usageLogsPartitioned(); partitioned {
		t.Fatal("usage_logs is partitioned before migration 43 ran")
	}
	var v43 int
	if err := plain.QueryRow(`SELECT COUNT(*) FROM schema_migrations WHERE version = 43`).Scan(&v43); err != nil {
		t.Fatalf("read schema_migrations: %v", err)
	}
	if v43 != 0 {
		t.Fatal("migration 43 was registered while GATEWAY_ENABLE_TABLE_PARTITIONING was false")
	}

	// Two full months in the past (one will be dropped by retention later) plus
	// rows in the current month.
	oldMonth := usageLogMonthStart(now.AddDate(0, 0, -100))
	olderMonth := usageLogMonthStart(now.AddDate(0, 0, -200))
	seedPartitionTestUsageLog(t, plain, "part-near-1", now.Add(-2*time.Hour), 100)
	seedPartitionTestUsageLog(t, plain, "part-near-2", now.Add(-26*time.Hour), 200)
	seedPartitionTestUsageLog(t, plain, "part-old-1", oldMonth.Add(3*24*time.Hour), 300)
	seedPartitionTestUsageLog(t, plain, "part-older-1", olderMonth.Add(2*24*time.Hour), 400)
	// A NULL created_at row must land in the DEFAULT partition, not abort the copy.
	if _, err := plain.Exec(`INSERT INTO usage_logs (trace_id, billing_id, created_at) VALUES ('tr-null-created', 'part-null-created', NULL)`); err != nil {
		t.Fatalf("seed NULL created_at row: %v", err)
	}

	beforeRows, beforeUnits := usageLogTestStats(t, plain)
	var maxBeforeID int64
	if err := plain.QueryRow(`SELECT COALESCE(MAX(id), 0) FROM usage_logs`).Scan(&maxBeforeID); err != nil {
		t.Fatalf("read max id: %v", err)
	}
	if err := plain.Close(); err != nil {
		t.Fatalf("close unpartitioned database: %v", err)
	}

	// Phase 2: enable the opt-in and let startup run migration 43.
	t.Setenv(enableTablePartitioningEnv, "true")
	db, err := OpenDB(dsn)
	if err != nil {
		t.Fatalf("open partitioned scratch database: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	repo := NewRepository(db)

	relkind, err := usageLogsRelkind(db)
	if err != nil {
		t.Fatalf("read relkind: %v", err)
	}
	if relkind != "p" {
		t.Fatalf("usage_logs relkind = %q, want \"p\" (partitioned)", relkind)
	}
	if !repo.usageLogsPartitioned() {
		t.Fatal("the writer's partition probe did not detect the partitioned table")
	}
	if err := db.QueryRow(`SELECT COUNT(*) FROM schema_migrations WHERE version = 43`).Scan(&v43); err != nil {
		t.Fatalf("read schema_migrations after migration: %v", err)
	}
	if v43 != 1 {
		t.Fatalf("migration 43 recorded %d times, want 1", v43)
	}

	// Data preserved exactly: same rows, same charged units, and the pre-partition
	// table still holds everything for rollback.
	afterRows, afterUnits := usageLogTestStats(t, db)
	if afterRows != beforeRows || afterUnits != beforeUnits {
		t.Fatalf("copy changed the audit data: rows %d -> %d, units %d -> %d",
			beforeRows, afterRows, beforeUnits, afterUnits)
	}
	var legacyRows int64
	var legacyTable string
	if err := db.QueryRow(`SELECT c.relname FROM pg_class c JOIN pg_namespace n ON n.oid = c.relnamespace
		WHERE n.nspname = current_schema() AND c.relkind = 'r' AND c.relname LIKE 'usage_logs_legacy_%'
		ORDER BY c.relname LIMIT 1`).Scan(&legacyTable); err != nil {
		t.Fatalf("find legacy table: %v", err)
	}
	if err := db.QueryRow(`SELECT COUNT(*) FROM ` + quotePGIdentifier(legacyTable)).Scan(&legacyRows); err != nil {
		t.Fatalf("count legacy rows: %v", err)
	}
	if legacyRows != beforeRows {
		t.Fatalf("legacy table %s has %d rows, want %d (rollback would lose data)", legacyTable, legacyRows, beforeRows)
	}

	// Monthly partitions exist for the data months, plus the DEFAULT safety net.
	partitions, err := repo.usageLogPartitions()
	if err != nil {
		t.Fatalf("list partitions: %v", err)
	}
	names := make(map[string]struct{}, len(partitions))
	hasDefault := false
	for _, partition := range partitions {
		if partition.isDefault {
			hasDefault = true
			continue
		}
		names[partition.name] = struct{}{}
	}
	if !hasDefault {
		t.Fatal("usage_logs_default partition is missing; out-of-range rows would fail to insert")
	}
	for _, want := range []string{
		usageLogPartitionName(usageLogMonthStart(now)),
		usageLogPartitionName(oldMonth),
		usageLogPartitionName(olderMonth),
	} {
		if _, ok := names[want]; !ok {
			t.Fatalf("partition %s is missing (have %v)", want, names)
		}
	}

	// The dedupe gate moved from UNIQUE(billing_id) to usage_audit_ids, and the
	// indexes the query paths need were rebuilt on the parent.
	indexes := usageLogTableIndexes(t, db)
	if _, ok := indexes["uq_usage_billing_id"]; ok {
		t.Fatal("uq_usage_billing_id must not exist on the partitioned table")
	}
	for _, want := range []string{
		"idx_usage_created", "idx_usage_id", "idx_usage_billing_id",
		"idx_usage_tenant_created", "idx_usage_key_created", "idx_usage_model_created",
	} {
		if _, ok := indexes[want]; !ok {
			t.Fatalf("index %s is missing from the partitioned usage_logs (have %v)", want, indexes)
		}
	}
	var registryRows int
	if err := db.QueryRow(`SELECT COUNT(*) FROM ` + usageAuditIDsTable + ` WHERE billing_id LIKE 'part-near-%'`).Scan(&registryRows); err != nil {
		t.Fatalf("read audit registry: %v", err)
	}
	if registryRows != 2 {
		t.Fatalf("registry backfilled %d recent ids, want 2", registryRows)
	}

	// The id sequence must continue past the copied ids, otherwise new audit rows
	// would reuse ids and pagination (ORDER BY id) would interleave them.
	var newID int64
	if err := db.QueryRow(`INSERT INTO usage_logs (trace_id, billing_id) VALUES ('tr-seq-check', 'part-seq-check') RETURNING id`).Scan(&newID); err != nil {
		t.Fatalf("insert after switch: %v", err)
	}
	if newID <= maxBeforeID {
		t.Fatalf("new id %d did not continue past the copied max %d", newID, maxBeforeID)
	}

	// Partition pruning: a one-day current-month window must touch one partition.
	today := usageLogMonthStart(now)
	scanned := scannedPartitionCount(t, db, today, today.AddDate(0, 0, 1))
	if scanned > 2 {
		t.Fatalf("one-month query scanned %d partitions, want at most 2 (pruning is not effective)", scanned)
	}

	// Exactly-once audit rows: the second delivery of the same billing_id is skipped
	// by the registry instead of a unique index.
	dedupeID := fmt.Sprintf("part-dedupe-%d", now.UnixNano())
	for attempt := 0; attempt < 2; attempt++ {
		record := &UsageLogRecord{
			TraceID:     dedupeID,
			APIKey:      "sk-partition-test",
			TenantID:    "tenant-partition-test",
			Model:       "partition-test-model",
			Cost:        0.01,
			TotalTokens: 10,
			BillingID:   dedupeID,
			CreatedAt:   now,
		}
		if err := repo.insertUsageLogsContext(ctx, []*UsageLogRecord{record}); err != nil {
			t.Fatalf("audit delivery %d: %v", attempt+1, err)
		}
	}
	var auditRows int
	if err := db.QueryRow(`SELECT COUNT(*) FROM usage_logs WHERE billing_id = $1`, dedupeID).Scan(&auditRows); err != nil {
		t.Fatalf("count deduped audit rows: %v", err)
	}
	if auditRows != 1 {
		t.Fatalf("redelivery produced %d audit rows, want exactly 1", auditRows)
	}

	// End to end through the real delivery API: settlement and audit insertion
	// together, twice for the same billing_id. The money must move once (the ledger
	// keeps its own unique key) and the audit row must exist once (the registry).
	suffix := now.UnixNano()
	settleUser := &UserRecord{
		Username:     fmt.Sprintf("part-settle-user-%d", suffix),
		Email:        fmt.Sprintf("part-settle-%d@example.invalid", suffix),
		PasswordHash: "synthetic",
	}
	if err := repo.CreateUser(settleUser); err != nil {
		t.Fatalf("create settle user: %v", err)
	}
	if err := repo.SetUserBalance(settleUser.Username, 10); err != nil {
		t.Fatalf("set settle balance: %v", err)
	}
	settleKey := &APIKeyRecord{Key: fmt.Sprintf("sk-part-settle-%d", suffix), UserID: settleUser.ID, TenantID: settleUser.Username}
	if err := repo.CreateAPIKey(settleKey); err != nil {
		t.Fatalf("create settle key: %v", err)
	}
	settleBillID := fmt.Sprintf("part-settle-bill-%d", suffix)
	deliver := func() []*UsageLogRecord {
		return []*UsageLogRecord{{
			TraceID:      settleBillID,
			APIKey:       settleKey.Key,
			TenantID:     settleUser.Username,
			Model:        "partition-test-model",
			PromptTokens: 500,
			TotalTokens:  1000,
			Cost:         0.5,
			StatusCode:   200,
			BillingID:    settleBillID,
			CreatedAt:    now,
		}}
	}
	for attempt := 0; attempt < 2; attempt++ {
		if err := repo.BatchRecordUsageLogsContext(ctx, deliver()); err != nil {
			t.Fatalf("partitioned delivery %d: %v", attempt+1, err)
		}
	}
	settledUser, err := repo.GetUserByID(settleUser.ID)
	if err != nil {
		t.Fatalf("reload settle user: %v", err)
	}
	if want := ToUnits(9.5); settledUser.BalanceUnits != want {
		t.Fatalf("balance_units = %d after redelivery, want %d (charged exactly once)", settledUser.BalanceUnits, want)
	}
	var settleLedgerRows, settleAuditRows int
	if err := db.QueryRow(`SELECT COUNT(*) FROM billing_settlements WHERE billing_id = $1`, settleBillID).Scan(&settleLedgerRows); err != nil {
		t.Fatalf("count settlement ledger: %v", err)
	}
	if settleLedgerRows != 1 {
		t.Fatalf("billing_settlements rows = %d for one charge, want 1", settleLedgerRows)
	}
	if err := db.QueryRow(`SELECT COUNT(*) FROM usage_logs WHERE billing_id = $1`, settleBillID).Scan(&settleAuditRows); err != nil {
		t.Fatalf("count partitioned audit rows: %v", err)
	}
	if settleAuditRows != 1 {
		t.Fatalf("partitioned audit rows = %d for one charge, want 1", settleAuditRows)
	}

	// Idempotency: deleting the version record simulates a crash between the
	// committed switch and the version insert; the re-run must detect relkind='p'
	// and record the version without touching data.
	if _, err := db.Exec(`DELETE FROM schema_migrations WHERE version = 43`); err != nil {
		t.Fatalf("drop version record: %v", err)
	}
	if err := db.migratePostgres(); err != nil {
		t.Fatalf("re-running the migration on an already partitioned table: %v", err)
	}
	rowsAfterRerun, unitsAfterRerun := usageLogTestStats(t, db)
	// Three rows were added after the post-migration measurement: the sequence check
	// insert, one deduplicated audit row (the second delivery was skipped), and one
	// settled audit row; the re-run must not have added anything else.
	if rowsAfterRerun != afterRows+3 || unitsAfterRerun != afterUnits+510_000_000 {
		t.Fatalf("re-run changed data: rows %d, units %d (want rows %d, units %d)",
			rowsAfterRerun, unitsAfterRerun, afterRows+3, afterUnits+510_000_000)
	}
	var versions int
	if err := db.QueryRow(`SELECT COUNT(*) FROM schema_migrations WHERE version = 43`).Scan(&versions); err != nil {
		t.Fatalf("read version after re-run: %v", err)
	}
	if versions != 1 {
		t.Fatalf("version 43 recorded %d times after re-run, want 1", versions)
	}
	var legacyTables int
	if err := db.QueryRow(`SELECT COUNT(*) FROM pg_class c JOIN pg_namespace n ON n.oid = c.relnamespace
		WHERE n.nspname = current_schema() AND c.relkind = 'r' AND c.relname LIKE 'usage_logs_legacy_%'`).Scan(&legacyTables); err != nil {
		t.Fatalf("count legacy tables: %v", err)
	}
	if legacyTables != 1 {
		t.Fatalf("legacy tables = %d after re-run, want 1 (the re-run must be a no-op)", legacyTables)
	}
}

// TestUsageLogsPartitionRetentionDropsExpiredMonths verifies the partition-aware
// retention: whole expired months are dropped with DROP TABLE, the boundary month is
// trimmed by the batched DELETE, surviving data stays intact, and the pruned-units
// baseline keeps reconciliation balanced.
func TestUsageLogsPartitionRetentionDropsExpiredMonths(t *testing.T) {
	dsn := partitionScratchDSN(t)
	now := time.Now().UTC()

	t.Setenv(enableTablePartitioningEnv, "false")
	plain, err := OpenDB(dsn)
	if err != nil {
		t.Fatalf("open unpartitioned scratch database: %v", err)
	}
	// fully expired month (will be DROPped), boundary month (kept, trimmed), and a
	// recent row that must survive.
	expiredMonth := usageLogMonthStart(now.AddDate(0, 0, -300))
	boundaryMonth := usageLogMonthStart(now.AddDate(0, 0, -100))
	seedPartitionTestUsageLog(t, plain, "ret-expired-1", expiredMonth.Add(2*24*time.Hour), 500)
	seedPartitionTestUsageLog(t, plain, "ret-expired-2", expiredMonth.Add(5*24*time.Hour), 700)
	seedPartitionTestUsageLog(t, plain, "ret-boundary-1", boundaryMonth.Add(2*24*time.Hour), 900)
	seedPartitionTestUsageLog(t, plain, "ret-kept-1", now.Add(-24*time.Hour), 1100)
	if err := plain.Close(); err != nil {
		t.Fatalf("close unpartitioned database: %v", err)
	}

	t.Setenv(enableTablePartitioningEnv, "true")
	db, err := OpenDB(dsn)
	if err != nil {
		t.Fatalf("open partitioned scratch database: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	repo := NewRepository(db)

	// Audit retention cutoff: everything older than 90 days goes away.
	cutoff := now.AddDate(0, 0, -90)
	deleted, err := repo.PurgeUsageLogsBefore(cutoff, 1000)
	if err != nil {
		t.Fatalf("partition retention: %v", err)
	}
	// Three rows are older than the cutoff: two in the dropped month, one in the
	// boundary month (removed by the batched DELETE).
	if deleted != 3 {
		t.Fatalf("retention deleted %d rows, want 3", deleted)
	}

	names := make(map[string]struct{})
	partitions, err := repo.usageLogPartitions()
	if err != nil {
		t.Fatalf("list partitions: %v", err)
	}
	for _, partition := range partitions {
		names[partition.name] = struct{}{}
	}
	if _, ok := names[usageLogPartitionName(expiredMonth)]; ok {
		t.Fatalf("expired partition %s still exists", usageLogPartitionName(expiredMonth))
	}
	if _, ok := names[usageLogPartitionName(boundaryMonth)]; !ok {
		// The boundary month must survive: its future half is still inside retention.
		t.Fatalf("boundary partition %s was dropped", usageLogPartitionName(boundaryMonth))
	}

	for _, gone := range []string{"ret-expired-1", "ret-expired-2", "ret-boundary-1"} {
		var count int
		if err := db.QueryRow(`SELECT COUNT(*) FROM usage_logs WHERE billing_id = $1`, gone).Scan(&count); err != nil {
			t.Fatalf("count %s: %v", gone, err)
		}
		if count != 0 {
			t.Fatalf("%s survived retention", gone)
		}
	}
	var kept int
	if err := db.QueryRow(`SELECT COUNT(*) FROM usage_logs WHERE billing_id = 'ret-kept-1'`).Scan(&kept); err != nil {
		t.Fatalf("count kept row: %v", err)
	}
	if kept != 1 {
		t.Fatal("the row inside the retention window was deleted")
	}

	// Reconciliation must be able to explain the removal: 500+700 (dropped month)
	// + 900 (boundary row) charged units are on the pruned baseline.
	if got, want := repo.UsageAuditPrunedUnits(), int64(2100); got != want {
		t.Fatalf("pruned audit units = %d, want %d (retention must keep reconciliation balanced)", got, want)
	}

	// The dedupe registry has its own, shorter clock: rows older than 30 days go,
	// recent ones stay so a redelivery inside the delivery window is still caught.
	old := now.AddDate(0, 0, -(usageAuditIDRetentionDays + 1))
	recent := now.Add(-time.Hour)
	for _, row := range []struct {
		id string
		at time.Time
	}{
		{"registry-old", old},
		{"registry-recent", recent},
	} {
		if _, err := db.Exec(`INSERT INTO `+usageAuditIDsTable+` (billing_id, created_at) VALUES (?, ?)
			ON CONFLICT (billing_id) DO NOTHING`, row.id, row.at); err != nil {
			t.Fatalf("seed registry row %s: %v", row.id, err)
		}
	}
	prunedIDs, err := repo.PurgeUsageAuditIDsBefore(now.Add(-usageAuditIDRetention), 100)
	if err != nil {
		t.Fatalf("registry retention: %v", err)
	}
	if prunedIDs < 1 {
		t.Fatalf("registry retention deleted %d rows, want at least 1", prunedIDs)
	}
	var oldRows, recentRows int
	if err := db.QueryRow(`SELECT COUNT(*) FROM ` + usageAuditIDsTable + ` WHERE billing_id = 'registry-old'`).Scan(&oldRows); err != nil {
		t.Fatalf("count old registry row: %v", err)
	}
	if err := db.QueryRow(`SELECT COUNT(*) FROM ` + usageAuditIDsTable + ` WHERE billing_id = 'registry-recent'`).Scan(&recentRows); err != nil {
		t.Fatalf("count recent registry row: %v", err)
	}
	if oldRows != 0 {
		t.Fatal("registry row older than the dedupe window survived")
	}
	if recentRows != 1 {
		t.Fatal("recent registry row was deleted; redeliveries would duplicate")
	}
}

// TestSQLiteKeepsSingleTableAuditDedupe pins the non-partitioned path: even with the
// opt-in flag set, SQLite never registers migration 43 and keeps using the
// UNIQUE(billing_id) conflict gate.
func TestSQLiteKeepsSingleTableAuditDedupe(t *testing.T) {
	t.Setenv(enableTablePartitioningEnv, "true")
	dsn := ":memory:"
	db, err := OpenDB(dsn)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	repo := NewRepository(db)

	if repo.usageLogsPartitioned() {
		t.Fatal("SQLite reported a partitioned usage_logs")
	}
	var registryTables int
	if err := db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = ?`, usageAuditIDsTable).Scan(&registryTables); err != nil {
		t.Fatalf("inspect sqlite schema: %v", err)
	}
	if registryTables != 0 {
		t.Fatal("SQLite must not create the PostgreSQL-only audit dedupe registry")
	}

	billingID := "sqlite-dedupe"
	for attempt := 0; attempt < 2; attempt++ {
		record := &UsageLogRecord{
			TraceID:     billingID,
			APIKey:      "sk-sqlite",
			TenantID:    "sqlite-tenant",
			Model:       "partition-test-model",
			Cost:        0.01,
			TotalTokens: 10,
			BillingID:   billingID,
			CreatedAt:   time.Now(),
		}
		if err := repo.insertUsageLogsContext(context.Background(), []*UsageLogRecord{record}); err != nil {
			t.Fatalf("sqlite audit delivery %d: %v", attempt+1, err)
		}
	}
	var rows int
	if err := db.QueryRow(`SELECT COUNT(*) FROM usage_logs WHERE billing_id = ?`, billingID).Scan(&rows); err != nil {
		t.Fatalf("count sqlite audit rows: %v", err)
	}
	if rows != 1 {
		t.Fatalf("sqlite redelivery produced %d rows, want exactly 1", rows)
	}
}

// TestUsageLogsPartitionMigrationDefersWhenLocked pins the safety valve the task
// requires: if the copy-switch cannot take its ACCESS EXCLUSIVE lock within
// lock_timeout, the migration is deferred (startup continues, the version is not
// recorded) and a later start retries it successfully.
func TestUsageLogsPartitionMigrationDefersWhenLocked(t *testing.T) {
	dsn := partitionScratchDSN(t)
	now := time.Now().UTC()

	t.Setenv(enableTablePartitioningEnv, "false")
	plain, err := OpenDB(dsn)
	if err != nil {
		t.Fatalf("open unpartitioned scratch database: %v", err)
	}
	seedPartitionTestUsageLog(t, plain, "deferred-1", now.Add(-time.Hour), 100)
	if err := plain.Close(); err != nil {
		t.Fatalf("close unpartitioned database: %v", err)
	}

	// Hold the audit table exclusively from a separate connection, simulating a
	// database that is too busy to allow the maintenance window right now.
	holder, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatalf("open lock holder: %v", err)
	}
	defer func() { _ = holder.Close() }()
	holderTx, err := holder.Begin()
	if err != nil {
		t.Fatalf("begin lock holder transaction: %v", err)
	}
	if _, err := holderTx.Exec(`LOCK TABLE usage_logs IN ACCESS EXCLUSIVE MODE`); err != nil {
		_ = holderTx.Rollback()
		t.Fatalf("lock usage_logs: %v", err)
	}

	t.Setenv(enableTablePartitioningEnv, "true")
	db, err := OpenDB(dsn)
	if err != nil {
		_ = holderTx.Rollback()
		t.Fatalf("startup must not fail on a deferred partition migration: %v", err)
	}
	relkind, err := usageLogsRelkind(db)
	if err != nil {
		_ = holderTx.Rollback()
		t.Fatalf("relkind while deferred: %v", err)
	}
	if relkind != "r" {
		_ = holderTx.Rollback()
		t.Fatalf("usage_logs relkind = %q while the lock is held, want the untouched plain table", relkind)
	}
	var version int
	if err := db.QueryRow(`SELECT COUNT(*) FROM schema_migrations WHERE version = 43`).Scan(&version); err != nil {
		_ = holderTx.Rollback()
		t.Fatalf("read version while deferred: %v", err)
	}
	if version != 0 {
		_ = holderTx.Rollback()
		t.Fatal("a deferred migration must not be recorded, otherwise it never retries")
	}

	// Release the lock and retry, as the next startup would.
	_ = holderTx.Rollback()
	if err := db.migratePostgres(); err != nil {
		t.Fatalf("retry after the lock was released: %v", err)
	}
	relkind, err = usageLogsRelkind(db)
	if err != nil {
		t.Fatalf("relkind after retry: %v", err)
	}
	if relkind != "p" {
		t.Fatalf("usage_logs relkind = %q after retry, want \"p\"", relkind)
	}
	if err := db.QueryRow(`SELECT COUNT(*) FROM schema_migrations WHERE version = 43`).Scan(&version); err != nil {
		t.Fatalf("read version after retry: %v", err)
	}
	if version != 1 {
		t.Fatalf("version 43 recorded %d times after retry, want 1", version)
	}
	_ = db.Close()
}

// TestUsageLogsPartitionMigrationOnEmptyTable covers a fresh deployment that turns
// the flag on before any audit traffic exists: the month-range math has no rows to
// learn from, so it must still provision the current month and the lookahead.
func TestUsageLogsPartitionMigrationOnEmptyTable(t *testing.T) {
	dsn := partitionScratchDSN(t)

	t.Setenv(enableTablePartitioningEnv, "false")
	plain, err := OpenDB(dsn)
	if err != nil {
		t.Fatalf("open unpartitioned scratch database: %v", err)
	}
	if err := plain.Close(); err != nil {
		t.Fatalf("close unpartitioned database: %v", err)
	}

	t.Setenv(enableTablePartitioningEnv, "true")
	db, err := OpenDB(dsn)
	if err != nil {
		t.Fatalf("migrate an empty usage_logs: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	repo := NewRepository(db)
	partitions, err := repo.usageLogPartitions()
	if err != nil {
		t.Fatalf("list partitions: %v", err)
	}
	monthly, hasDefault := 0, false
	for _, partition := range partitions {
		if partition.isDefault {
			hasDefault = true
			continue
		}
		monthly++
	}
	if !hasDefault {
		t.Fatal("default partition missing on an empty table")
	}
	if monthly < usageLogPartitionMonthsAhead+1 {
		t.Fatalf("provisioned %d monthly partitions, want at least %d (current month + lookahead)",
			monthly, usageLogPartitionMonthsAhead+1)
	}
	if _, err := db.Exec(`INSERT INTO usage_logs (trace_id, billing_id) VALUES ('tr-empty-table', 'part-empty-table')`); err != nil {
		t.Fatalf("insert after an empty-table migration: %v", err)
	}
	var rows int
	if err := db.QueryRow(`SELECT COUNT(*) FROM usage_logs`).Scan(&rows); err != nil {
		t.Fatalf("count after insert: %v", err)
	}
	if rows != 1 {
		t.Fatalf("rows = %d, want 1", rows)
	}
}
