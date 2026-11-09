package storage

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
)

func reservationTestRepo(t *testing.T, balance, budget float64) (*Repository, *APIKeyRecord, *UserRecord) {
	t.Helper()
	r, key, user := settlementTestRepo(t)
	if err := r.SetUserBalance(user.Username, balance); err != nil {
		t.Fatal(err)
	}
	key.Budget = budget
	if err := r.UpdateAPIKey(key); err != nil {
		t.Fatal(err)
	}
	return r, key, user
}

func reserveForTest(t *testing.T, r *Repository, key *APIKeyRecord, amount float64) *BillingReservation {
	t.Helper()
	hold, err := r.ReserveBilling(context.Background(), key.Key, ToUnits(amount), 64)
	if err != nil || hold == nil {
		t.Fatalf("reserve: hold=%v err=%v", hold, err)
	}
	return hold
}

func TestBillingReservationConcurrentWalletCeiling(t *testing.T) {
	r, key, user := reservationTestRepo(t, 1, 0)
	var admitted atomic.Int64
	var wg sync.WaitGroup
	for range 50 {
		wg.Go(func() {
			_, err := r.ReserveBilling(context.Background(), key.Key, ToUnits(0.05), 64)
			if err == nil {
				admitted.Add(1)
			} else if !errors.Is(err, ErrWalletReserved) {
				t.Errorf("reserve: %v", err)
			}
		})
	}
	wg.Wait()
	if admitted.Load() != 20 {
		t.Fatalf("admitted=%d, want 20", admitted.Load())
	}
	var total int64
	if err := r.db.QueryRow(`SELECT SUM(amount_units) FROM billing_reservations WHERE user_id = ?`, user.ID).Scan(&total); err != nil || total != ToUnits(1) {
		t.Fatalf("held=%d err=%v", total, err)
	}
}

func TestBillingReservationTopUpRequiresFullFunding(t *testing.T) {
	r, key, _ := reservationTestRepo(t, 1, 0)
	hold := reserveForTest(t, r, key, 0.05)
	if err := r.ResizeBilling(context.Background(), hold.ID, ToUnits(50)); !errors.Is(err, ErrWalletReserved) {
		t.Fatalf("unfunded top-up err=%v, want wallet refusal", err)
	}
	var amount int64
	if err := r.db.QueryRow(`SELECT amount_units FROM billing_reservations WHERE id = ?`, hold.ID).Scan(&amount); err != nil || amount != ToUnits(0.05) {
		t.Fatalf("partial top-up was committed: amount=%d err=%v", amount, err)
	}
}

func TestBillingReservationKeyBudgetIncludesPendingCharges(t *testing.T) {
	r, key, _ := reservationTestRepo(t, 100, 0.10)
	a := reserveForTest(t, r, key, 0.05)
	_ = reserveForTest(t, r, key, 0.05)
	if _, err := r.ReserveBilling(context.Background(), key.Key, ToUnits(0.05), 64); !errors.Is(err, ErrKeyBudgetReserved) {
		t.Fatalf("pending budget bypassed: %v", err)
	}
	if err := r.ResizeBilling(context.Background(), a.ID, ToUnits(0.06)); !errors.Is(err, ErrKeyBudgetReserved) {
		t.Fatalf("top-up bypassed key budget: %v", err)
	}
}

func TestBillingReservationOverrunNeverReleasesOtherRequest(t *testing.T) {
	r, key, user := reservationTestRepo(t, 1, 0)
	a := reserveForTest(t, r, key, 0.05)
	b := reserveForTest(t, r, key, 0.05)
	ctx := context.Background()
	rec := billingRecord(key, user.Username, "overrun", "overrun", 0.20)
	rec.ReservationID = a.ID
	if err := r.PendBilling(ctx, a.ID, rec.BillingID, ToUnits(rec.Cost)); err != nil {
		t.Fatal(err)
	}
	if err := r.SettleUsageCharge(ctx, rec); err != nil {
		t.Fatal(err)
	}
	_ = r.CancelBilling(ctx, a.ID)
	var amount int64
	if err := r.db.QueryRow(`SELECT amount_units FROM billing_reservations WHERE id = ?`, b.ID).Scan(&amount); err != nil || amount != ToUnits(0.05) {
		t.Fatalf("B's reserve changed: amount=%d err=%v", amount, err)
	}
	u, _ := r.GetUserByID(user.ID)
	if u.BalanceUnits != ToUnits(0.8) {
		t.Fatalf("balance=%d", u.BalanceUnits)
	}
}

func TestBillingReservationPendingSurvivesTimeoutAndRepositoryRestart(t *testing.T) {
	r, key, user := reservationTestRepo(t, 0.05, 0)
	ctx := context.Background()
	hold := reserveForTest(t, r, key, 0.05)
	if err := r.PendBilling(ctx, hold.ID, "deferred", ToUnits(0.05)); err != nil {
		t.Fatal(err)
	}
	if err := r.CancelBilling(ctx, hold.ID); err != nil {
		t.Fatal(err)
	}
	otherNode := NewRepository(r.db)
	if _, err := otherNode.ReserveBilling(ctx, key.Key, ToUnits(0.05), 64); !errors.Is(err, ErrWalletReserved) {
		t.Fatalf("reused pending balance: %v", err)
	}
	rec := billingRecord(key, user.Username, "deferred", "deferred", 0.05)
	rec.ReservationID = hold.ID
	if err := otherNode.SettleUsageCharge(ctx, rec); err != nil {
		t.Fatal(err)
	}
	if _, err := r.ReserveBilling(ctx, key.Key, ToUnits(0.05), 64); !errors.Is(err, ErrWalletReserved) {
		t.Fatalf("reused old wallet snapshot after commit: %v", err)
	}
	if err := r.UpdateUserBalance(user.Username, 0.05); err != nil {
		t.Fatal(err)
	}
	fresh := reserveForTest(t, r, key, 0.05)
	_ = r.CancelBilling(ctx, hold.ID)
	var count int
	if err := r.db.QueryRow(`SELECT COUNT(*) FROM billing_reservations WHERE id = ?`, fresh.ID).Scan(&count); err != nil || count != 1 {
		t.Fatalf("old cancellation erased a new request: count=%d err=%v", count, err)
	}
}

func TestBillingReservationRollbackRetainsHold(t *testing.T) {
	r, key, user := reservationTestRepo(t, 1, 0)
	hold := reserveForTest(t, r, key, 0.10)
	rec := billingRecord(key, user.Username, "rollback", "rollback", 0.10)
	rec.ReservationID = hold.ID
	if _, err := r.db.Exec(`CREATE TRIGGER fail_reserved_debit BEFORE UPDATE ON users BEGIN SELECT RAISE(ABORT, 'synthetic debit failure'); END`); err != nil {
		t.Fatal(err)
	}
	if err := r.SettleUsageCharge(context.Background(), rec); err == nil {
		t.Fatal("expected debit failure")
	}
	var count int
	if err := r.db.QueryRow(`SELECT COUNT(*) FROM billing_reservations WHERE id = ?`, hold.ID).Scan(&count); err != nil || count != 1 {
		t.Fatalf("rollback lost the hold: count=%d err=%v", count, err)
	}
	if _, err := r.db.Exec(`DROP TRIGGER fail_reserved_debit`); err != nil {
		t.Fatal(err)
	}
	if err := r.SettleUsageCharge(context.Background(), rec); err != nil {
		t.Fatal(err)
	}
	u, _ := r.GetUserByID(user.ID)
	if u.BalanceUnits != ToUnits(0.9) {
		t.Fatalf("wallet=%d", u.BalanceUnits)
	}
}

func TestBillingJournalRecoversChargeAndAuditAfterQueueLoss(t *testing.T) {
	r, key, user := reservationTestRepo(t, 1, 0)
	hold := reserveForTest(t, r, key, 0.10)
	rec := billingRecord(key, user.Username, "journal", "journal", 0.10)
	rec.ReservationID = hold.ID
	if err := r.PendBillingRecord(context.Background(), rec); err != nil {
		t.Fatal(err)
	}
	// No Redis stream, memory queue, or original process remains available.
	otherNode := NewRepository(r.db)
	for range 2 {
		if err := otherNode.recoverBillingJournal(context.Background()); err != nil {
			t.Fatal(err)
		}
	}
	u, _ := r.GetUserByID(user.ID)
	if u.BalanceUnits != ToUnits(0.9) {
		t.Fatalf("wallet=%d, want one charge", u.BalanceUnits)
	}
	var audit, pending, holds int
	if err := r.db.QueryRow(`SELECT (SELECT COUNT(*) FROM usage_logs WHERE billing_id = 'journal'), (SELECT COUNT(*) FROM billing_usage_journal), (SELECT COUNT(*) FROM billing_reservations)`).Scan(&audit, &pending, &holds); err != nil {
		t.Fatal(err)
	}
	if audit != 1 || pending != 0 || holds != 0 {
		t.Fatalf("audit=%d pending=%d holds=%d", audit, pending, holds)
	}
}
