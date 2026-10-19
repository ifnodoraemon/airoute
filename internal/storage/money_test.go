package storage

import (
	"path/filepath"
	"testing"
)

func TestToNanoRoundTrip(t *testing.T) {
	cases := []float64{0, 0.000000001, 0.000252, 1.5, 25.5, 1234.567890123}
	for _, in := range cases {
		nano := ToNano(in)
		got := FromNano(nano)
		if diff := got - in; diff > 1e-9 || diff < -1e-9 {
			t.Fatalf("round trip %v -> %d -> %v exceeds 1e-9", in, nano, got)
		}
	}
}

func TestToNanoHandlesInvalidValues(t *testing.T) {
	if got := ToNano(0); got != 0 {
		t.Fatalf("expected 0, got %d", got)
	}
}

func TestReconcileMoneyDetectsDrift(t *testing.T) {
	db, err := OpenDB(filepath.Join(t.TempDir(), "money.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer db.Close()
	repo := NewRepository(db)

	key := &APIKeyRecord{Key: "sk-reconcile-test-key-0001", TenantID: "t", Status: "active", Budget: 10}
	if err := repo.CreateAPIKey(key); err != nil {
		t.Fatalf("create key: %v", err)
	}
	// Ledger says 0.5 CNY was spent, but the key mirror stays at zero.
	if err := repo.RecordUsageLog(&UsageLogRecord{
		APIKey: key.Key, TenantID: "t", Model: "m", TotalTokens: 10, Cost: 0.5, StatusCode: 200,
	}); err != nil {
		t.Fatalf("record log: %v", err)
	}
	if err := repo.RecordUsageLog(&UsageLogRecord{
		APIKey: key.Key, TenantID: "t", Model: "m", TotalTokens: 10, Cost: 0.25, StatusCode: 200,
	}); err != nil {
		t.Fatalf("record log: %v", err)
	}
	// Simulate a mirror that only saw the first entry.
	if _, err := db.Exec(`UPDATE api_keys SET used_cost_nano = ? WHERE key = ?`, ToNano(0.5), key.Key); err != nil {
		t.Fatalf("simulate drift: %v", err)
	}

	drifts, err := repo.ReconcileMoney(10)
	if err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	if len(drifts) != 1 {
		t.Fatalf("expected 1 drift row, got %d: %+v", len(drifts), drifts)
	}
	if drifts[0].Difference != ToNano(0.25) {
		t.Fatalf("expected difference %d, got %d", ToNano(0.25), drifts[0].Difference)
	}
}
