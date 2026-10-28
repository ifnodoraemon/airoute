package storage

import (
	"database/sql"
	"errors"
	"testing"
)

// TestSupplierCostCRUD covers the price list that replaced the old cost/margin
// analytics: rows are keyed by (channel, model) and carry the contracted
// per-million-token rate only. Nothing here derives a request cost.
func TestSupplierCostCRUD(t *testing.T) {
	db, err := OpenDB(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	repo := NewRepository(db)

	rec := &SupplierCostRecord{Channel: " ch-a ", Model: " m-1 ", InputCostUnitsPerMillion: ToUnits(2), OutputCostUnitsPerMillion: ToUnits(4)}
	if err := repo.UpsertSupplierCost(rec); err != nil {
		t.Fatalf("upsert: %v", err)
	}
	if rec.Channel != "ch-a" || rec.Model != "m-1" || rec.Status != "active" {
		t.Fatalf("normalized record = %+v", rec)
	}

	stored, err := repo.GetSupplierCost("ch-a", "m-1")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if stored.InputCostUnitsPerMillion != ToUnits(2) || stored.OutputCostUnitsPerMillion != ToUnits(4) {
		t.Fatalf("stored rates = %+v", stored)
	}

	// Upserting the same pair updates in place instead of duplicating it.
	if err := repo.UpsertSupplierCost(&SupplierCostRecord{Channel: "ch-a", Model: "m-1", InputCostUnitsPerMillion: ToUnits(3), OutputCostUnitsPerMillion: ToUnits(6)}); err != nil {
		t.Fatalf("second upsert: %v", err)
	}
	rows, err := repo.ListSupplierCosts()
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(rows) != 1 || rows[0].InputCostUnitsPerMillion != ToUnits(3) {
		t.Fatalf("list after upsert = %+v", rows)
	}

	if err := repo.DeleteSupplierCost(rows[0].ID); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if _, err := repo.GetSupplierCost("ch-a", "m-1"); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("get after delete err = %v, want sql.ErrNoRows", err)
	}
	if err := repo.DeleteSupplierCost(rows[0].ID); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("delete missing err = %v, want sql.ErrNoRows", err)
	}
}

func TestSupplierCostValidation(t *testing.T) {
	cases := []struct {
		name string
		rec  *SupplierCostRecord
	}{
		{"missing channel", &SupplierCostRecord{Model: "m"}},
		{"missing model", &SupplierCostRecord{Channel: "ch"}},
		{"negative input", &SupplierCostRecord{Channel: "ch", Model: "m", InputCostUnitsPerMillion: -1}},
		{"unknown status", &SupplierCostRecord{Channel: "ch", Model: "m", Status: "retired"}},
	}
	for _, tc := range cases {
		if err := ValidateSupplierCost(tc.rec); err == nil {
			t.Fatalf("%s: expected validation error", tc.name)
		}
	}
	if err := ValidateSupplierCost(&SupplierCostRecord{Channel: "ch", Model: "m", Status: "disabled"}); err != nil {
		t.Fatalf("disabled status must be valid: %v", err)
	}
}
