package storage

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/ifnodoraemon/airoute/internal/telemetry"
)

func settlementTestRepo(t *testing.T) (*Repository, *APIKeyRecord, *UserRecord) {
	t.Helper()
	r, key, u := billingIDTestRepo(t)
	return r, key, u
}

// The whole point of the split: the wallet moves even when no audit row is ever
// written. Before this, the charge lived inside the audit insert transaction, so
// a failing log pipeline silently deferred (or lost) revenue.
func TestSettlementMovesMoneyWithoutAnAuditRow(t *testing.T) {
	r, key, u := settlementTestRepo(t)

	rec := billingRecord(key, u.Username, "tr-settle-only", "bill-settle-only", 1.50)
	if err := r.SettleUsageCharge(context.Background(), rec); err != nil {
		t.Fatalf("settle: %v", err)
	}

	var audits int
	if err := r.db.QueryRow(`SELECT COUNT(*) FROM usage_logs`).Scan(&audits); err != nil {
		t.Fatal(err)
	}
	if audits != 0 {
		t.Fatalf("usage_logs rows = %d, want 0: settlement must not write the audit row", audits)
	}

	var settlements int
	if err := r.db.QueryRow(`SELECT COUNT(*) FROM billing_settlements`).Scan(&settlements); err != nil {
		t.Fatal(err)
	}
	if settlements != 1 {
		t.Fatalf("billing_settlements rows = %d, want 1", settlements)
	}

	user, _ := r.GetUserByID(u.ID)
	if want := ToUnits(8.50); user.BalanceUnits != want {
		t.Fatalf("balance units = %d, want %d (money moved without an audit row)", user.BalanceUnits, want)
	}
	gotKey, _ := r.GetAPIKeyByKey(key.Key)
	if want := ToUnits(1.50); gotKey.UsedCostUnits != want {
		t.Fatalf("key used_cost_units = %d, want %d", gotKey.UsedCostUnits, want)
	}

	// Writing the audit row afterwards must not charge again.
	if err := r.RecordUsageLogContext(context.Background(), rec); err != nil {
		t.Fatalf("audit write: %v", err)
	}
	user, _ = r.GetUserByID(u.ID)
	if want := ToUnits(8.50); user.BalanceUnits != want {
		t.Fatalf("balance units = %d after the audit write, want %d (no double charge)", user.BalanceUnits, want)
	}
	if err := r.db.QueryRow(`SELECT COUNT(*) FROM usage_logs`).Scan(&audits); err != nil {
		t.Fatal(err)
	}
	if audits != 1 {
		t.Fatalf("usage_logs rows = %d, want 1", audits)
	}
}

func TestSettlementIsIdempotentPerBillingID(t *testing.T) {
	r, key, u := settlementTestRepo(t)
	ctx := context.Background()

	for i := 0; i < 3; i++ {
		rec := billingRecord(key, u.Username, "tr-settle-dup", "bill-settle-dup", 2.00)
		if err := r.SettleUsageCharge(ctx, rec); err != nil {
			t.Fatalf("settle %d: %v", i+1, err)
		}
	}

	var settlements int
	if err := r.db.QueryRow(`SELECT COUNT(*) FROM billing_settlements`).Scan(&settlements); err != nil {
		t.Fatal(err)
	}
	if settlements != 1 {
		t.Fatalf("billing_settlements rows = %d, want 1", settlements)
	}
	user, _ := r.GetUserByID(u.ID)
	if want := ToUnits(8.00); user.BalanceUnits != want {
		t.Fatalf("balance units = %d, want %d (charged once)", user.BalanceUnits, want)
	}
}

// Records with nothing to charge must not pollute the settlement ledger, but they
// are marked settled so the writer does not reconsider them.
func TestSettlementSkipsFreeRecords(t *testing.T) {
	r := accountWalletRepo(t)

	rec := &UsageLogRecord{TraceID: "tr-free-settle", BillingID: "bill-free-settle", StatusCode: 200}
	if err := r.SettleUsageCharge(context.Background(), rec); err != nil {
		t.Fatalf("settle: %v", err)
	}
	var settlements int
	if err := r.db.QueryRow(`SELECT COUNT(*) FROM billing_settlements`).Scan(&settlements); err != nil {
		t.Fatal(err)
	}
	if settlements != 0 {
		t.Fatalf("billing_settlements rows = %d, want 0 for a free record", settlements)
	}
	if !rec.Settled {
		t.Fatal("free record was not marked settled")
	}
}

func TestSettleChargeNowRequiresInitialization(t *testing.T) {
	previous := settlementRepo
	settlementRepo = nil
	defer func() { settlementRepo = previous }()

	if err := SettleChargeNow(context.Background(), &UsageLogRecord{Cost: 1}); !errors.Is(err, ErrSettlementNotInitialized) {
		t.Fatalf("error = %v, want ErrSettlementNotInitialized", err)
	}
}

// A settled charge with no audit row is exactly the state the reconciliation
// audit-lag gauge has to surface.
func TestReconcilerReportsAuditLagForUnwrittenRows(t *testing.T) {
	r, key, u := settlementTestRepo(t)
	// The gauge compares both sides at a watermark that trails the wall clock (see
	// billingAuditLagOverlapWindow). This test is about the missing-audit-row state
	// itself, so it pins the overlap to zero; with the shipped 5-minute window the
	// same charge surfaces on the next pass instead of immediately, which is what
	// keeps a charge whose audit row is still in flight from looking like lag.
	previousOverlap := billingAuditLagOverlapWindow
	billingAuditLagOverlapWindow = 0
	t.Cleanup(func() { billingAuditLagOverlapWindow = previousOverlap })

	if err := r.SettleUsageCharge(context.Background(), billingRecord(key, u.Username, "tr-lag", "bill-lag", 3.00)); err != nil {
		t.Fatalf("settle: %v", err)
	}

	worker := &BillingReconciler{repo: r, stop: make(chan struct{})}
	worker.runOnce()

	if got := telemetry.GlobalMetrics.BillingAuditLagUnits.Load(); got != ToUnits(3.00) {
		t.Fatalf("audit lag = %d units, want %d", got, ToUnits(3.00))
	}
	if got := telemetry.GlobalMetrics.BillingLedgerDriftUnits.Load(); got != 0 {
		t.Fatalf("ledger drift = %d units, want 0 (key counters follow the settlement ledger)", got)
	}
}

// A rolling deploy leaves audit rows written by the previous binary, which knows
// nothing about the settlement ledger. The reconciler must record the missing
// ledger rows (without moving money again) so the two totals converge.
func TestReconcilerRepairsAuditRowsWithoutSettlement(t *testing.T) {
	r, key, u := settlementTestRepo(t)

	rec := billingRecord(key, u.Username, "tr-gap", "bill-gap", 4.00)
	rec.CreatedAt = time.Now().Add(-10 * time.Minute)

	// Audit row only, as the pre-ledger binary would have written it.
	if err := r.insertUsageLogsContext(context.Background(), []*UsageLogRecord{rec}); err != nil {
		t.Fatalf("audit write: %v", err)
	}
	// The old binary moved the key counter and wallet; mirror that state.
	if _, err := r.db.Exec(`UPDATE api_keys SET used_cost_units = ? WHERE key = ?`, ToUnits(4), key.Key); err != nil {
		t.Fatal(err)
	}
	if _, err := r.db.Exec(`UPDATE users SET balance_units = ?, balance = 6 WHERE id = ?`, ToUnits(6), u.ID); err != nil {
		t.Fatal(err)
	}

	var settlements int
	if err := r.db.QueryRow(`SELECT COUNT(*) FROM billing_settlements WHERE billing_id = 'bill-gap'`).Scan(&settlements); err != nil {
		t.Fatal(err)
	}
	if settlements != 0 {
		t.Fatalf("settlement rows = %d before repair, want 0", settlements)
	}

	worker := &BillingReconciler{repo: r, stop: make(chan struct{})}
	worker.runOnce()

	if err := r.db.QueryRow(`SELECT COUNT(*) FROM billing_settlements WHERE billing_id = 'bill-gap'`).Scan(&settlements); err != nil {
		t.Fatal(err)
	}
	if settlements != 1 {
		t.Fatalf("settlement rows = %d after repair, want 1", settlements)
	}
	if got := telemetry.GlobalMetrics.BillingAuditLagUnits.Load(); got != 0 {
		t.Fatalf("audit lag = %d units, want 0 after repair", got)
	}
	user, _ := r.GetUserByID(u.ID)
	if want := ToUnits(6); user.BalanceUnits != want {
		t.Fatalf("balance units = %d, want %d (repair must not move money)", user.BalanceUnits, want)
	}
}
