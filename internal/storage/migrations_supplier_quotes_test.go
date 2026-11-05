package storage

import "testing"

func TestSupplierQuoteLegacyMigrationAndNullableRates(t *testing.T) {
	db, err := OpenDB(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	_, err = db.Exec(`DROP TABLE supplier_costs`)
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(`CREATE TABLE supplier_costs (id INTEGER PRIMARY KEY AUTOINCREMENT, channel TEXT NOT NULL, model TEXT NOT NULL, input_cost_units_per_million BIGINT NOT NULL, output_cost_units_per_million BIGINT NOT NULL, status TEXT NOT NULL DEFAULT 'active', created_at DATETIME DEFAULT CURRENT_TIMESTAMP, updated_at DATETIME DEFAULT CURRENT_TIMESTAMP, UNIQUE(channel,model))`)
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(`INSERT INTO supplier_costs(channel,model,input_cost_units_per_million,output_cost_units_per_million) VALUES('legacy','model',12,34)`)
	if err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if err := addSupplierQuoteFields(db); err != nil {
			t.Fatal(err)
		}
	}
	repo := NewRepository(db)
	rec, err := repo.GetSupplierCost("legacy", "model")
	if err != nil {
		t.Fatal(err)
	}
	if rec.Currency != "CNY" || rec.BillingMode != "token" || rec.CacheReadCostUnitsPerMillion != nil || rec.FixedCostUnits != nil || rec.InputCostUnitsPerMillion != 12 {
		t.Fatalf("legacy changed: %+v", rec)
	}
	zero, write := int64(0), int64(123456789)
	rec.Currency, rec.CacheReadCostUnitsPerMillion, rec.CacheWriteCostUnitsPerMillion = " usd ", &zero, &write
	if err := repo.UpsertSupplierCost(rec); err != nil {
		t.Fatal(err)
	}
	if rec.Currency != "USD" || rec.CacheReadCostUnitsPerMillion == nil || *rec.CacheReadCostUnitsPerMillion != 0 || *rec.CacheWriteCostUnitsPerMillion != write {
		t.Fatalf("nullable rates lost: %+v", rec)
	}
	rec.BillingMode, rec.FixedCostUnits = "request", &zero
	if err := repo.UpsertSupplierCost(rec); err != nil {
		t.Fatal(err)
	}
	if rec.CacheReadCostUnitsPerMillion != nil || rec.CacheWriteCostUnitsPerMillion != nil || rec.FixedCostUnits == nil || *rec.FixedCostUnits != 0 || rec.InputCostUnitsPerMillion != 0 {
		t.Fatalf("mode switch not normalized: %+v", rec)
	}
	rec.BillingMode = "token"
	if err := repo.UpsertSupplierCost(rec); err != nil {
		t.Fatal(err)
	}
	if rec.FixedCostUnits != nil {
		t.Fatal("token quote retained fixed rate")
	}
}
