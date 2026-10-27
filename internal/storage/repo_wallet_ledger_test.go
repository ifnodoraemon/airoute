package storage

import (
	"context"
	"testing"
)

func TestCreateWalletLedgerBackfillsOpeningIdempotently(t *testing.T) {
	db, err := OpenDB(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	repo := NewRepository(db)
	user := &UserRecord{
		Username: "ledger-user", Email: "ledger@example.com", PasswordHash: "x",
		Role: "user", Status: "active", Balance: 12.5, BalanceUnits: ToUnits(12.5), GroupName: "default",
	}
	if err := repo.CreateUser(user); err != nil {
		t.Fatal(err)
	}
	if err := CreateWalletLedger(db); err != nil {
		t.Fatal(err)
	}
	if err := CreateWalletLedger(db); err != nil {
		t.Fatalf("replay: %v", err)
	}
	entries, err := repo.ListWalletTransactions(user.ID, 10, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatalf("opening entries = %d, want 1", len(entries))
	}
	if entries[0].Type != WalletTypeOpening || entries[0].DeltaUnits != ToUnits(12.5) || entries[0].BalanceAfterUnits != ToUnits(12.5) {
		t.Fatalf("opening = %+v", entries[0])
	}
	sum, err := repo.WalletLedgerSumContext(context.Background(), user.ID)
	if err != nil {
		t.Fatal(err)
	}
	if sum != ToUnits(12.5) {
		t.Fatalf("ledger sum = %d, want %d", sum, ToUnits(12.5))
	}
}

func TestWalletTransactionsAppendAndDedupeByRef(t *testing.T) {
	db, err := OpenDB(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	repo := NewRepository(db)
	user := &UserRecord{
		Username: "tx-user", Email: "tx@example.com", PasswordHash: "x",
		Role: "user", Status: "active", Balance: 100, BalanceUnits: ToUnits(100), GroupName: "default",
	}
	if err := repo.CreateUser(user); err != nil {
		t.Fatal(err)
	}
	if err := CreateWalletLedger(db); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()

	recharge := &WalletTransactionRecord{UserID: user.ID, Type: WalletTypeRecharge, DeltaUnits: ToUnits(50), BalanceAfterUnits: ToUnits(150), RefID: "recharge:1", Operator: "stripe"}
	inserted, err := repo.RecordWalletTransactionContext(ctx, recharge)
	if err != nil || !inserted {
		t.Fatalf("recharge: inserted=%v err=%v", inserted, err)
	}
	inserted, err = repo.RecordWalletTransactionContext(ctx, recharge)
	if err != nil || inserted {
		t.Fatalf("duplicate ref must be a no-op: inserted=%v err=%v", inserted, err)
	}

	charge := &WalletTransactionRecord{UserID: user.ID, Type: WalletTypeCharge, DeltaUnits: -ToUnits(30), BalanceAfterUnits: ToUnits(120), RefID: "charge:1"}
	if inserted, err := repo.RecordWalletTransactionContext(ctx, charge); err != nil || !inserted {
		t.Fatalf("charge: inserted=%v err=%v", inserted, err)
	}

	// Empty refs are allowed and not deduped (manual adjustments).
	for i := 0; i < 2; i++ {
		entry := &WalletTransactionRecord{UserID: user.ID, Type: WalletTypeAdjustment, DeltaUnits: ToUnits(1), BalanceAfterUnits: ToUnits(121), Operator: "ops"}
		if inserted, err := repo.RecordWalletTransactionContext(ctx, entry); err != nil || !inserted {
			t.Fatalf("adjustment %d: inserted=%v err=%v", i, inserted, err)
		}
	}

	entries, err := repo.ListWalletTransactions(user.ID, 10, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 5 { // opening + recharge + charge + 2 adjustments
		t.Fatalf("entries = %d: %+v", len(entries), entries)
	}
	sum, err := repo.WalletLedgerSumContext(ctx, user.ID)
	if err != nil {
		t.Fatal(err)
	}
	want := ToUnits(100) + ToUnits(50) - ToUnits(30) + ToUnits(2)
	if sum != want {
		t.Fatalf("sum = %d, want %d", sum, want)
	}
	latest, err := repo.LatestWalletBalanceContext(ctx, user.ID)
	if err != nil {
		t.Fatal(err)
	}
	if latest != ToUnits(121) {
		t.Fatalf("latest balance = %d", latest)
	}

	if _, err := repo.RecordWalletTransactionContext(ctx, &WalletTransactionRecord{UserID: user.ID, Type: "bogus", DeltaUnits: 1}); err == nil {
		t.Fatal("invalid type must be rejected")
	}
	if _, err := repo.RecordWalletTransactionContext(ctx, &WalletTransactionRecord{UserID: 0, Type: WalletTypeAdjustment}); err == nil {
		t.Fatal("missing user must be rejected")
	}
}
