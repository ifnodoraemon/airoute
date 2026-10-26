//go:build integration

package storage

import (
	"context"
	"testing"
	"time"
)

// TestPostgresUsageLogRedeliveryIsBilledOnce exercises the exactly-once billing
// path on real PostgreSQL: the partial unique index on billing_id, the
// multi-row INSERT ... ON CONFLICT DO NOTHING RETURNING projection, and the
// wallet/key aggregation that must only count rows actually inserted.
//
// SQLite and PostgreSQL differ here in ways unit tests cannot cover (RETURNING
// with a multi-row VALUES list, conflict arbitration and RowsAffected).
func TestPostgresUsageLogRedeliveryIsBilledOnce(t *testing.T) {
	db, err := OpenDB(testPostgresURL(t))
	if err != nil {
		t.Fatalf("open postgres: %v", err)
	}
	defer db.Close()
	repo := NewRepository(db)
	ctx := context.Background()
	suffix := itSuffix()

	user := &UserRecord{Username: "pg-billing-" + suffix, Email: "pg-billing-" + suffix + "@example.invalid", PasswordHash: "synthetic", Balance: 10}
	if err := repo.CreateUser(user); err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	key := &APIKeyRecord{Key: "sk-pg-billing-" + suffix, UserID: user.ID, TenantID: user.Username}
	if err := repo.CreateAPIKey(key); err != nil {
		t.Fatalf("CreateAPIKey: %v", err)
	}

	// The auth middleware's authoritative lookup is context-bound and uses '?'
	// placeholders. It silently failed on PostgreSQL until the DB wrapper also
	// rebound the *Context variants, so pin the behaviour here.
	ctxLookup, cancelLookup := context.WithTimeout(context.Background(), 5*time.Second)
	looked, err := repo.GetAPIKeyByKeyContext(ctxLookup, key.Key)
	cancelLookup()
	if err != nil {
		t.Fatalf("GetAPIKeyByKeyContext on postgres: %v", err)
	}
	if looked.UserID != user.ID || looked.UserStatus != "active" {
		t.Fatalf("context-bound key lookup returned %+v", looked)
	}
	if _, err := repo.GetUserByIDContext(context.Background(), user.ID); err != nil {
		t.Fatalf("GetUserByIDContext on postgres: %v", err)
	}

	deliver := func() []*UsageLogRecord {
		return []*UsageLogRecord{
			{TraceID: "tr-pg-" + suffix + "-a", APIKey: key.Key, TenantID: user.Username, Model: "m", PromptTokens: 1_000_000, TotalTokens: 1_000_000, Cost: 1.00, StatusCode: 200, BillingID: "bill-pg-" + suffix + "-a"},
			{TraceID: "tr-pg-" + suffix + "-b", APIKey: key.Key, TenantID: user.Username, Model: "m", PromptTokens: 1_000_000, TotalTokens: 1_000_000, Cost: 0.50, StatusCode: 200, BillingID: "bill-pg-" + suffix + "-b"},
		}
	}
	for i := 0; i < 3; i++ {
		if err := repo.BatchRecordUsageLogsContext(ctx, deliver()); err != nil {
			t.Fatalf("delivery %d: %v", i+1, err)
		}
	}

	var rows int
	if err := db.QueryRow(`SELECT COUNT(*) FROM usage_logs WHERE api_key = ?`, key.Key).Scan(&rows); err != nil {
		t.Fatalf("count rows: %v", err)
	}
	if rows != 2 {
		t.Fatalf("usage_logs rows = %d, want 2 after three identical deliveries", rows)
	}

	gotUser, err := repo.GetUserByID(user.ID)
	if err != nil {
		t.Fatalf("GetUserByID: %v", err)
	}
	if want := ToUnits(8.50); gotUser.BalanceUnits != want {
		t.Fatalf("balance units = %d, want %d (wallet charged more than once)", gotUser.BalanceUnits, want)
	}

	gotKey, err := repo.GetAPIKeyByKey(key.Key)
	if err != nil {
		t.Fatalf("GetAPIKeyByKey: %v", err)
	}
	if want := ToUnits(1.50); gotKey.UsedCostUnits != want {
		t.Fatalf("key used_cost_units = %d, want %d", gotKey.UsedCostUnits, want)
	}

	// A single-record delivery (the flush-of-one path) must be idempotent too.
	single := func() []*UsageLogRecord {
		return []*UsageLogRecord{{TraceID: "tr-pg-" + suffix + "-c", APIKey: key.Key, TenantID: user.Username, Model: "m", Cost: 2.00, TotalTokens: 10, StatusCode: 200, BillingID: "bill-pg-" + suffix + "-c"}}
	}
	for i := 0; i < 2; i++ {
		if err := repo.BatchRecordUsageLogsContext(ctx, single()); err != nil {
			t.Fatalf("single delivery %d: %v", i+1, err)
		}
	}
	if err := db.QueryRow(`SELECT COUNT(*) FROM usage_logs WHERE api_key = ?`, key.Key).Scan(&rows); err != nil {
		t.Fatalf("count rows: %v", err)
	}
	if rows != 3 {
		t.Fatalf("usage_logs rows = %d, want 3", rows)
	}
	gotUser, _ = repo.GetUserByID(user.ID)
	if want := ToUnits(6.50); gotUser.BalanceUnits != want {
		t.Fatalf("balance units = %d, want %d", gotUser.BalanceUnits, want)
	}
}

// The clamped deduction uses UPDATE ... RETURNING plus a portable CASE
// expression and numeric division for the float mirror. PostgreSQL differs from
// SQLite in all three, so exercise it against a real server.
func TestPostgresWalletDeductionClampsAtOverdraftFloor(t *testing.T) {
	db, err := OpenDB(testPostgresURL(t))
	if err != nil {
		t.Fatalf("open postgres: %v", err)
	}
	defer db.Close()
	repo := NewRepository(db)
	suffix := itSuffix()

	user := &UserRecord{Username: "pg-overdraft-" + suffix, PasswordHash: "synthetic", Balance: 0.01}
	if err := repo.CreateUser(user); err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	key := &APIKeyRecord{Key: "sk-pg-overdraft-" + suffix, UserID: user.ID, TenantID: user.Username}
	if err := repo.CreateAPIKey(key); err != nil {
		t.Fatalf("CreateAPIKey: %v", err)
	}
	if err := repo.BatchRecordUsageLogsContext(context.Background(), []*UsageLogRecord{{
		TraceID: "tr-pg-od-" + suffix, APIKey: key.Key, TenantID: user.Username, Model: "m",
		Cost: 9.0, StatusCode: 200, BillingID: "bill-pg-od-" + suffix,
	}}); err != nil {
		t.Fatalf("write: %v", err)
	}

	got, err := repo.GetUserByID(user.ID)
	if err != nil {
		t.Fatalf("GetUserByID: %v", err)
	}
	if want := -ToUnits(defaultMaxOverdraftCNY); got.BalanceUnits != want {
		t.Fatalf("balance_units = %d, want the %d floor", got.BalanceUnits, want)
	}
	if got.Balance < -defaultMaxOverdraftCNY-1e-9 || got.Balance > -defaultMaxOverdraftCNY+1e-9 {
		t.Fatalf("balance = %v, want the float mirror at %v", got.Balance, -defaultMaxOverdraftCNY)
	}
}

// The legacy-row repair uses a dialect-specific cast; verify it against real
// PostgreSQL rather than only SQLite.
func TestPostgresBackfillUsageLogCostUnits(t *testing.T) {
	db, err := OpenDB(testPostgresURL(t))
	if err != nil {
		t.Fatalf("open postgres: %v", err)
	}
	defer db.Close()
	suffix := itSuffix()
	traceID := "tr-pg-legacy-" + suffix

	if _, err := db.Exec(`INSERT INTO usage_logs (trace_id, model, cost, cost_units) VALUES (?, 'm', 2.25, 0)`, traceID); err != nil {
		t.Fatalf("insert legacy row: %v", err)
	}
	if err := backfillUsageLogCostUnits(db); err != nil {
		t.Fatalf("backfill: %v", err)
	}
	var units int64
	if err := db.QueryRow(`SELECT cost_units FROM usage_logs WHERE trace_id = ?`, traceID).Scan(&units); err != nil {
		t.Fatalf("read back: %v", err)
	}
	if want := ToUnits(2.25); units != want {
		t.Fatalf("cost_units = %d, want %d", units, want)
	}
}
