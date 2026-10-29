package storage

// Regression tests for JOBS-03 (2026-10-29 bughunt round 3): the local usage-log
// spill file must be replayed continuously, not only once at process start.
//
// Before the fix the replay ran in a one-shot goroutine started by
// InitAsyncLogger: a database outage that began *after* boot left the spilled
// records (and, for charges the settlement batcher had deferred, the money) on
// disk until the next restart, and they were dropped outright once the file hit
// its size cap. The loop now replays periodically and is kicked immediately after
// a spill, so a recovered database drains the file within moments.

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// waitForUsageLogCount polls until usage_logs holds want rows with billingID.
func waitForUsageLogCount(t *testing.T, repo *Repository, billingID string, want int, timeout time.Duration) int {
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
		time.Sleep(20 * time.Millisecond)
	}
	return got
}

func waitForCondition(t *testing.T, timeout time.Duration, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", what)
}

// TestUsageSpillIsReplayedAfterStartup is the flipped JOBS-03 probe: a record
// spilled after the process started must reach the database and its file must be
// removed, without a restart.
func TestUsageSpillIsReplayedAfterStartup(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("GATEWAY_USAGE_SPILL_DIR", dir)
	// A file left by "a previous process" is claimable immediately.
	t.Setenv("GATEWAY_USAGE_SPILL_CLAIM_GRACE_SEC", "0")
	// Short interval so the periodic path (not just the kick) is exercised.
	t.Setenv("GATEWAY_USAGE_SPILL_REPLAY_SEC", "1")

	db, err := OpenDB(":memory:")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer db.Close()
	repo := NewRepository(db)

	prevLogger := GlobalAsyncLogger
	defer func() { GlobalAsyncLogger = prevLogger }()

	// A file that already exists at boot is replayed by the first pass.
	bootSpill := newUsageSpill()
	if _, err := bootSpill.append([]*UsageLogRecord{
		{TraceID: "tr-spill-boot", BillingID: "bill-spill-boot", CreatedAt: time.Now()},
	}); err != nil {
		t.Fatalf("append boot spill record: %v", err)
	}

	al := InitAsyncLogger(repo, 64, 8, 40*time.Millisecond)
	defer al.Stop()

	if got := waitForUsageLogCount(t, repo, "bill-spill-boot", 1, 3*time.Second); got != 1 {
		t.Fatalf("startup replay: usage_logs rows = %d, want 1", got)
	}

	// A spill produced *after* startup (what the worker writes when flushes keep
	// failing) must be drained by the running process.
	lateBillingID := "bill-spill-late"
	al.spillRecords([]*UsageLogRecord{
		{TraceID: "tr-spill-late", BillingID: lateBillingID, CreatedAt: time.Now()},
	}, "test: database unavailable")

	if got := waitForUsageLogCount(t, repo, lateBillingID, 1, 5*time.Second); got != 1 {
		t.Fatalf("post-startup spill was not replayed: usage_logs rows = %d, want 1", got)
	}
	// The replayed file must be gone: leaving it would replay it again forever.
	waitForCondition(t, 3*time.Second, "the replayed spill file to be removed", func() bool {
		_, statErr := os.Stat(filepath.Join(dir, filepath.Base(al.spill.path)))
		return os.IsNotExist(statErr)
	})
}

// TestUsageSpillReplayPicksUpAnotherProcessFile covers the periodic pass: a file
// written by another replica (or by an earlier process) becomes claimable once it
// goes quiet, and must be drained without waiting for a restart. This is the case
// two replicas share the gateway-data volume.
func TestUsageSpillReplayPicksUpAnotherProcessFile(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("GATEWAY_USAGE_SPILL_DIR", dir)
	t.Setenv("GATEWAY_USAGE_SPILL_CLAIM_GRACE_SEC", "0")
	t.Setenv("GATEWAY_USAGE_SPILL_REPLAY_SEC", "1")

	db, err := OpenDB(":memory:")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer db.Close()
	repo := NewRepository(db)

	prevLogger := GlobalAsyncLogger
	defer func() { GlobalAsyncLogger = prevLogger }()

	al := InitAsyncLogger(repo, 64, 8, 40*time.Millisecond)
	defer al.Stop()

	// A peer's spill file appears after our startup pass.
	peer := newUsageSpill()
	if _, err := peer.append([]*UsageLogRecord{
		{TraceID: "tr-spill-peer", BillingID: "bill-spill-peer", CreatedAt: time.Now()},
	}); err != nil {
		t.Fatalf("append peer spill record: %v", err)
	}
	// The replay interval is 1s, but the first pass runs at init (before this file
	// existed), so the first claimable pass is the second tick. Under `-race` with
	// every package running in parallel the goroutine can be scheduled late enough
	// that a 5s bound flakes (observed once); 20s still fails fast on a real
	// regression while absorbing scheduler noise.
	if got := waitForUsageLogCount(t, repo, "bill-spill-peer", 1, 20*time.Second); got != 1 {
		t.Fatalf("peer spill file was not replayed by the periodic pass: rows = %d", got)
	}
}
