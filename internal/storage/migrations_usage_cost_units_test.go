package storage

import "testing"

// Rows written before the integer mirror existed must be repaired, otherwise the
// reconciliation ledger under-counts them forever.
func TestBackfillUsageLogCostUnitsRepairsLegacyRows(t *testing.T) {
	db, err := OpenDB(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	// Simulate a legacy row: float cost set, integer mirror still zero. Insert it
	// after migrations so the repair function is what has to fix it.
	if _, err := db.Exec(`INSERT INTO usage_logs (trace_id, model, cost, cost_units) VALUES ('tr-legacy', 'm', 1.5, 0)`); err != nil {
		t.Fatal(err)
	}
	// A row that is genuinely free must stay at zero.
	if _, err := db.Exec(`INSERT INTO usage_logs (trace_id, model, cost, cost_units) VALUES ('tr-free', 'm', 0, 0)`); err != nil {
		t.Fatal(err)
	}

	if err := backfillUsageLogCostUnits(db); err != nil {
		t.Fatalf("backfill: %v", err)
	}

	var legacy, free int64
	if err := db.QueryRow(`SELECT cost_units FROM usage_logs WHERE trace_id = 'tr-legacy'`).Scan(&legacy); err != nil {
		t.Fatal(err)
	}
	if want := ToUnits(1.5); legacy != want {
		t.Fatalf("legacy cost_units = %d, want %d", legacy, want)
	}
	if err := db.QueryRow(`SELECT cost_units FROM usage_logs WHERE trace_id = 'tr-free'`).Scan(&free); err != nil {
		t.Fatal(err)
	}
	if free != 0 {
		t.Fatalf("free row cost_units = %d, want 0", free)
	}

	// Re-running must not double-count.
	if err := backfillUsageLogCostUnits(db); err != nil {
		t.Fatalf("second backfill: %v", err)
	}
	if err := db.QueryRow(`SELECT cost_units FROM usage_logs WHERE trace_id = 'tr-legacy'`).Scan(&legacy); err != nil {
		t.Fatal(err)
	}
	if want := ToUnits(1.5); legacy != want {
		t.Fatalf("legacy cost_units after rerun = %d, want %d (idempotent)", legacy, want)
	}
}
