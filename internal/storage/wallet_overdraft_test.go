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

// A single expensive charge (or a burst aggregated in one flush) used to drive a
// wallet arbitrarily negative with no floor and no signal.
func TestWalletDeductionClampsAtOverdraftFloor(t *testing.T) {
	r, key, u := overdraftTestRepo(t, 0.01)
	ctx := context.Background()

	before := telemetry.GlobalMetrics.BillingOverdrafts.Load()
	// Two charges of 3 CNY on the same key: the batch aggregates them into one
	// deduction, which must stop at the default -1 CNY floor.
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
	if want := -ToUnits(defaultMaxOverdraftCNY); user.BalanceUnits != want {
		t.Fatalf("balance_units = %d, want the %d floor (clamped)", user.BalanceUnits, want)
	}
	if want := -defaultMaxOverdraftCNY; user.Balance < want-1e-9 || user.Balance > want+1e-9 {
		t.Fatalf("balance = %v, want the float mirror to match the floor %v", user.Balance, want)
	}
	if telemetry.GlobalMetrics.BillingOverdrafts.Load() <= before {
		t.Fatal("clamped overdraft was not counted on /metrics")
	}
}

// A charge that merely crosses zero is a real overdraft (charge it, count it) but
// must not be pushed down to the floor.
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

// GATEWAY_MAX_OVERDRAFT_CNY=0 means "never below zero".
func TestWalletDeductionHonoursZeroOverdraftSetting(t *testing.T) {
	t.Setenv("GATEWAY_MAX_OVERDRAFT_CNY", "0")
	r, key, u := overdraftTestRepo(t, 0.10)
	ctx := context.Background()

	if err := r.BatchRecordUsageLogsContext(ctx, []*UsageLogRecord{
		billingRecord(key, u.Username, "tr-zero", "bill-zero", 5.0),
	}); err != nil {
		t.Fatalf("write: %v", err)
	}

	user, _ := r.GetUserByID(u.ID)
	if user.BalanceUnits != 0 {
		t.Fatalf("balance_units = %d, want 0 with overdraft disabled", user.BalanceUnits)
	}
	if user.Balance != 0 {
		t.Fatalf("balance = %v, want 0 with overdraft disabled", user.Balance)
	}
}

// The tenant/username fallback path uses the same clamp.
func TestTenantFallbackDeductionClamps(t *testing.T) {
	r, _, u := overdraftTestRepo(t, 0.01)
	ctx := context.Background()

	if err := r.BatchRecordUsageLogsContext(ctx, []*UsageLogRecord{
		billingRecord(&APIKeyRecord{Key: ""}, u.Username, "tr-tenant-clamp", "bill-tenant-clamp", 8.0),
	}); err != nil {
		t.Fatalf("write: %v", err)
	}
	user, _ := r.GetUserByID(u.ID)
	if want := -ToUnits(defaultMaxOverdraftCNY); user.BalanceUnits != want {
		t.Fatalf("tenant fallback balance_units = %d, want %d", user.BalanceUnits, want)
	}
}
