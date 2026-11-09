package storage

import (
	"context"
	"testing"

	"github.com/ifnodoraemon/airoute/internal/telemetry"
)

func overdraftTestRepo(t *testing.T, balance float64) (*Repository, *APIKeyRecord, *UserRecord) {
	t.Helper()
	r := accountWalletRepo(t)
	u := walletTestUser(t, r, "overdraft-user", "")
	if err := r.SetUserBalance(u.Username, balance); err != nil {
		t.Fatal(err)
	}
	key := &APIKeyRecord{Key: "sk-overdraft", UserID: u.ID, TenantID: u.Username}
	if err := r.CreateAPIKey(key); err != nil {
		t.Fatal(err)
	}
	return r, key, u
}

// Provider-reported overruns remain real debt; admission blocks further spending.
func TestWalletDeductionRecordsFullActualCost(t *testing.T) {
	r, key, u := overdraftTestRepo(t, 0.01)
	ctx := context.Background()

	before := telemetry.GlobalMetrics.BillingOverdrafts.Load()
	// Two charges of 3 CNY on the same key: the batch aggregates them into one
	// deduction, which must preserve all six yuan of delivered usage.
	records := []*UsageLogRecord{
		billingRecord(key, u.Username, "tr-od-1", "bill-od-1", 3.0),
		billingRecord(key, u.Username, "tr-od-2", "bill-od-2", 3.0),
	}
	if err := r.BatchRecordUsageLogsContext(ctx, records); err != nil {
		t.Fatalf("batch write: %v", err)
	}

	user, err := r.GetUserByID(u.ID)
	if err != nil {
		t.Fatal(err)
	}
	if want := ToUnits(-5.99); user.BalanceUnits != want {
		t.Fatalf("balance_units = %d, want %d after full deduction", user.BalanceUnits, want)
	}
	if want := -5.99; user.Balance < want-1e-9 || user.Balance > want+1e-9 {
		t.Fatalf("balance = %v, want the float mirror to match %v", user.Balance, want)
	}
	if telemetry.GlobalMetrics.BillingOverdrafts.Load() <= before {
		t.Fatal("overdraft was not counted on /metrics")
	}
}

// A charge that crosses zero records the exact debt and counts an overdraft.
func TestWalletDeductionRecordsSmallOverdraft(t *testing.T) {
	r, key, u := overdraftTestRepo(t, 0.10)
	ctx := context.Background()

	before := telemetry.GlobalMetrics.BillingOverdrafts.Load()
	if err := r.BatchRecordUsageLogsContext(ctx, []*UsageLogRecord{
		billingRecord(key, u.Username, "tr-small", "bill-small", 0.25),
	}); err != nil {
		t.Fatalf("write: %v", err)
	}

	user, _ := r.GetUserByID(u.ID)
	if want := -ToUnits(0.15); user.BalanceUnits != want {
		t.Fatalf("balance_units = %d, want %d (0.10 - 0.25)", user.BalanceUnits, want)
	}
	if telemetry.GlobalMetrics.BillingOverdrafts.Load() <= before {
		t.Fatal("negative balance was not counted on /metrics")
	}
}

// Completed work is never forgiven even when it exhausts the wallet.
func TestWalletDeductionNeverForgivesDeliveredUsage(t *testing.T) {
	r, key, u := overdraftTestRepo(t, 0.10)
	ctx := context.Background()

	if err := r.BatchRecordUsageLogsContext(ctx, []*UsageLogRecord{
		billingRecord(key, u.Username, "tr-zero", "bill-zero", 5.0),
	}); err != nil {
		t.Fatalf("write: %v", err)
	}

	user, _ := r.GetUserByID(u.ID)
	if user.BalanceUnits != ToUnits(-4.9) {
		t.Fatalf("balance_units = %d, want %d", user.BalanceUnits, ToUnits(-4.9))
	}
	if user.Balance != -4.9 {
		t.Fatalf("balance = %v, want -4.9", user.Balance)
	}
}

// The tenant/username fallback preserves the same full cost.
func TestTenantFallbackDeductionRecordsFullActualCost(t *testing.T) {
	r, _, u := overdraftTestRepo(t, 0.01)
	ctx := context.Background()

	if err := r.BatchRecordUsageLogsContext(ctx, []*UsageLogRecord{
		billingRecord(&APIKeyRecord{Key: ""}, u.Username, "tr-tenant-clamp", "bill-tenant-clamp", 8.0),
	}); err != nil {
		t.Fatalf("write: %v", err)
	}
	user, _ := r.GetUserByID(u.ID)
	if want := ToUnits(-7.99); user.BalanceUnits != want {
		t.Fatalf("tenant fallback balance_units = %d, want %d", user.BalanceUnits, want)
	}
}
