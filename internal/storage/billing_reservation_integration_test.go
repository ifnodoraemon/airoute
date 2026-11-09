//go:build integration

package storage

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"
)

func TestPostgresBillingReservationsAcrossIndependentPools(t *testing.T) {
	db1, err := OpenDB(testPostgresURL(t))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db1.Close() })
	db2, err := OpenDB(testPostgresURL(t))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db2.Close() })
	repos := []*Repository{NewRepository(db1), NewRepository(db2)}
	suffix := fmt.Sprintf("%d", time.Now().UnixNano())
	user := &UserRecord{Username: "reserved-pg-" + suffix, PasswordHash: "synthetic", Role: "user", Status: "active", Balance: 1}
	if err := repos[0].CreateUser(user); err != nil {
		t.Fatal(err)
	}
	keys := []*APIKeyRecord{{Key: "sk-pg-reserved-a-" + suffix, UserID: user.ID, TenantID: user.Username}, {Key: "sk-pg-reserved-b-" + suffix, UserID: user.ID, TenantID: user.Username}}
	for _, key := range keys {
		if err := repos[0].CreateAPIKey(key); err != nil {
			t.Fatal(err)
		}
	}
	holds := make(chan *BillingReservation, 64)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	var wg sync.WaitGroup
	for i := range 64 {
		wg.Go(func() {
			hold, err := repos[i%2].ReserveBilling(ctx, keys[i%2].Key, ToUnits(0.05), 64)
			if err == nil {
				holds <- hold
			} else if !errors.Is(err, ErrWalletReserved) {
				t.Errorf("reserve: %v", err)
			}
		})
	}
	wg.Wait()
	close(holds)
	if len(holds) != 20 {
		t.Fatalf("admitted %d requests across two pools, want 20", len(holds))
	}
	for hold := range holds {
		wg.Go(func() {
			rec := &UsageLogRecord{BillingID: hold.ID, ReservationID: hold.ID, APIKey: hold.APIKey, TenantID: user.Username, Cost: 0.05, TotalTokens: 1}
			if err := repos[1].SettleUsageCharge(ctx, rec); err != nil {
				t.Errorf("settle: %v", err)
				return
			}
			rec.Settled = false
			if err := repos[0].SettleUsageCharge(ctx, rec); err != nil {
				t.Errorf("redeliver: %v", err)
			}
		})
	}
	wg.Wait()
	loaded, err := repos[0].GetUserByID(user.ID)
	if err != nil || loaded.BalanceUnits != 0 {
		t.Fatalf("wallet=%v err=%v", loaded, err)
	}
	var held, spent int64
	if err := db1.QueryRow(`SELECT (SELECT COALESCE(SUM(amount_units),0) FROM billing_reservations WHERE user_id = ?), (SELECT COALESCE(SUM(cost_units),0) FROM billing_settlements WHERE tenant_id = ?)`, user.ID, user.Username).Scan(&held, &spent); err != nil {
		t.Fatal(err)
	}
	if held != 0 || spent != ToUnits(1) {
		t.Fatalf("held=%d spent=%d", held, spent)
	}
	if _, err := repos[0].ReserveBilling(ctx, keys[0].Key, ToUnits(0.05), 64); !errors.Is(err, ErrWalletReserved) {
		t.Fatalf("old balance became spendable: %v", err)
	}
}
