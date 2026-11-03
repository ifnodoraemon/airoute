package storage

// Regression tests for JOBS-06 / JOBS-07 (2026-10-29 bughunt round 3).
//
// JOBS-06: a ledger-retention batch that collides with another replica must not be
// reported as "nothing to do" (which silently ended the pass); it is classified,
// counted and retried once.
// JOBS-07: an opportunistic retention prune whose DELETE fails must not consume the
// once-per-hour throttle slot, otherwise one transient database error freezes that
// table's cleanup for a full interval.

import (
	"errors"
	"sync/atomic"
	"testing"
	"time"
)

func TestLedgerPruneDeletionMismatchClassification(t *testing.T) {
	if err := ledgerPruneDeletionMismatch(10, 10); err != nil {
		t.Fatalf("matching counts must not be reported as contention, got %v", err)
	}
	err := ledgerPruneDeletionMismatch(9, 10)
	if !errors.Is(err, errBillingLedgerPruneContended) {
		t.Fatalf("a DELETE that removed fewer rows than scanned must be contended, got %v", err)
	}
	// More rows than scanned is also a mismatch (a trigger or a manual write), and
	// the archive must not be trusted for either direction.
	if err := ledgerPruneDeletionMismatch(11, 10); !errors.Is(err, errBillingLedgerPruneContended) {
		t.Fatalf("a DELETE that removed more rows than scanned must be contended, got %v", err)
	}
}

func TestMaybePruneFailureAllowsFasterRetry(t *testing.T) {
	db, err := OpenDB(":memory:")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer db.Close()
	repo := NewRepository(db)

	var slot atomic.Int64
	t.Setenv("GATEWAY_VERIFICATION_RETENTION_DAYS", "7")
	repo.maybePrune(&slot, verificationRetention,
		func(time.Time) (int64, error) { return 0, errors.New("database is down") },
		"verification-prune-failed", "failed to prune", "pruned")

	stored := slot.Load()
	if stored == 0 {
		t.Fatal("the throttle slot must be written so concurrent writers do not all prune")
	}
	// The slot must now describe "this attempt happened (interval - retryInterval)
	// ago", i.e. the next attempt becomes due after retentionPruneRetryInterval
	// instead of a full retentionPruneInterval.
	dueAt := time.Unix(0, stored).Add(retentionPruneInterval)
	untilDue := time.Until(dueAt)
	if untilDue > retentionPruneRetryInterval+5*time.Second {
		t.Fatalf("a failed prune must be retried in about %s, but the next attempt is %s away",
			retentionPruneRetryInterval, untilDue)
	}
	if untilDue < 0 {
		t.Fatalf("a failed prune must not become due immediately (retry storm), got %s", untilDue)
	}
	// And the retry gate must still reject an immediate second attempt.
	if shouldRunRetention(&slot, retentionPruneInterval) {
		t.Fatal("an immediate retry must stay throttled")
	}
}

func TestMaybePruneSuccessKeepsFullInterval(t *testing.T) {
	db, err := OpenDB(":memory:")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer db.Close()
	repo := NewRepository(db)

	var slot atomic.Int64
	calls := 0
	purge := func(time.Time) (int64, error) { calls++; return 3, nil }
	repo.maybePrune(&slot, verificationRetention, purge, "k", "m", "i")
	repo.maybePrune(&slot, verificationRetention, purge, "k", "m", "i")
	if calls != 1 {
		t.Fatalf("a successful prune must keep the once-per-interval throttle, ran %d times", calls)
	}
}

func TestRescheduleRetentionRetryEdgeCases(t *testing.T) {
	var slot atomic.Int64
	// A nil slot must be a no-op rather than a panic.
	rescheduleRetentionRetry(nil, time.Hour, time.Minute)

	// A retry window that is not shorter than the interval cannot express "retry
	// sooner": fall back to "due now".
	slot.Store(123)
	rescheduleRetentionRetry(&slot, time.Hour, 2*time.Hour)
	if slot.Load() != 0 {
		t.Fatalf("a non-shorter retry interval must clear the slot, got %d", slot.Load())
	}
}
