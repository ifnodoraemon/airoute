package storage

import (
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// newMigrationsRepo opens a fresh file-backed SQLite database, which means the
// whole migration set (including v42) runs exactly as it does on boot.
func newMigrationsRepo(t *testing.T) *Repository {
	t.Helper()
	db, err := OpenDB(filepath.Join(t.TempDir(), "migrations.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return NewRepository(db)
}

// E-07/E-16: migration 42 drops the single-column usage_logs indexes no query can
// use and replaces the unusable composite video_tasks index.
func TestStorageRepairMigrationIndexes(t *testing.T) {
	repo := newMigrationsRepo(t)
	db := repo.GetDB()

	var applied int
	if err := db.QueryRow(`SELECT COUNT(*) FROM schema_migrations WHERE version = 42`).Scan(&applied); err != nil {
		t.Fatalf("read schema_migrations: %v", err)
	}
	if applied != 1 {
		t.Fatalf("migration 42 recorded %d times, want 1", applied)
	}

	for _, name := range []string{"idx_usage_trace", "idx_usage_chat", "idx_usage_session"} {
		if indexExists(t, repo, name) {
			t.Errorf("index %q should have been dropped: the filter is `col = ? OR col LIKE '%%?%%'`, which it cannot serve", name)
		}
	}

	if !indexExists(t, repo, "idx_video_tasks_created") {
		t.Error("idx_video_tasks_created is missing; the retention purge scans created_at")
	}
	if !indexExists(t, repo, "idx_video_tasks_owner") {
		t.Error("idx_video_tasks_owner is missing")
	}
	var ownerSQL string
	if err := db.QueryRow(`SELECT COALESCE(sql, '') FROM sqlite_master WHERE type = 'index' AND name = 'idx_video_tasks_owner'`).Scan(&ownerSQL); err != nil {
		t.Fatalf("read index SQL: %v", err)
	}
	if ownerSQL == "" || !strings.Contains(ownerSQL, "(owner)") {
		t.Errorf("idx_video_tasks_owner should index (owner) alone, got %q", ownerSQL)
	}
}

// E-10: the FTS5 index follows UPDATEs, which matters because migrations 24/32
// rewrite usage_logs columns that are indexed.
func TestStorageRepairMigrationFTSUpdateTrigger(t *testing.T) {
	repo := newMigrationsRepo(t)
	db := repo.GetDB()

	var triggers int
	if err := db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type = 'trigger' AND name = 'usage_logs_fts_au'`).Scan(&triggers); err != nil {
		t.Fatalf("read triggers: %v", err)
	}
	if triggers != 1 {
		t.Fatalf("usage_logs_fts_au trigger count = %d, want 1", triggers)
	}

	rec := &UsageLogRecord{TraceID: "tr-fts", Model: "m", StatusCode: 500, ErrorMessage: "legacy artefact text"}
	if err := repo.RecordUsageLogContext(t.Context(), rec); err != nil {
		t.Fatalf("record usage: %v", err)
	}
	if got := countLogsWithKeyword(t, repo, "artefact"); got != 1 {
		t.Fatalf("keyword search found %d rows before the update, want 1", got)
	}

	// Simulate what the reason-repair migrations do to an already-indexed column.
	if _, err := db.Exec(`UPDATE usage_logs SET error_message = ? WHERE billing_id = ?`,
		"reconstructed circuit breaker reason", rec.BillingID); err != nil {
		t.Fatalf("update error_message: %v", err)
	}

	if got := countLogsWithKeyword(t, repo, "artefact"); got != 0 {
		t.Fatalf("stale FTS entry still matches the old text (%d rows)", got)
	}
	if got := countLogsWithKeyword(t, repo, "reconstructed"); got != 1 {
		t.Fatalf("FTS index did not follow the update (%d rows for the new text)", got)
	}
}

func countLogsWithKeyword(t *testing.T, repo *Repository, keyword string) int {
	t.Helper()
	logs, err := repo.ListUsageLogsWithFilter(LogFilter{Keyword: keyword, Limit: 50})
	if err != nil {
		t.Fatalf("keyword search %q: %v", keyword, err)
	}
	return len(logs)
}

// Migration 42 must be replay-safe: the runner may replay it when the process dies
// between the DDL and the version record.
func TestStorageRepairMigrationIsIdempotent(t *testing.T) {
	repo := newMigrationsRepo(t)
	db := repo.GetDB()
	for i := 0; i < 2; i++ {
		if err := repairStorageIndexesAndTraceSearch(db); err != nil {
			t.Fatalf("replay %d: %v", i+1, err)
		}
	}
	if !indexExists(t, repo, "idx_video_tasks_created") {
		t.Fatal("index disappeared on replay")
	}
	if indexExists(t, repo, "idx_usage_trace") {
		t.Fatal("dropped index came back on replay")
	}
}

// E-07 behaviour: the retention purge only needs created_at, so the dedicated
// index must be the one the query can use.
func TestVideoTaskRetentionStillWorksAfterIndexChange(t *testing.T) {
	repo := newMigrationsRepo(t)
	if _, err := repo.db.Exec(`INSERT INTO video_tasks (id, upstream_id, owner, model, channel_id, channel_name, base_url, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		"task-old", "up", "alice", "m", 1, "ch", "http://upstream.invalid",
		repo.dialectTime(time.Now().AddDate(0, 0, -90))); err != nil {
		t.Fatalf("insert old task: %v", err)
	}
	deleted, err := repo.PurgeVideoTasksBefore(time.Now().AddDate(0, 0, -30))
	if err != nil {
		t.Fatalf("purge: %v", err)
	}
	if deleted != 1 {
		t.Fatalf("purge deleted %d rows, want 1", deleted)
	}
}
