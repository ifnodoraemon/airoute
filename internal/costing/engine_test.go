package costing

import (
	"testing"

	"github.com/ifnodoraemon/airoute/internal/storage"
)

func TestCostBasisUnitsAndCacheInvalidation(t *testing.T) {
	db, err := storage.OpenDB(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	repo := storage.NewRepository(db)
	rec := &storage.SupplierCostRecord{
		Channel: "ch-a", Model: "m-1",
		InputCostUnitsPerMillion:  storage.ToUnits(2),
		OutputCostUnitsPerMillion: storage.ToUnits(4),
	}
	if err := repo.UpsertSupplierCost(rec); err != nil {
		t.Fatal(err)
	}
	engine := InitGlobal(repo)

	units, found := engine.CostBasisUnits("ch-a", "m-1", 1_000_000, 0)
	if !found || units != storage.ToUnits(2) {
		t.Fatalf("input cost = %d found=%v", units, found)
	}
	units, found = engine.CostBasisUnits("ch-a", "m-1", 0, 1_000_000)
	if !found || units != storage.ToUnits(4) {
		t.Fatalf("output cost = %d found=%v", units, found)
	}
	if _, found := engine.CostBasisUnits("ch-unknown", "m-1", 1000, 0); found {
		t.Fatal("unknown channel must not report a cost")
	}

	// Cached price survives a delete until the admin write invalidates it.
	if err := repo.DeleteSupplierCost(rec.ID); err != nil {
		t.Fatal(err)
	}
	if _, found := engine.CostBasisUnits("ch-a", "m-1", 1_000_000, 0); !found {
		t.Fatal("expected the cached price before invalidation")
	}
	engine.Invalidate()
	if _, found := engine.CostBasisUnits("ch-a", "m-1", 1_000_000, 0); found {
		t.Fatal("price still served after invalidation")
	}
}
