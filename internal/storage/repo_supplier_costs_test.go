package storage

import (
	"context"
	"database/sql"
	"testing"
)

func TestUsageLogPersistsNullableCostBasis(t *testing.T) {
	db, err := OpenDB(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	repo := NewRepository(db)

	units := ToUnits(2)
	priced := &UsageLogRecord{
		APIKey: "sk-test", TenantID: "t", Model: "m-1", Channel: "ch-a",
		PromptTokens: 100, TotalTokens: 100, CostBasisUnits: &units, StatusCode: 200,
	}
	unpriced := &UsageLogRecord{
		APIKey: "sk-test", TenantID: "t", Model: "m-2", Channel: "ch-b",
		PromptTokens: 100, TotalTokens: 100, StatusCode: 200,
	}
	if err := repo.insertUsageLogsContext(context.Background(), []*UsageLogRecord{priced, unpriced}); err != nil {
		t.Fatal(err)
	}

	var pricedUnits sql.NullInt64
	if err := db.QueryRow(`SELECT cost_basis_units FROM usage_logs WHERE billing_id = ?`, priced.BillingID).Scan(&pricedUnits); err != nil {
		t.Fatal(err)
	}
	if !pricedUnits.Valid || pricedUnits.Int64 != units {
		t.Fatalf("priced cost basis = %+v", pricedUnits)
	}
	var unpricedUnits sql.NullInt64
	if err := db.QueryRow(`SELECT cost_basis_units FROM usage_logs WHERE billing_id = ?`, unpriced.BillingID).Scan(&unpricedUnits); err != nil {
		t.Fatal(err)
	}
	if unpricedUnits.Valid {
		t.Fatalf("unknown cost must stay NULL, got %d", unpricedUnits.Int64)
	}
}
