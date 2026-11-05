//go:build integration

package storage

import "testing"

func TestPostgresSupplierQuoteRates(t *testing.T) {
	db, err := OpenDB(testPostgresURL(t))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	repo := NewRepository(db)
	name := "quote-" + itSuffix()
	t.Cleanup(func() { _, _ = db.Exec(`DELETE FROM supplier_costs WHERE channel=?`, name) })
	zero, write := int64(0), int64(1)
	rec := &SupplierCostRecord{Channel: name, Model: "physical", Currency: "USD", InputCostUnitsPerMillion: 1000000001, OutputCostUnitsPerMillion: 2000000000, CacheReadCostUnitsPerMillion: &zero, CacheWriteCostUnitsPerMillion: &write}
	if err := repo.UpsertSupplierCost(rec); err != nil {
		t.Fatal(err)
	}
	if rec.Currency != "USD" || rec.CacheReadCostUnitsPerMillion == nil || *rec.CacheReadCostUnitsPerMillion != 0 || *rec.CacheWriteCostUnitsPerMillion != 1 {
		t.Fatalf("token quote=%+v", rec)
	}
	rec.BillingMode, rec.FixedCostUnits = "request", &write
	if err := repo.UpsertSupplierCost(rec); err != nil {
		t.Fatal(err)
	}
	if rec.FixedCostUnits == nil || *rec.FixedCostUnits != 1 || rec.CacheReadCostUnitsPerMillion != nil {
		t.Fatalf("request quote=%+v", rec)
	}
}
