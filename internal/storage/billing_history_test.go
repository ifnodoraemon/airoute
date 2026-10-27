package storage

import (
	"testing"
	"time"
)

func TestOnboardingRollsBackAccountWhenKeyCreationFails(t *testing.T) {
	db, err := OpenDB(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	r := NewRepository(db)
	if err := r.CreateAPIKey(&APIKeyRecord{Key: "sk-existing"}); err != nil {
		t.Fatal(err)
	}
	u := &UserRecord{Username: "new-account", PasswordHash: "synthetic", Balance: 5}
	if err := r.CreateUserWithAPIKey(u, &APIKeyRecord{Key: "sk-existing"}); err == nil {
		t.Fatal("expected duplicate-key failure")
	}
	if u, err := r.GetUserByUsername("new-account"); err == nil || u != nil {
		t.Fatal("failed onboarding left a partial account")
	}
}

func TestPricingSnapshotsSurvivePriceChangesAndBatchWrites(t *testing.T) {
	db, err := OpenDB(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	r := NewRepository(db)
	snapshot := &UsagePricingSnapshot{Model: "images", Currency: "CNY", FixedPrice: 10, GroupMultiplier: .8, OffPeakDiscount: 1, BillableUnits: 2}
	for _, batch := range []bool{false, true} {
		logs := []*UsageLogRecord{{Model: "images", Cost: 16, PricingSnapshot: snapshot, StatusCode: 200}, {Model: "images", Cost: 16, PricingSnapshot: snapshot, StatusCode: 200, CreatedAt: time.Now()}}
		if batch {
			err = r.BatchRecordUsageLogs(logs)
		} else {
			for _, l := range logs {
				if err = r.RecordUsageLog(l); err != nil {
					break
				}
			}
		}
		if err != nil {
			t.Fatal(err)
		}
	}
	if err := r.SaveModelPrice(&ModelPriceRecord{Model: "images", FixedPrice: 99}); err != nil {
		t.Fatal(err)
	}
	logs, err := r.ListUsageLogs(50, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(logs) != 4 {
		t.Fatalf("lost records: %d", len(logs))
	}
	for _, l := range logs {
		if l.PricingSnapshot == nil || *l.PricingSnapshot != *snapshot || l.Cost != 16 {
			t.Fatalf("historical charge changed: %+v", l)
		}
	}
}
