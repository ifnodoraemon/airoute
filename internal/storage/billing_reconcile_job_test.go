package storage

import (
	"testing"

	"github.com/ifnodoraemon/airoute/internal/telemetry"
)

func TestBillingReconcilePublishesDrift(t *testing.T) {
	r := accountWalletRepo(t)
	u := walletTestUser(t, r, "recon-user", "")
	key := &APIKeyRecord{Key: "sk-recon", UserID: u.ID, TenantID: u.Username}
	if err := r.CreateAPIKey(key); err != nil {
		t.Fatal(err)
	}

	// Deliberately break both mirrors: the key claims ¥5 of usage that no audit
	// row backs, and the wallet's integer column disagrees with its float mirror.
	if _, err := r.db.Exec(`UPDATE api_keys SET used_cost_units = ? WHERE key = ?`, ToUnits(5), key.Key); err != nil {
		t.Fatal(err)
	}
	if _, err := r.db.Exec(`UPDATE users SET balance_units = ?, balance = 3 WHERE id = ?`, ToUnits(10), u.ID); err != nil {
		t.Fatal(err)
	}

	worker := &BillingReconciler{repo: r, stop: make(chan struct{})}
	worker.runOnce()

	if got, want := telemetry.GlobalMetrics.BillingLedgerDriftUnits.Load(), ToUnits(5); got != want {
		t.Fatalf("ledger drift = %d units, want %d", got, want)
	}
	if got, want := telemetry.GlobalMetrics.BillingWalletDriftUnits.Load(), ToUnits(7); got != want {
		t.Fatalf("wallet drift = %d units, want %d", got, want)
	}
}

func TestBillingReconcileCleanLedgerHasNoDrift(t *testing.T) {
	r, key, u := billingIDTestRepo(t)
	if err := r.BatchRecordUsageLogsContext(t.Context(), []*UsageLogRecord{
		billingRecord(key, u.Username, "tr-recon-clean", "bill-recon-clean", 1.25),
	}); err != nil {
		t.Fatal(err)
	}

	worker := &BillingReconciler{repo: r, stop: make(chan struct{})}
	worker.runOnce()

	if got := telemetry.GlobalMetrics.BillingLedgerDriftUnits.Load(); got != 0 {
		t.Fatalf("ledger drift = %d units, want 0 for a balanced ledger", got)
	}
	if got := telemetry.GlobalMetrics.BillingWalletDriftUnits.Load(); got != 0 {
		t.Fatalf("wallet drift = %d units, want 0", got)
	}
}

func TestBillingReconcileIntervalSetting(t *testing.T) {
	t.Setenv("GATEWAY_BILLING_RECONCILE_HOURS", "0")
	if got := ResolveBillingReconcileInterval(); got != 0 {
		t.Fatalf("interval = %v, want 0 (disabled)", got)
	}
	t.Setenv("GATEWAY_BILLING_RECONCILE_HOURS", "2")
	if got := ResolveBillingReconcileInterval(); got.Hours() != 2 {
		t.Fatalf("interval = %v, want 2h", got)
	}
}
