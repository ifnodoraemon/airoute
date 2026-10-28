package storage

import (
	"context"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/lib/pq"

	"github.com/ifnodoraemon/airoute/internal/telemetry"
)

// A batch flush charges every key it contains exactly once, regardless of the
// order the records arrived in. The lock order is sorted internally (see
// sortedMapKeys), so the map iteration order cannot leak into the result.
func TestSettlementBatchChargesEveryKeyInStableOrder(t *testing.T) {
	r := accountWalletRepo(t)
	ctx := context.Background()

	names := []string{"batch-user-c", "batch-user-a", "batch-user-b"}
	type fixture struct {
		user *UserRecord
		key  *APIKeyRecord
		cost float64
		bill string
	}
	fixtures := make([]fixture, 0, len(names))
	for i, name := range names {
		u := walletTestUser(t, r, name, name+"@example.invalid")
		k := &APIKeyRecord{Key: "sk-" + name, UserID: u.ID, TenantID: u.Username}
		if err := r.CreateAPIKey(k); err != nil {
			t.Fatal(err)
		}
		fixtures = append(fixtures, fixture{
			user: u,
			key:  k,
			cost: []float64{1.00, 2.50, 0.25}[i],
			bill: fmt.Sprintf("bill-batch-%d", i),
		})
	}

	// Deliver in reverse order: a map-based implementation would lock different
	// keys in a different order depending on Go's randomised iteration.
	logs := make([]*UsageLogRecord, 0, len(fixtures))
	for i := len(fixtures) - 1; i >= 0; i-- {
		f := fixtures[i]
		logs = append(logs, billingRecord(f.key, f.user.Username, "tr-batch-"+f.bill, f.bill, f.cost))
	}
	if err := r.settleCharges(ctx, logs); err != nil {
		t.Fatalf("batch settle: %v", err)
	}

	for _, f := range fixtures {
		user, err := r.GetUserByID(f.user.ID)
		if err != nil {
			t.Fatalf("load user %s: %v", f.user.Username, err)
		}
		if want := ToUnits(10 - f.cost); user.BalanceUnits != want {
			t.Fatalf("%s balance_units = %d, want %d", f.user.Username, user.BalanceUnits, want)
		}
		gotKey, err := r.GetAPIKeyByKey(f.key.Key)
		if err != nil {
			t.Fatalf("load key %s: %v", f.key.Key, err)
		}
		if want := ToUnits(f.cost); gotKey.UsedCostUnits != want {
			t.Fatalf("%s used_cost_units = %d, want %d", f.key.Key, gotKey.UsedCostUnits, want)
		}
		if want := int64(1_000_000); gotKey.UsedTokens != want {
			t.Fatalf("%s used_tokens = %d, want %d", f.key.Key, gotKey.UsedTokens, want)
		}
	}

	var settlements int
	if err := r.db.QueryRow(`SELECT COUNT(*) FROM billing_settlements`).Scan(&settlements); err != nil {
		t.Fatal(err)
	}
	if settlements != len(fixtures) {
		t.Fatalf("billing_settlements rows = %d, want %d", settlements, len(fixtures))
	}

	// Redeliver the same billing_ids in the opposite order: the ledger is
	// idempotent, so the balances must not move again.
	redelivery := make([]*UsageLogRecord, 0, len(fixtures))
	for _, f := range fixtures {
		redelivery = append(redelivery, billingRecord(f.key, f.user.Username, "tr-batch-"+f.bill, f.bill, f.cost))
	}
	if err := r.settleCharges(ctx, redelivery); err != nil {
		t.Fatalf("redelivery: %v", err)
	}
	for _, f := range fixtures {
		user, err := r.GetUserByID(f.user.ID)
		if err != nil {
			t.Fatal(err)
		}
		if want := ToUnits(10 - f.cost); user.BalanceUnits != want {
			t.Fatalf("%s balance_units = %d after redelivery, want %d", f.user.Username, user.BalanceUnits, want)
		}
	}
}

func TestSortedMapKeysIsAscending(t *testing.T) {
	keys := sortedMapKeys(map[string]float64{"tenant-b": 1, "tenant-a": 2, "tenant-c": 3})
	want := []string{"tenant-a", "tenant-b", "tenant-c"}
	if len(keys) != len(want) {
		t.Fatalf("keys = %v, want %v", keys, want)
	}
	for i := range want {
		if keys[i] != want[i] {
			t.Fatalf("keys = %v, want %v", keys, want)
		}
	}
	if got := sortedMapKeys[float64](nil); len(got) != 0 {
		t.Fatalf("sortedMapKeys(nil) = %v, want empty", got)
	}
}

// A wallet that has been clamped to the overdraft floor could not absorb the
// charge, and the waived-units counter is the only place that shortfall is
// visible. The second charge starts from the floor, so it is entirely waived.
func TestWalletDeductionCountsWaivedUnitsAtFloor(t *testing.T) {
	t.Setenv("GATEWAY_MAX_OVERDRAFT_CNY", "1")
	r := accountWalletRepo(t)
	ctx := context.Background()

	u := walletTestUser(t, r, "waived-user", "")
	if err := r.SetUserBalance(u.Username, 0.01); err != nil {
		t.Fatal(err)
	}
	key := &APIKeyRecord{Key: "sk-waived", UserID: u.ID, TenantID: u.Username}
	if err := r.CreateAPIKey(key); err != nil {
		t.Fatal(err)
	}

	before := telemetry.GlobalMetrics.BillingWaivedUnits.Load()
	if err := r.settleCharges(ctx, []*UsageLogRecord{
		billingRecord(key, u.Username, "tr-waived-1", "bill-waived-1", 5.0),
	}); err != nil {
		t.Fatalf("settle 1: %v", err)
	}
	if got, want := telemetry.GlobalMetrics.BillingWaivedUnits.Load()-before, ToUnits(5.0); got != want {
		t.Fatalf("waived units after the clamped charge = %d, want %d", got, want)
	}
	user, err := r.GetUserByID(u.ID)
	if err != nil {
		t.Fatal(err)
	}
	if want := -ToUnits(1.0); user.BalanceUnits != want {
		t.Fatalf("balance_units = %d, want the %d floor", user.BalanceUnits, want)
	}

	// The wallet is already at the floor: the whole second charge is forgiven.
	before = telemetry.GlobalMetrics.BillingWaivedUnits.Load()
	if err := r.settleCharges(ctx, []*UsageLogRecord{
		billingRecord(key, u.Username, "tr-waived-2", "bill-waived-2", 2.0),
	}); err != nil {
		t.Fatalf("settle 2: %v", err)
	}
	if got, want := telemetry.GlobalMetrics.BillingWaivedUnits.Load()-before, ToUnits(2.0); got != want {
		t.Fatalf("waived units after the floor charge = %d, want %d", got, want)
	}
}

// A charge the wallet can afford is not waived. Counting every clamped UPDATE as
// waived would make the metric useless as a credit signal.
func TestWalletDeductionDoesNotCountWaivedUnitsAboveFloor(t *testing.T) {
	t.Setenv("GATEWAY_MAX_OVERDRAFT_CNY", "1")
	r := accountWalletRepo(t)
	ctx := context.Background()

	u := walletTestUser(t, r, "funded-user", "")
	key := &APIKeyRecord{Key: "sk-funded", UserID: u.ID, TenantID: u.Username}
	if err := r.CreateAPIKey(key); err != nil {
		t.Fatal(err)
	}

	before := telemetry.GlobalMetrics.BillingWaivedUnits.Load()
	if err := r.settleCharges(ctx, []*UsageLogRecord{
		billingRecord(key, u.Username, "tr-funded", "bill-funded", 1.5),
	}); err != nil {
		t.Fatalf("settle: %v", err)
	}
	if got := telemetry.GlobalMetrics.BillingWaivedUnits.Load() - before; got != 0 {
		t.Fatalf("waived units = %d for a funded wallet, want 0", got)
	}
}

// With overdraft disabled the wallet stops at exactly zero (floor == 0). The
// balance never goes negative, so the overdraft warning cannot fire, but the
// charge is still waived and must be counted.
func TestWalletDeductionCountsWaivedUnitsAtZeroFloor(t *testing.T) {
	t.Setenv("GATEWAY_MAX_OVERDRAFT_CNY", "0")
	r := accountWalletRepo(t)
	ctx := context.Background()

	u := walletTestUser(t, r, "zero-floor-user", "")
	if err := r.SetUserBalance(u.Username, 1.0); err != nil {
		t.Fatal(err)
	}
	key := &APIKeyRecord{Key: "sk-zero-floor", UserID: u.ID, TenantID: u.Username}
	if err := r.CreateAPIKey(key); err != nil {
		t.Fatal(err)
	}

	overdraftsBefore := telemetry.GlobalMetrics.BillingOverdrafts.Load()
	waivedBefore := telemetry.GlobalMetrics.BillingWaivedUnits.Load()
	if err := r.settleCharges(ctx, []*UsageLogRecord{
		billingRecord(key, u.Username, "tr-zero-floor", "bill-zero-floor", 1.0),
	}); err != nil {
		t.Fatalf("settle: %v", err)
	}
	user, err := r.GetUserByID(u.ID)
	if err != nil {
		t.Fatal(err)
	}
	if user.BalanceUnits != 0 {
		t.Fatalf("balance_units = %d, want 0", user.BalanceUnits)
	}
	if got, want := telemetry.GlobalMetrics.BillingWaivedUnits.Load()-waivedBefore, ToUnits(1.0); got != want {
		t.Fatalf("waived units = %d, want %d", got, want)
	}
	if got := telemetry.GlobalMetrics.BillingOverdrafts.Load() - overdraftsBefore; got != 0 {
		t.Fatalf("overdraft counter moved by %d with a zero floor, want 0", got)
	}
}

func TestIsRetryableSettlementErr(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want bool
	}{
		{"nil", nil, false},
		{"deadlock", &pq.Error{Code: "40P01"}, true},
		{"lock not available", &pq.Error{Code: "55P03"}, true},
		{"query canceled", &pq.Error{Code: "57014"}, true},
		{"wrapped deadlock", fmt.Errorf("settle: %w", &pq.Error{Code: "40P01"}), true},
		{"unique violation", &pq.Error{Code: "23505"}, false},
		{"plain error", errors.New("boom"), false},
		{"context canceled", context.Canceled, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := isRetryableSettlementErr(tc.err); got != tc.want {
				t.Fatalf("isRetryableSettlementErr(%v) = %v, want %v", tc.err, got, tc.want)
			}
		})
	}
}

func TestSettlementRetryDelayStaysWithinBounds(t *testing.T) {
	for attempt := 0; attempt < 16; attempt++ {
		delay := settlementRetryDelay(attempt)
		if delay < settlementRetryBaseDelay || delay > settlementRetryMaxDelay {
			t.Fatalf("attempt %d delay = %v, want [%v, %v]",
				attempt, delay, settlementRetryBaseDelay, settlementRetryMaxDelay)
		}
	}
}

// The settlement histogram measures the database transaction, and it must record
// one observation per attempt even when the attempt succeeds immediately.
func TestSettlementObservesTransactionDuration(t *testing.T) {
	r, key, u := billingIDTestRepo(t)

	_, _, before := telemetry.SettlementDurationSeconds.Snapshot()
	if err := r.settleCharges(context.Background(), []*UsageLogRecord{
		billingRecord(key, u.Username, "tr-duration", "bill-duration", 0.5),
	}); err != nil {
		t.Fatalf("settle: %v", err)
	}
	_, _, after := telemetry.SettlementDurationSeconds.Snapshot()
	if after <= before {
		t.Fatalf("SettlementDurationSeconds count = %d after settlement, want more than %d", after, before)
	}
}

// settlementPostgresTestURL mirrors the integration-suite helper
// (testPostgresURL, which is behind the `integration` build tag) so this
// regression test can also be run on its own against a scratch database.
func settlementPostgresTestURL(t *testing.T) string {
	t.Helper()
	dsn := os.Getenv("AIROUTE_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("AIROUTE_TEST_DATABASE_URL is not set; skipping PostgreSQL integration test")
	}
	return dsn
}

// TestPostgresMergedSettlementPath exercises the PostgreSQL-only merged
// key+counter+wallet statement end to end (Rebind, lib/pq parameter binding,
// admin exemption, tenant fallback). It only runs when AIROUTE_TEST_DATABASE_URL
// points at a PostgreSQL server; the SQLite suite never takes this branch.
func TestPostgresMergedSettlementPath(t *testing.T) {
	db, err := OpenDB(settlementPostgresTestURL(t))
	if err != nil {
		t.Fatalf("open postgres: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if db.Dialect() != "postgres" {
		t.Fatalf("dialect = %q, want postgres", db.Dialect())
	}
	// PostgreSQL does not need a separate read pool.
	if db.Reader() != db.DB {
		t.Fatal("postgres Reader() should fall back to the main pool")
	}

	r := NewRepository(db)
	ctx := context.Background()
	suffix := fmt.Sprintf("%d", time.Now().UnixNano())

	// Normal user: the merged statement moves the counter and the wallet together.
	user := &UserRecord{
		Username:     "pg-settle-user-" + suffix,
		Email:        "pg-settle-" + suffix + "@example.invalid",
		PasswordHash: "synthetic",
	}
	if err := r.CreateUser(user); err != nil {
		t.Fatalf("create user: %v", err)
	}
	if err := r.SetUserBalance(user.Username, 10); err != nil {
		t.Fatalf("set balance: %v", err)
	}
	key := &APIKeyRecord{Key: "sk-pg-settle-" + suffix, UserID: user.ID, TenantID: user.Username}
	if err := r.CreateAPIKey(key); err != nil {
		t.Fatalf("create key: %v", err)
	}

	billID := "bill-pg-merged-" + suffix
	if err := r.settleCharges(ctx, []*UsageLogRecord{
		billingRecord(key, user.Username, "tr-pg-merged-"+suffix, billID, 2.5),
	}); err != nil {
		t.Fatalf("merged settle: %v", err)
	}
	loaded, err := r.GetUserByID(user.ID)
	if err != nil {
		t.Fatalf("reload user: %v", err)
	}
	if want := ToUnits(7.5); loaded.BalanceUnits != want {
		t.Fatalf("balance_units = %d, want %d", loaded.BalanceUnits, want)
	}
	loadedKey, err := r.GetAPIKeyByKey(key.Key)
	if err != nil {
		t.Fatalf("reload key: %v", err)
	}
	if want := ToUnits(2.5); loadedKey.UsedCostUnits != want {
		t.Fatalf("used_cost_units = %d, want %d", loadedKey.UsedCostUnits, want)
	}
	if want := int64(1_000_000); loadedKey.UsedTokens != want {
		t.Fatalf("used_tokens = %d, want %d", loadedKey.UsedTokens, want)
	}

	// Redelivery must not charge again.
	if err := r.settleCharges(ctx, []*UsageLogRecord{
		billingRecord(key, user.Username, "tr-pg-merged-"+suffix, billID, 2.5),
	}); err != nil {
		t.Fatalf("redelivery: %v", err)
	}
	loaded, err = r.GetUserByID(user.ID)
	if err != nil {
		t.Fatal(err)
	}
	if want := ToUnits(7.5); loaded.BalanceUnits != want {
		t.Fatalf("balance_units = %d after redelivery, want %d", loaded.BalanceUnits, want)
	}

	// Admin-owned key: counters move, the wallet is exempt and no tenant fallback
	// may collect the charge.
	admin := &UserRecord{
		Username:     "pg-settle-admin-" + suffix,
		Role:         "admin",
		PasswordHash: "synthetic",
	}
	if err := r.CreateUser(admin); err != nil {
		t.Fatalf("create admin: %v", err)
	}
	if err := r.SetUserBalance(admin.Username, 5); err != nil {
		t.Fatalf("set admin balance: %v", err)
	}
	adminKey := &APIKeyRecord{Key: "sk-pg-admin-" + suffix, UserID: admin.ID, TenantID: admin.Username}
	if err := r.CreateAPIKey(adminKey); err != nil {
		t.Fatalf("create admin key: %v", err)
	}
	unattributedBefore := telemetry.GlobalMetrics.BillingUnattributed.Load()
	if err := r.settleCharges(ctx, []*UsageLogRecord{
		billingRecord(adminKey, admin.Username, "tr-pg-admin-"+suffix, "bill-pg-admin-"+suffix, 3.0),
	}); err != nil {
		t.Fatalf("admin settle: %v", err)
	}
	adminLoaded, err := r.GetUserByID(admin.ID)
	if err != nil {
		t.Fatal(err)
	}
	if want := ToUnits(5.0); adminLoaded.BalanceUnits != want {
		t.Fatalf("admin balance_units = %d, want %d (admin keys are exempt)", adminLoaded.BalanceUnits, want)
	}
	adminKeyLoaded, err := r.GetAPIKeyByKey(adminKey.Key)
	if err != nil {
		t.Fatal(err)
	}
	if want := ToUnits(3.0); adminKeyLoaded.UsedCostUnits != want {
		t.Fatalf("admin key used_cost_units = %d, want %d (counters still move)", adminKeyLoaded.UsedCostUnits, want)
	}
	if got := telemetry.GlobalMetrics.BillingUnattributed.Load() - unattributedBefore; got != 0 {
		t.Fatalf("admin charge reported %d unattributed charges, want 0", got)
	}

	// Orphan key (user_id = 0) with a named tenant: the charge falls back to the
	// tenant's wallet, which is exactly what the merged statement's owner_is_admin
	// = 0 path must preserve.
	tenant := &UserRecord{
		Username:     "pg-settle-tenant-" + suffix,
		Email:        "pg-settle-tenant-" + suffix + "@example.invalid",
		PasswordHash: "synthetic",
	}
	if err := r.CreateUser(tenant); err != nil {
		t.Fatalf("create tenant: %v", err)
	}
	if err := r.SetUserBalance(tenant.Username, 10); err != nil {
		t.Fatalf("set tenant balance: %v", err)
	}
	orphanKey := &APIKeyRecord{Key: "sk-pg-orphan-" + suffix, TenantID: tenant.Username}
	if err := r.CreateAPIKey(orphanKey); err != nil {
		t.Fatalf("create orphan key: %v", err)
	}
	// CreateAPIKey resolves a tenant to its user id, so force the orphan shape.
	if _, err := db.Exec(`UPDATE api_keys SET user_id = 0 WHERE key = ?`, orphanKey.Key); err != nil {
		t.Fatalf("orphan the key: %v", err)
	}
	if err := r.settleCharges(ctx, []*UsageLogRecord{
		billingRecord(orphanKey, tenant.Username, "tr-pg-orphan-"+suffix, "bill-pg-orphan-"+suffix, 4.0),
	}); err != nil {
		t.Fatalf("tenant fallback settle: %v", err)
	}
	tenantLoaded, err := r.GetUserByID(tenant.ID)
	if err != nil {
		t.Fatal(err)
	}
	if want := ToUnits(6.0); tenantLoaded.BalanceUnits != want {
		t.Fatalf("tenant balance_units = %d, want %d (fallback must collect the charge)", tenantLoaded.BalanceUnits, want)
	}

	// Clean up this run's rows so a re-run against the same database stays clean.
	cleanup := []string{
		`DELETE FROM billing_settlements WHERE billing_id LIKE ?`,
		`DELETE FROM api_keys WHERE key LIKE ?`,
		`DELETE FROM users WHERE username LIKE ?`,
	}
	like := "%" + suffix
	for _, q := range cleanup {
		if _, err := db.Exec(q, like); err != nil {
			t.Logf("cleanup %q: %v", q, err)
		}
	}
}
