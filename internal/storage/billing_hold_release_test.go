package storage

import (
	"context"
	"fmt"
	"testing"
	"time"
)

func holdCount(t *testing.T, r *Repository, id string) int {
	t.Helper()
	var n int
	if err := r.db.QueryRow(`SELECT COUNT(*) FROM billing_reservations WHERE id = ?`, id).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func saveJobState(t *testing.T, r *Repository, owner, id, holdID string, expires time.Time) {
	t.Helper()
	err := r.SaveNativeState(context.Background(), NativeState{
		Namespace: "gemini-batch", ID: id, Owner: owner, Identity: "up-" + id, Model: "m",
		Billing: []byte(fmt.Sprintf(`{"_airoute_reservation_id":%q}`, holdID)), ExpiresAt: expires,
	})
	if err != nil {
		t.Fatal(err)
	}
}

func pendingHold(t *testing.T, r *Repository, key string) *BillingReservation {
	t.Helper()
	hold, err := r.ReserveBilling(context.Background(), key, ToUnits(0.05), 100)
	if err != nil {
		t.Fatal(err)
	}
	if err := r.PendBilling(context.Background(), hold.ID, hold.ID, hold.AmountUnits); err != nil {
		t.Fatal(err)
	}
	return hold
}

// An asynchronous job that expires while still running is pruned without ever
// reaching settlement; its pending hold must be released, not orphaned forever.
func TestPruneNativeStatesReleasesExpiredHold(t *testing.T) {
	r, key, u := billingIDTestRepo(t)
	hold := pendingHold(t, r, key.Key)
	saveJobState(t, r, u.Username, "job-expired", hold.ID, time.Now().Add(-time.Hour))
	if holdCount(t, r, hold.ID) != 1 {
		t.Fatal("hold should exist before prune")
	}
	if err := r.PruneNativeStates(context.Background()); err != nil {
		t.Fatal(err)
	}
	if holdCount(t, r, hold.ID) != 0 {
		t.Fatal("expired job's hold was orphaned by prune")
	}
	if _, err := r.LoadNativeState(context.Background(), "gemini-batch", "job-expired", u.Username); err == nil {
		t.Fatal("expired state should have been pruned")
	}
}

// Deleting a still-running job (client DELETE) abandons it; the hold must go too.
func TestDeleteNativeStateReleasesHold(t *testing.T) {
	r, key, u := billingIDTestRepo(t)
	hold := pendingHold(t, r, key.Key)
	saveJobState(t, r, u.Username, "job-deleted", hold.ID, time.Now().Add(time.Hour))
	if err := r.DeleteNativeState(context.Background(), "gemini-batch", "job-deleted", u.Username); err != nil {
		t.Fatal(err)
	}
	if holdCount(t, r, hold.ID) != 0 {
		t.Fatal("deleted job's hold was orphaned")
	}
}

// The reaper is a safety net for orphaned native holds (e.g. a crash between Hold
// and SaveNativeState). It must never free a synchronous charge (which always has
// a journal row) nor a running native job (which still has its state).
func TestReapOrphanBillingHoldsSparesLiveCharges(t *testing.T) {
	r, key, u := billingIDTestRepo(t)
	ctx := context.Background()

	orphan := pendingHold(t, r, key.Key) // no state, no journal

	sync := pendingHold(t, r, key.Key) // synchronous charge: journal exists
	rec := billingRecord(key, u.Username, "tr-sync", sync.ID, 0.05)
	rec.ReservationID = sync.ID
	if err := r.PendBillingRecord(ctx, rec); err != nil {
		t.Fatal(err)
	}

	running := pendingHold(t, r, key.Key) // live native job: state exists
	saveJobState(t, r, u.Username, "job-running", running.ID, time.Now().Add(time.Hour))

	reaped, err := r.ReapOrphanBillingHolds(ctx, -time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if reaped != 1 {
		t.Fatalf("reaped %d holds, want only the orphan (1)", reaped)
	}
	if holdCount(t, r, orphan.ID) != 0 {
		t.Fatal("orphan hold should have been reaped")
	}
	if holdCount(t, r, sync.ID) != 1 {
		t.Fatal("synchronous charge hold must not be reaped (journal exists)")
	}
	if holdCount(t, r, running.ID) != 1 {
		t.Fatal("running native job hold must not be reaped (state exists)")
	}
}