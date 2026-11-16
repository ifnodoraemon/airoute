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

	// The key claims usage that no ledger entry backs. Wallet funds are read
	// directly from integer units, without a separate floating-point mirror.
	if _, err := r.db.Exec(`UPDATE api_keys SET used_cost_units = ? WHERE key = ?`, ToUnits(5), key.Key); err != nil {
		t.Fatal(err)
	}
	if _, err := r.db.Exec(`UPDATE users SET balance_units = ? WHERE id = ?`, ToUnits(10), u.ID); err != nil {
		t.Fatal(err)
	}

	worker := &BillingReconciler{repo: r, stop: make(chan struct{})}
	worker.runOnce()

	if got, want := telemetry.GlobalMetrics.BillingLedgerDriftUnits.Load(), ToUnits(5); got != want {
		t.Fatalf("ledger drift = %d units, want %d", got, want)
	}
	if got, want := telemetry.GlobalMetrics.BillingWalletTotalUnits.Load(), ToUnits(10); got != want {
		t.Fatalf("wallet total = %d units, want %d", got, want)
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
