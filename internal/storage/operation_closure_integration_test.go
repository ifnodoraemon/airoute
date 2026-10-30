//go:build integration

package storage

import (
	"context"
	"fmt"
	"testing"
	"time"
)

func TestPostgresSupplierImportRollsBackWriteFailure(t *testing.T) {
	db, err := OpenDB(testPostgresURL(t))
	if err != nil {
		t.Fatal(err)
	}
	repo := NewRepository(db)
	suffix := itSuffix()
	channel, rejected, constraint := "import-"+suffix, "rejected-"+suffix, "import_reject_"+suffix
	t.Cleanup(func() {
		_, _ = db.Exec(`ALTER TABLE supplier_costs DROP CONSTRAINT IF EXISTS ` + constraint)
		_, _ = db.Exec(`DELETE FROM supplier_costs WHERE channel = ?`, channel)
		_ = db.Close()
	})
	if err := repo.UpsertSupplierCost(&SupplierCostRecord{Channel: channel, Model: "existing", InputCostUnitsPerMillion: 10}); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(fmt.Sprintf(`ALTER TABLE supplier_costs ADD CONSTRAINT %s CHECK (model <> '%s')`, constraint, rejected)); err != nil {
		t.Fatal(err)
	}
	err = repo.BatchUpsertSupplierCosts(context.Background(), []*SupplierCostRecord{
		{Channel: channel, Model: "existing", InputCostUnitsPerMillion: 99},
		{Channel: channel, Model: rejected, InputCostUnitsPerMillion: 5},
	})
	if err == nil {
		t.Fatal("expected PostgreSQL constraint violation")
	}
	stored, err := repo.GetSupplierCost(channel, "existing")
	if err != nil || stored.InputCostUnitsPerMillion != 10 {
		t.Fatalf("failed batch changed original price: %+v err=%v", stored, err)
	}
}

func TestPostgresStartupWaitsForPeerMigration(t *testing.T) {
	dsn := testPostgresURL(t)
	db, err := OpenDB(dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	conn, err := db.acquirePostgresMigrationLock(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer releasePostgresMigrationLock(conn)
	result := make(chan error, 1)
	go func() {
		peer, err := OpenDB(dsn)
		if peer != nil {
			_ = peer.Close()
		}
		result <- err
	}()
	// The old blocking pg_advisory_lock inherited lock_timeout=500ms and exited
	// here. A competing replica must still be waiting after that budget expires.
	select {
	case err := <-result:
		t.Fatalf("peer startup returned before migration lock release: %v", err)
	case <-time.After(time.Second):
	}
	releasePostgresMigrationLock(conn)
	select {
	case err := <-result:
		if err != nil {
			t.Fatalf("peer did not start after release: %v", err)
		}
	case <-ctx.Done():
		t.Fatal("peer startup never resumed")
	}
}
