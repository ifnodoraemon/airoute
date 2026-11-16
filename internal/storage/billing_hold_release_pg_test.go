//go:build integration

package storage

import (
	"context"
	"fmt"
	"testing"
	"time"
)

// TestPostgresNativeHoldReleaseAndReaper exercises the hold-release paths on a
// real PostgreSQL: the transactional prune must free an expired job's hold, and
// the reaper's NOT EXISTS / || concatenation must reap only the orphan while
// sparing a synchronous charge (journal row) and a running job (live state).
func TestPostgresNativeHoldReleaseAndReaper(t *testing.T) {
	db, err := OpenDB(testPostgresURL(t))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	r := NewRepository(db)
	suffix := fmt.Sprintf("%d", time.Now().UnixNano())
	user := &UserRecord{Username: "hold-pg-" + suffix, PasswordHash: "synthetic", Role: "user", Status: "active", Balance: 100}
	if err := r.CreateUser(user); err != nil {
		t.Fatal(err)
	}
	key := &APIKeyRecord{Key: "sk-pg-hold-" + suffix, UserID: user.ID, TenantID: user.Username}
	if err := r.CreateAPIKey(key); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	state := func(id, holdID string, expires time.Time) NativeState {
		return NativeState{Namespace: "gemini-batch", ID: id, Owner: user.Username, Identity: "up", Model: "m",
			Billing: []byte(fmt.Sprintf(`{"_airoute_reservation_id":%q}`, holdID)), ExpiresAt: expires}
	}
	pend := func() *BillingReservation {
		hold, err := r.ReserveBilling(ctx, key.Key, ToUnits(0.05), 100)
		if err != nil {
			t.Fatal(err)
		}
		if err := r.PendBilling(ctx, hold.ID, hold.ID, hold.AmountUnits); err != nil {
			t.Fatal(err)
		}
		return hold
	}

	expired := pend()
	if err := r.SaveNativeState(ctx, state("job-"+suffix, expired.ID, time.Now().Add(-time.Hour))); err != nil {
		t.Fatal(err)
	}
	if err := r.PruneNativeStates(ctx); err != nil {
		t.Fatalf("prune on PostgreSQL: %v", err)
	}
	if holdCount(t, r, expired.ID) != 0 {
		t.Fatal("PostgreSQL prune orphaned the expired job's hold")
	}

	orphan := pend()
	sync := pend()
	rec := &UsageLogRecord{TraceID: "tr-" + suffix, APIKey: key.Key, TenantID: user.Username, Model: "m", Cost: 0.05, BillingID: sync.ID, ReservationID: sync.ID, StatusCode: 200, TotalTokens: 1}
	if err := r.PendBillingRecord(ctx, rec); err != nil {
		t.Fatal(err)
	}
	running := pend()
	if err := r.SaveNativeState(ctx, state("run-"+suffix, running.ID, time.Now().Add(time.Hour))); err != nil {
		t.Fatal(err)
	}
	reaped, err := r.ReapOrphanBillingHolds(ctx, -time.Hour)
	if err != nil {
		t.Fatalf("reaper on PostgreSQL: %v", err)
	}
	if reaped != 1 {
		t.Fatalf("PostgreSQL reaper reaped %d holds, want only the orphan (1)", reaped)
	}
	if holdCount(t, r, orphan.ID) != 0 {
		t.Fatal("orphan hold should have been reaped")
	}
	if holdCount(t, r, sync.ID) != 1 || holdCount(t, r, running.ID) != 1 {
		t.Fatal("PostgreSQL reaper freed a live synchronous charge or running job")
	}
}