package storage

import (
	"context"
	"fmt"
	"strconv"
	"testing"
	"time"
)

// TestResolveBillingLedgerRetention pins the opt-in defaults: an unset or zero
// window keeps every ledger row.
func TestResolveBillingLedgerRetention(t *testing.T) {
	t.Setenv("GATEWAY_BILLING_LEDGER_RETENTION_DAYS", "")
	t.Setenv("GATEWAY_BILLING_LEDGER_PRUNE_INTERVAL_HOURS", "")
	retention, interval := ResolveBillingLedgerRetention()
	if retention != 0 {
		t.Fatalf("default retention = %v, want 0 (the ledger must be kept by default)", retention)
	}
	if interval != 24*time.Hour {
		t.Fatalf("default interval = %v, want 24h", interval)
	}

	t.Setenv("GATEWAY_BILLING_LEDGER_RETENTION_DAYS", "0")
	if retention, _ := ResolveBillingLedgerRetention(); retention != 0 {
		t.Fatalf("retention = %v, want 0", retention)
	}

	t.Setenv("GATEWAY_BILLING_LEDGER_RETENTION_DAYS", "730")
	t.Setenv("GATEWAY_BILLING_LEDGER_PRUNE_INTERVAL_HOURS", "6")
	retention, interval = ResolveBillingLedgerRetention()
	if retention != 730*24*time.Hour {
		t.Fatalf("retention = %v, want 730 days", retention)
	}
	if interval != 6*time.Hour {
		t.Fatalf("interval = %v, want 6h", interval)
	}

	// Garbage falls back to the safe default instead of deleting something.
	t.Setenv("GATEWAY_BILLING_LEDGER_RETENTION_DAYS", "-5")
	if retention, _ := ResolveBillingLedgerRetention(); retention != 0 {
		t.Fatalf("negative retention = %v, want 0", retention)
	}
}

// TestBillingLedgerRetentionDisabledByDefault proves the worker is inert unless an
// operator opts in.
func TestBillingLedgerRetentionDisabledByDefault(t *testing.T) {
	t.Setenv("GATEWAY_BILLING_LEDGER_RETENTION_DAYS", "0")

	r, key, u := billingIDTestRepo(t)
	ctx := context.Background()
	record := billingRecord(key, u.Username, "tr-keep", "bill-keep", 1.00)
	record.CreatedAt = time.Now().Add(-400 * 24 * time.Hour)
	if err := r.RecordUsageLogContext(ctx, record); err != nil {
		t.Fatal(err)
	}
	// Remove the audit evidence so the row would be retirable if the policy were on.
	if _, err := r.PurgeUsageLogsBefore(time.Now().Add(-300*24*time.Hour), 100); err != nil {
		t.Fatal(err)
	}

	worker := InitBillingLedgerRetention(r)
	defer worker.Stop()

	deleted, units, err := worker.pruneOnce()
	if err != nil {
		t.Fatalf("pruneOnce: %v", err)
	}
	if deleted != 0 || units != 0 {
		t.Fatalf("disabled retention deleted %d rows / %d units, want a no-op", deleted, units)
	}

	var remaining int
	if err := r.db.QueryRow(`SELECT COUNT(*) FROM billing_settlements WHERE billing_id = 'bill-keep'`).Scan(&remaining); err != nil {
		t.Fatal(err)
	}
	if remaining != 1 {
		t.Fatalf("ledger row was deleted with retention disabled (remaining = %d)", remaining)
	}
}

// TestBillingLedgerRetentionRetiresOnlyRowsWithoutAuditEvidence is the safety
// contract: only rows whose audit row is already gone AND whose created_at is
// older than the cutoff are retired; everything else stays, the money moves into
// the archive aggregate, and both reconciliation gauges stay at zero.
func TestBillingLedgerRetentionRetiresOnlyRowsWithoutAuditEvidence(t *testing.T) {
	previous := billingAuditLagOverlapWindow
	billingAuditLagOverlapWindow = 0
	t.Cleanup(func() { billingAuditLagOverlapWindow = previous })

	r, key, u := billingIDTestRepo(t)
	ctx := context.Background()

	now := time.Now().UTC()
	ancient := now.AddDate(0, 0, -400)
	middle := now.AddDate(0, 0, -200)

	// 1. A balanced ancient charge: its audit row is pruned below, so the ledger row
	//    becomes retirable.
	oldPruned := billingRecord(key, u.Username, "tr-ret-old-pruned", "bill-ret-old-pruned", 1.00)
	oldPruned.CreatedAt = ancient
	if err := r.RecordUsageLogContext(ctx, oldPruned); err != nil {
		t.Fatal(err)
	}
	// 2. An ancient settlement whose audit row never existed: retirable.
	oldUnlinked := billingRecord(key, u.Username, "tr-ret-old-unlinked", "bill-ret-old-unlinked", 2.00)
	oldUnlinked.CreatedAt = ancient
	if err := r.SettleUsageCharge(ctx, oldUnlinked); err != nil {
		t.Fatal(err)
	}
	// 3. An older-than-cutoff charge whose audit row still exists: must be kept.
	oldAudited := billingRecord(key, u.Username, "tr-ret-old-audited", "bill-ret-old-audited", 8.00)
	oldAudited.CreatedAt = middle
	if err := r.RecordUsageLogContext(ctx, oldAudited); err != nil {
		t.Fatal(err)
	}
	// 4. A recent balanced charge: younger than the cutoff, must be kept.
	recent := billingRecord(key, u.Username, "tr-ret-recent", "bill-ret-recent", 4.00)
	recent.CreatedAt = now
	if err := r.RecordUsageLogContext(ctx, recent); err != nil {
		t.Fatal(err)
	}

	// Baseline both gauges with all four rows present. The settlement without an
	// audit row (2.00) is the genuine, expected lag; everything else is balanced.
	worker := &BillingReconciler{repo: r, stop: make(chan struct{})}
	lagBefore, err := worker.auditLag()
	if err != nil {
		t.Fatalf("audit lag before retention: %v", err)
	}
	if want := ToUnits(2.00); lagBefore != want {
		t.Fatalf("audit lag before retention = %d units, want %d (the charge with no audit row)", lagBefore, want)
	}
	if drift, _, err := worker.ledgerDrift(); err != nil || drift != 0 {
		t.Fatalf("ledger drift before retention = %d (%v), want 0", drift, err)
	}

	// Audit retention removes the ancient evidence and moves its units to the pruned
	// baseline.
	purged, err := r.PurgeUsageLogsBefore(now.AddDate(0, 0, -300), 100)
	if err != nil {
		t.Fatalf("prune audit rows: %v", err)
	}
	if purged != 1 {
		t.Fatalf("pruned %d audit rows, want 1", purged)
	}

	retirer := &BillingLedgerRetention{repo: r, retention: 100 * 24 * time.Hour}
	deleted, archivedUnits, err := retirer.pruneOnce()
	if err != nil {
		t.Fatalf("pruneOnce: %v", err)
	}
	if deleted != 2 {
		t.Fatalf("retired %d ledger rows, want 2 (the two ancient ones with no audit row)", deleted)
	}
	if want := ToUnits(3.00); archivedUnits != want {
		t.Fatalf("archived %d units, want %d", archivedUnits, want)
	}

	// The audited and the recent rows survive.
	for _, billingID := range []string{"bill-ret-old-audited", "bill-ret-recent"} {
		var count int
		if err := r.db.QueryRow(`SELECT COUNT(*) FROM billing_settlements WHERE billing_id = ?`, billingID).Scan(&count); err != nil {
			t.Fatal(err)
		}
		if count != 1 {
			t.Fatalf("ledger row %s was retired but its audit evidence still exists", billingID)
		}
	}
	for _, billingID := range []string{"bill-ret-old-pruned", "bill-ret-old-unlinked"} {
		var count int
		if err := r.db.QueryRow(`SELECT COUNT(*) FROM billing_settlements WHERE billing_id = ?`, billingID).Scan(&count); err != nil {
			t.Fatal(err)
		}
		if count != 0 {
			t.Fatalf("ledger row %s should have been retired", billingID)
		}
	}

	// The money did not vanish: it sits in the per-key archive aggregate.
	var archived int64
	if err := r.db.QueryRow(`SELECT COALESCE(SUM(cost_units), 0) FROM ` + billingLedgerArchivesTable).Scan(&archived); err != nil {
		t.Fatal(err)
	}
	if want := ToUnits(3.00); archived != want {
		t.Fatalf("archive aggregate = %d units, want %d", archived, want)
	}
	var archivedForKey int64
	if err := r.db.QueryRow(`SELECT COALESCE(SUM(cost_units), 0) FROM `+billingLedgerArchivesTable+` WHERE api_key = ?`, key.Key).Scan(&archivedForKey); err != nil {
		t.Fatal(err)
	}
	if want := ToUnits(3.00); archivedForKey != want {
		t.Fatalf("archive aggregate for the key = %d units, want %d", archivedForKey, want)
	}

	// The monthly audit key records the retired amount.
	monthKey := billingLedgerArchiveKeyPrefix + ancient.Format("2006-01")
	raw := r.GetSetting(monthKey, "")
	if raw == "" {
		t.Fatalf("monthly archive key %s was not written", monthKey)
	}
	monthUnits, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		t.Fatalf("monthly archive key %s = %q, want integer units", monthKey, raw)
	}
	if want := ToUnits(3.00); monthUnits != want {
		t.Fatalf("monthly archive units = %d, want %d", monthUnits, want)
	}

	// Archiving is neutral for both gauges: settled money is still settled, and the
	// per-key counters still match the ledger plus its archive. The expected lag is
	// exactly the one measured before the rows were retired.
	if lag, err := worker.auditLag(); err != nil || lag != lagBefore {
		t.Fatalf("audit lag after retention = %d (%v), want %d (archiving must not move the gauge)", lag, err, lagBefore)
	}
	if got, want := runAuditLag(t, r), fullAuditLagUnits(t, r); got != want {
		t.Fatalf("audit lag after retention = %d units, full recomputation = %d units", got, want)
	}
	if got, want := runAuditLag(t, r), lagBefore; got != want {
		t.Fatalf("audit lag after retention = %d units, want %d", got, want)
	}
	if drift, keys, err := worker.ledgerDrift(); err != nil || drift != 0 || keys != 0 {
		t.Fatalf("ledger drift after retention = %d units over %d keys (%v), want 0", drift, keys, err)
	}

	// A second pass is a no-op: the policy must be idempotent.
	deleted, archivedUnits, err = retirer.pruneOnce()
	if err != nil {
		t.Fatalf("second pruneOnce: %v", err)
	}
	if deleted != 0 || archivedUnits != 0 {
		t.Fatalf("second pass retired %d rows / %d units, want a no-op", deleted, archivedUnits)
	}
	var after int64
	if err := r.db.QueryRow(`SELECT COALESCE(SUM(cost_units), 0) FROM ` + billingLedgerArchivesTable).Scan(&after); err != nil {
		t.Fatal(err)
	}
	if after != archived {
		t.Fatalf("archive aggregate changed on the second pass: %d -> %d", archived, after)
	}
}

// TestBillingLedgerRetentionLegacyBillingIDs covers the pre-billing_id shape:
// migration 23 keys those ledger rows as "legacy-<usage_logs.id>", so the audit
// row must be looked up by primary key, not by billing_id.
func TestBillingLedgerRetentionLegacyBillingIDs(t *testing.T) {
	r, key, u := billingIDTestRepo(t)
	ctx := context.Background()

	old := time.Now().UTC().AddDate(0, 0, -400)
	oldValue := old.Format(sqliteTimestampLayout)

	// A live legacy audit row: usage_logs.billing_id is NULL, its id is the ledger
	// key suffix.
	live := billingRecord(key, u.Username, "tr-legacy-live", "", 1.00)
	live.CreatedAt = old
	if err := r.insertUsageLogsContext(ctx, []*UsageLogRecord{live}); err != nil {
		t.Fatal(err)
	}
	var liveID int64
	if err := r.db.QueryRow(`SELECT id FROM usage_logs WHERE trace_id = 'tr-legacy-live'`).Scan(&liveID); err != nil {
		t.Fatal(err)
	}
	if _, err := r.db.Exec(`UPDATE usage_logs SET billing_id = NULL WHERE id = ?`, liveID); err != nil {
		t.Fatal(err)
	}

	// Two legacy ledger rows: one with a surviving audit row, one orphaned.
	for _, tc := range []struct {
		billingID string
		units     int64
	}{
		{fmt.Sprintf("legacy-%d", liveID), ToUnits(1.00)},
		{"legacy-999999", ToUnits(2.00)},
	} {
		if _, err := r.db.Exec(`INSERT INTO billing_settlements (billing_id, api_key, tenant_id, cost_units, total_tokens, created_at)
			VALUES (?, ?, ?, ?, 0, ?)`, tc.billingID, key.Key, u.Username, tc.units, oldValue); err != nil {
			t.Fatalf("insert legacy ledger row %s: %v", tc.billingID, err)
		}
	}

	retirer := &BillingLedgerRetention{repo: r, retention: 100 * 24 * time.Hour}
	deleted, archivedUnits, err := retirer.pruneOnce()
	if err != nil {
		t.Fatalf("pruneOnce: %v", err)
	}
	if deleted != 1 {
		t.Fatalf("retired %d legacy rows, want 1 (the orphaned one)", deleted)
	}
	if want := ToUnits(2.00); archivedUnits != want {
		t.Fatalf("archived %d units, want %d", archivedUnits, want)
	}

	var linked int
	if err := r.db.QueryRow(`SELECT COUNT(*) FROM billing_settlements WHERE billing_id = ?`,
		fmt.Sprintf("legacy-%d", liveID)).Scan(&linked); err != nil {
		t.Fatal(err)
	}
	if linked != 1 {
		t.Fatal("the legacy ledger row whose audit row still exists was retired")
	}
}

// TestBillingLedgerRetentionPruneIsBatched checks the batching contract: a backlog
// larger than one batch is drained over several transactions without losing or
// double-counting anything.
func TestBillingLedgerRetentionPruneIsBatched(t *testing.T) {
	r, key, u := billingIDTestRepo(t)

	old := time.Now().UTC().AddDate(0, 0, -400)
	const rows = billingLedgerPruneBatchSize + 3
	want := int64(0)
	for i := 0; i < rows; i++ {
		billingID := fmt.Sprintf("bill-batch-%d", i)
		if _, err := r.db.Exec(`INSERT INTO billing_settlements (billing_id, api_key, tenant_id, cost_units, total_tokens, created_at)
			VALUES (?, ?, ?, ?, 0, ?)`, billingID, key.Key, u.Username, 1000, old.Format(sqliteTimestampLayout)); err != nil {
			t.Fatalf("insert %s: %v", billingID, err)
		}
		want += 1000
	}

	retirer := &BillingLedgerRetention{repo: r, retention: 100 * 24 * time.Hour}
	deleted, archivedUnits, err := retirer.pruneOnce()
	if err != nil {
		t.Fatalf("pruneOnce: %v", err)
	}
	if deleted != rows {
		t.Fatalf("retired %d rows, want %d", deleted, rows)
	}
	if archivedUnits != want {
		t.Fatalf("archived %d units, want %d", archivedUnits, want)
	}
	var left int
	if err := r.db.QueryRow(`SELECT COUNT(*) FROM billing_settlements`).Scan(&left); err != nil {
		t.Fatal(err)
	}
	if left != 0 {
		t.Fatalf("%d ledger rows left after a full drain", left)
	}
	var archived int64
	if err := r.db.QueryRow(`SELECT COALESCE(SUM(cost_units), 0) FROM ` + billingLedgerArchivesTable).Scan(&archived); err != nil {
		t.Fatal(err)
	}
	if archived != want {
		t.Fatalf("archive aggregate = %d units, want %d", archived, want)
	}
}
