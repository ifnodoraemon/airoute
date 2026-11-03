package storage

import (
	"context"
	"database/sql"
	"errors"
	"testing"
)

func TestMCPEditsAndDeletionSurviveCatalogReads(t *testing.T) {
	db, err := OpenDB(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	repo := NewRepository(db)
	_, err = repo.ListMCPServers()
	if err != nil {
		t.Fatal(err)
	}
	server, err := repo.GetMCPServer("github-mcp")
	if err != nil {
		t.Fatal(err)
	}
	server.Endpoint, server.Status, server.EnvVars = `python "/path with spaces/server.py"`, "offline", `{"SERVICE_TOKEN":"configured"}`
	server.Tools = []string{"custom_echo"}
	if err := repo.SaveMCPServer(server); err != nil {
		t.Fatal(err)
	}
	if err := repo.SetMCPServerEnabled(server.ID, false); err != nil {
		t.Fatal(err)
	}
	fresh := NewRepository(db)
	stored, err := fresh.GetMCPServer(server.ID)
	if err != nil || stored.Endpoint != server.Endpoint || stored.Status != "offline" || stored.Enabled || stored.EnvVars != server.EnvVars || len(stored.Tools) != 1 || stored.Tools[0] != "custom_echo" {
		t.Fatalf("read overwrote saved configuration: %+v err=%v", stored, err)
	}
	if err := fresh.DeleteMCPServer(server.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := fresh.ListMCPServers(); err != nil {
		t.Fatal(err)
	}
	if _, err := NewRepository(db).GetMCPServer(server.ID); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("deleted server returned after read: %v", err)
	}
}

func TestSupplierCSVBatchRollsBackWriteFailure(t *testing.T) {
	db, err := OpenDB(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	repo := NewRepository(db)
	record := &SupplierCostRecord{Channel: "ch", Model: "existing", InputCostUnitsPerMillion: 10}
	if err := repo.UpsertSupplierCost(record); err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(`CREATE TRIGGER reject_supplier_row BEFORE INSERT ON supplier_costs WHEN NEW.model = 'fail' BEGIN SELECT RAISE(ABORT, 'injected write failure'); END`)
	if err != nil {
		t.Fatal(err)
	}
	err = repo.BatchUpsertSupplierCosts(context.Background(), []*SupplierCostRecord{
		{Channel: "ch", Model: "existing", InputCostUnitsPerMillion: 99},
		{Channel: "ch", Model: "fail", InputCostUnitsPerMillion: 12},
	})
	if err == nil {
		t.Fatal("injected write failure was not reported")
	}
	row, err := repo.GetSupplierCost("ch", "existing")
	if err != nil || row.InputCostUnitsPerMillion != 10 {
		t.Fatalf("failed import changed an earlier row: %+v err=%v", row, err)
	}
}
