package storage

import (
	"context"
	"testing"
)

// BUG-DATA-01 regression: a settlement batch that carries the same billing_id
// twice must move the money once. The ledger insert has always deduped on the
// unique billing_id; the money aggregation used to walk the raw input, so a batch
// carrying the same record twice debited the wallet and the key counters a second
// time while the ledger recorded a single charge.
func TestSettlementBatchDuplicateBillingIDChargesOnce(t *testing.T) {
	r, key, u := billingIDTestRepo(t)
	ctx := context.Background()

	startUnits := ToUnits(10.0) // walletTestUser creates a 10 CNY wallet
	const (
		billingID = "bill-dup-in-one-batch"
		cost      = 2.0
		tokens    = 1_000_000
	)

	logs := []*UsageLogRecord{
		billingRecord(key, u.Username, "tr-dup-a", billingID, cost),
		billingRecord(key, u.Username, "tr-dup-b", billingID, cost),
	}
	if err := r.BatchRecordUsageLogsContext(ctx, logs); err != nil {
		t.Fatalf("batch delivery: %v", err)
	}

	// Both copies of the record are considered settled after the commit.
	for _, log := range logs {
		if !log.Settled {
			t.Fatalf("record %s (billing_id %s) not marked settled", log.TraceID, log.BillingID)
		}
	}

	var ledgerRows int
	if err := r.db.QueryRow(`SELECT COUNT(*) FROM billing_settlements WHERE billing_id = ?`, billingID).Scan(&ledgerRows); err != nil {
		t.Fatalf("count ledger rows: %v", err)
	}
	if ledgerRows != 1 {
		t.Fatalf("ledger rows for one billing_id = %d, want 1", ledgerRows)
	}

	user, err := r.GetUserByID(u.ID)
	if err != nil {
		t.Fatalf("reload user: %v", err)
	}
	if want := startUnits - ToUnits(cost); user.BalanceUnits != want {
		t.Fatalf("wallet debited for a duplicated delivery: balance_units=%d, want %d (one charge)", user.BalanceUnits, want)
	}

	var usedCostUnits, usedTokens int64
	if err := r.db.QueryRow(`SELECT COALESCE(used_cost_units, 0), COALESCE(used_tokens, 0) FROM api_keys WHERE key = ?`, key.Key).
		Scan(&usedCostUnits, &usedTokens); err != nil {
		t.Fatalf("read key counters: %v", err)
	}
	if want := ToUnits(cost); usedCostUnits != want {
		t.Fatalf("used_cost_units=%d, want %d (one charge)", usedCostUnits, want)
	}
	if want := int64(tokens); usedTokens != want {
		t.Fatalf("used_tokens=%d, want %d (one charge)", usedTokens, want)
	}

	// The two mirrors must agree: this is what BillingReconciler.ledgerDrift checks.
	var ledgerUnits int64
	if err := r.db.QueryRow(`SELECT COALESCE(SUM(cost_units), 0) FROM billing_settlements WHERE billing_id = ?`, billingID).
		Scan(&ledgerUnits); err != nil {
		t.Fatalf("sum ledger units: %v", err)
	}
	if usedCostUnits != ledgerUnits {
		t.Fatalf("key counter (%d) and ledger (%d) disagree after an in-batch duplicate", usedCostUnits, ledgerUnits)
	}
}

// BUG-DATA-01 combined case: an in-batch duplicate followed by a cross-batch
// redelivery of the same charge, plus a genuinely different charge. Exactly two
// charges must be applied, no matter how many deliveries each one took.
func TestSettlementBatchDuplicateAndRedeliveryChargesOnce(t *testing.T) {
	r, key, u := billingIDTestRepo(t)
	ctx := context.Background()

	startUnits := ToUnits(10.0)
	const (
		repeatedID = "bill-repeat-across-batches"
		freshID    = "bill-distinct-charge"
	)

	// Batch 1: the same charge delivered twice in one batch.
	if err := r.BatchRecordUsageLogsContext(ctx, []*UsageLogRecord{
		billingRecord(key, u.Username, "tr-repeat-1", repeatedID, 2.0),
		billingRecord(key, u.Username, "tr-repeat-2", repeatedID, 2.0),
	}); err != nil {
		t.Fatalf("first batch: %v", err)
	}

	// Batch 2: a redelivery of that charge plus one different charge.
	redelivery := billingRecord(key, u.Username, "tr-repeat-3", repeatedID, 2.0)
	distinct := billingRecord(key, u.Username, "tr-distinct", freshID, 1.0)
	if err := r.BatchRecordUsageLogsContext(ctx, []*UsageLogRecord{redelivery, distinct}); err != nil {
		t.Fatalf("second batch: %v", err)
	}
	if !redelivery.Settled || !distinct.Settled {
		t.Fatalf("second batch records not marked settled: redelivery=%v distinct=%v", redelivery.Settled, distinct.Settled)
	}

	user, err := r.GetUserByID(u.ID)
	if err != nil {
		t.Fatalf("reload user: %v", err)
	}
	if want := startUnits - ToUnits(2.0) - ToUnits(1.0); user.BalanceUnits != want {
		t.Fatalf("balance_units=%d, want %d (one repeat charge + one distinct charge)", user.BalanceUnits, want)
	}

	var ledgerRows int
	if err := r.db.QueryRow(`SELECT COUNT(*) FROM billing_settlements`).Scan(&ledgerRows); err != nil {
		t.Fatalf("count ledger rows: %v", err)
	}
	if ledgerRows != 2 {
		t.Fatalf("ledger rows=%d, want 2 (one per distinct billing_id)", ledgerRows)
	}

	// Both settlement attempts of the repeated charge must have succeeded; a
	// rollback would leave the copies unsettled so the pipeline could retry.
	var settledFlag int
	if err := r.db.QueryRow(`SELECT COUNT(*) FROM billing_settlements WHERE billing_id IN (?, ?)`, repeatedID, freshID).
		Scan(&settledFlag); err != nil {
		t.Fatalf("count distinct ledger ids: %v", err)
	}
	if settledFlag != 2 {
		t.Fatalf("ledger ids=%d, want both charges recorded", settledFlag)
	}
}
