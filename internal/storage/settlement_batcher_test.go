package storage

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/ifnodoraemon/airoute/internal/telemetry"
)

// swapGlobalSettlementBatcher installs a batcher for one test and restores the
// previous global on cleanup.
func swapGlobalSettlementBatcher(t *testing.T, b *SettlementBatcher) {
	t.Helper()
	prev := globalSettlementBatcher
	globalSettlementBatcher = b
	t.Cleanup(func() { globalSettlementBatcher = prev })
}

// TestSettlementBatcherMergesConcurrentCharges is the core group-commit test:
// many concurrent charges must settle exactly once each, while the number of
// database transactions is far below the number of charges.
func TestSettlementBatcherMergesConcurrentCharges(t *testing.T) {
	r, key, u := settlementTestRepo(t)
	prevRepo := settlementRepo
	settlementRepo = r
	t.Cleanup(func() { settlementRepo = prevRepo })

	batcher := InitSettlementBatcher(r)
	swapGlobalSettlementBatcher(t, batcher)
	t.Cleanup(batcher.Stop)

	const (
		workers        = 50
		perWorker      = 10
		total          = workers * perWorker
		costPerCharge  = 0.01
		initialBalance = 10.0
	)
	batchesBefore := telemetry.GlobalMetrics.SettlementBatches.Load()

	var wg sync.WaitGroup
	errCh := make(chan error, total)
	start := make(chan struct{})
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func(worker int) {
			defer wg.Done()
			<-start
			for i := 0; i < perWorker; i++ {
				rec := billingRecord(key, u.Username,
					"tr-batch", NewBillingID(), costPerCharge)
				ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
				err := SettleChargeBatched(ctx, rec)
				cancel()
				if err != nil {
					errCh <- err
				}
			}
		}(w)
	}
	close(start)
	wg.Wait()
	close(errCh)
	for err := range errCh {
		t.Fatalf("settle via batcher: %v", err)
	}

	var ledger int
	if err := r.db.QueryRow(`SELECT COUNT(*) FROM billing_settlements`).Scan(&ledger); err != nil {
		t.Fatal(err)
	}
	if ledger != total {
		t.Fatalf("billing_settlements rows = %d, want %d", ledger, total)
	}

	wantUnits := ToUnits(initialBalance - total*costPerCharge)
	user, err := r.GetUserByID(u.ID)
	if err != nil {
		t.Fatal(err)
	}
	if user.BalanceUnits != wantUnits {
		t.Fatalf("wallet units = %d, want %d (every charge must apply exactly once)", user.BalanceUnits, wantUnits)
	}

	gotKey, err := r.GetAPIKeyByKey(key.Key)
	if err != nil {
		t.Fatal(err)
	}
	if want := ToUnits(total * costPerCharge); gotKey.UsedCostUnits != want {
		t.Fatalf("key used units = %d, want %d", gotKey.UsedCostUnits, want)
	}

	batches := telemetry.GlobalMetrics.SettlementBatches.Load() - batchesBefore
	if batches == 0 {
		t.Fatal("no settlement batch recorded")
	}
	if batches >= total {
		t.Fatalf("batches = %d for %d charges: group commit did not merge anything", batches, total)
	}
}

// A redelivered record (same billing_id) must not charge twice through the
// batcher, exactly like the synchronous path.
func TestSettlementBatcherIsIdempotentPerBillingID(t *testing.T) {
	r, key, u := settlementTestRepo(t)
	prevRepo := settlementRepo
	settlementRepo = r
	t.Cleanup(func() { settlementRepo = prevRepo })

	batcher := InitSettlementBatcher(r)
	swapGlobalSettlementBatcher(t, batcher)
	t.Cleanup(batcher.Stop)

	rec := billingRecord(key, u.Username, "tr-batch-dup", "bill-batch-dup", 1.25)
	for i := 0; i < 3; i++ {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		if err := SettleChargeBatched(ctx, rec); err != nil {
			cancel()
			t.Fatalf("settle attempt %d: %v", i+1, err)
		}
		cancel()
	}

	user, err := r.GetUserByID(u.ID)
	if err != nil {
		t.Fatal(err)
	}
	if want := ToUnits(10.00 - 1.25); user.BalanceUnits != want {
		t.Fatalf("wallet units = %d, want %d (redelivery must be free)", user.BalanceUnits, want)
	}
	var ledger int
	if err := r.db.QueryRow(`SELECT COUNT(*) FROM billing_settlements`).Scan(&ledger); err != nil {
		t.Fatal(err)
	}
	if ledger != 1 {
		t.Fatalf("billing_settlements rows = %d, want 1", ledger)
	}
}

// Without a batcher (tests, CLI, shutdown) the call must degrade to the direct
// synchronous settlement instead of silently dropping the charge.
func TestSettleChargeBatchedFallsBackWithoutBatcher(t *testing.T) {
	r, key, u := settlementTestRepo(t)
	prevRepo := settlementRepo
	prevBatcher := globalSettlementBatcher
	settlementRepo = r
	globalSettlementBatcher = nil
	t.Cleanup(func() {
		settlementRepo = prevRepo
		globalSettlementBatcher = prevBatcher
	})

	rec := billingRecord(key, u.Username, "tr-fallback", "bill-fallback", 0.60)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := SettleChargeBatched(ctx, rec); err != nil {
		t.Fatalf("fallback settle: %v", err)
	}
	if !rec.Settled {
		t.Fatal("record not marked settled by the direct path")
	}
	user, err := r.GetUserByID(u.ID)
	if err != nil {
		t.Fatal(err)
	}
	if want := ToUnits(10.00 - 0.60); user.BalanceUnits != want {
		t.Fatalf("wallet units = %d, want %d", user.BalanceUnits, want)
	}
}

// After Stop the batcher must not accept work it cannot flush; the fallback to
// direct settlement keeps the money path working (this is what shutdown looks
// like to an in-flight request).
func TestSettlementBatcherStopDegradesToDirectPath(t *testing.T) {
	r, key, u := settlementTestRepo(t)
	prevRepo := settlementRepo
	settlementRepo = r
	t.Cleanup(func() { settlementRepo = prevRepo })

	batcher := InitSettlementBatcher(r)
	swapGlobalSettlementBatcher(t, batcher)
	batcher.Stop()

	rec := billingRecord(key, u.Username, "tr-stopped", "bill-stopped", 0.25)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := SettleChargeBatched(ctx, rec); err != nil {
		t.Fatalf("settle after Stop: %v", err)
	}
	user, err := r.GetUserByID(u.ID)
	if err != nil {
		t.Fatal(err)
	}
	if want := ToUnits(10.00 - 0.25); user.BalanceUnits != want {
		t.Fatalf("wallet units = %d, want %d", user.BalanceUnits, want)
	}
}

// A caller whose wait expires still gets the charge applied later by the flush;
// the ledger key prevents the async pipeline from charging it a second time.
func TestSettlementBatcherTimeoutStillSettlesLater(t *testing.T) {
	r, key, u := settlementTestRepo(t)
	prevRepo := settlementRepo
	settlementRepo = r
	t.Cleanup(func() { settlementRepo = prevRepo })

	batcher := InitSettlementBatcher(r)
	swapGlobalSettlementBatcher(t, batcher)
	t.Cleanup(batcher.Stop)

	rec := billingRecord(key, u.Username, "tr-timeout", NewBillingID(), 0.30)
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // an already-expired wait must not lose the charge
	err := SettleChargeBatched(ctx, rec)
	if err == nil {
		t.Log("charge committed before the expired context was observed; acceptable")
	}

	deadline := time.Now().Add(5 * time.Second)
	var ledger int
	for time.Now().Before(deadline) {
		if qerr := r.db.QueryRow(`SELECT COUNT(*) FROM billing_settlements`).Scan(&ledger); qerr != nil {
			t.Fatal(qerr)
		}
		if ledger > 0 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if ledger == 0 {
		t.Fatal("charge was neither committed nor left for the pipeline after a wait timeout")
	}
}
