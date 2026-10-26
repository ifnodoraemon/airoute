package storage

import (
	"context"
	"testing"
)

func billingIDTestRepo(t *testing.T) (*Repository, *APIKeyRecord, *UserRecord) {
	t.Helper()
	r := accountWalletRepo(t)
	u := walletTestUser(t, r, "paid-user", "")
	key := &APIKeyRecord{Key: "sk-paid", UserID: u.ID, TenantID: u.Username}
	if err := r.CreateAPIKey(key); err != nil {
		t.Fatal(err)
	}
	return r, key, u
}

func billingRecord(key *APIKeyRecord, tenant, traceID, billingID string, cost float64) *UsageLogRecord {
	return &UsageLogRecord{
		TraceID:      traceID,
		APIKey:       key.Key,
		TenantID:     tenant,
		Model:        "test-model",
		PromptTokens: 1_000_000,
		TotalTokens:  1_000_000,
		Cost:         cost,
		StatusCode:   200,
		BillingID:    billingID,
	}
}

// A Redis Stream delivery is at-least-once: once the database transaction
// commits but the XACK is lost, the message is redelivered by startup PEL
// recovery or the AutoClaim ticker. Before the billing_id key existed that
// replay inserted a second usage row *and* deducted the wallet a second time,
// with no way to identify the duplicate afterwards.
func TestBatchRedeliveryIsBilledOnce(t *testing.T) {
	r, key, u := billingIDTestRepo(t)
	ctx := context.Background()

	deliver := func() []*UsageLogRecord {
		return []*UsageLogRecord{
			billingRecord(key, u.Username, "tr-a", "bill-fixed-a", 1.00),
			billingRecord(key, u.Username, "tr-b", "bill-fixed-b", 0.50),
		}
	}

	if err := r.BatchRecordUsageLogsContext(ctx, deliver()); err != nil {
		t.Fatalf("first delivery: %v", err)
	}
	if err := r.BatchRecordUsageLogsContext(ctx, deliver()); err != nil {
		t.Fatalf("redelivery: %v", err)
	}

	var rows int
	if err := r.db.QueryRow(`SELECT COUNT(*) FROM usage_logs`).Scan(&rows); err != nil {
		t.Fatal(err)
	}
	if rows != 2 {
		t.Fatalf("usage_logs rows = %d, want 2 (redelivery must not insert)", rows)
	}

	// 10.00 - 1.50, applied once.
	user, err := r.GetUserByID(u.ID)
	if err != nil {
		t.Fatal(err)
	}
	if want := ToUnits(8.50); user.BalanceUnits != want || user.Balance != 8.50 {
		t.Fatalf("balance = %v (%d units), want 8.5 (%d units) - wallet was charged twice", user.Balance, user.BalanceUnits, want)
	}

	// The api key's quota mirror must also advance exactly once.
	gotKey, err := r.GetAPIKeyByKey(key.Key)
	if err != nil {
		t.Fatal(err)
	}
	if want := ToUnits(1.50); gotKey.UsedCostUnits != want {
		t.Fatalf("api key used_cost_units = %d, want %d", gotKey.UsedCostUnits, want)
	}
}

// The single-record path (a flush that contains exactly one row) must be just
// as idempotent as the batched one.
func TestSingleRedeliveryIsBilledOnce(t *testing.T) {
	r, key, u := billingIDTestRepo(t)
	ctx := context.Background()

	deliver := func() []*UsageLogRecord {
		return []*UsageLogRecord{billingRecord(key, u.Username, "tr-single", "bill-single", 2.00)}
	}
	for i := 0; i < 3; i++ {
		if err := r.BatchRecordUsageLogsContext(ctx, deliver()); err != nil {
			t.Fatalf("delivery %d: %v", i+1, err)
		}
	}

	var rows int
	if err := r.db.QueryRow(`SELECT COUNT(*) FROM usage_logs`).Scan(&rows); err != nil {
		t.Fatal(err)
	}
	if rows != 1 {
		t.Fatalf("usage_logs rows = %d, want 1", rows)
	}
	user, _ := r.GetUserByID(u.ID)
	if want := ToUnits(8.00); user.BalanceUnits != want {
		t.Fatalf("balance units = %d, want %d after three identical deliveries", user.BalanceUnits, want)
	}
}

// Deduplication must key on billing_id only. Legitimate requests can share a
// client-supplied trace id (X-Request-ID / traceparent), and every one of them
// still has to be billed.
func TestSameTraceIDStillBillsEachRequest(t *testing.T) {
	r, key, u := billingIDTestRepo(t)
	ctx := context.Background()

	recs := []*UsageLogRecord{
		billingRecord(key, u.Username, "tr-shared", "bill-1", 1.00),
		billingRecord(key, u.Username, "tr-shared", "bill-2", 1.00),
	}
	if err := r.BatchRecordUsageLogsContext(ctx, recs); err != nil {
		t.Fatal(err)
	}

	var rows int
	if err := r.db.QueryRow(`SELECT COUNT(*) FROM usage_logs WHERE trace_id = 'tr-shared'`).Scan(&rows); err != nil {
		t.Fatal(err)
	}
	if rows != 2 {
		t.Fatalf("rows for one trace id = %d, want 2 (a shared trace id must not dedupe)", rows)
	}
	user, _ := r.GetUserByID(u.ID)
	if want := ToUnits(8.00); user.BalanceUnits != want {
		t.Fatalf("balance units = %d, want %d", user.BalanceUnits, want)
	}
}

// Records without a key (open mode / tenant fallback) are deduplicated the same
// way, so a replay cannot charge the tenant's wallet twice.
func TestTenantFallbackRedeliveryIsBilledOnce(t *testing.T) {
	r, _, u := billingIDTestRepo(t)
	ctx := context.Background()

	deliver := func() []*UsageLogRecord {
		return []*UsageLogRecord{
			billingRecord(&APIKeyRecord{Key: ""}, u.Username, "tr-tenant", "bill-tenant", 1.25),
			billingRecord(&APIKeyRecord{Key: ""}, u.Username, "tr-tenant-2", "bill-tenant-2", 0.25),
		}
	}
	for i := 0; i < 2; i++ {
		if err := r.BatchRecordUsageLogsContext(ctx, deliver()); err != nil {
			t.Fatalf("delivery %d: %v", i+1, err)
		}
	}
	user, _ := r.GetUserByID(u.ID)
	if want := ToUnits(8.50); user.BalanceUnits != want {
		t.Fatalf("tenant balance units = %d, want %d", user.BalanceUnits, want)
	}
}
