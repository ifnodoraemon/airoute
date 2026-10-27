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

func TestRedeemCodeWritesRedemptionLedger(t *testing.T) {
	db, err := OpenDB(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	repo := NewRepository(db)
	user := &UserRecord{Username: "redeem-user", Email: "redeem@example.com", PasswordHash: "x", Role: "user", Status: "active", Balance: 100, BalanceUnits: ToUnits(100), GroupName: "default"}
	if err := repo.CreateUser(user); err != nil {
		t.Fatal(err)
	}
	if err := CreateWalletLedger(db); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO redemption_codes (code, name, amount, status) VALUES ('CODE-1', 'test', 10, 'active')`); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.RedeemCode("CODE-1", user.Username); err != nil {
		t.Fatalf("redeem: %v", err)
	}

	entries, err := repo.ListWalletTransactions(user.ID, 10, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 2 || entries[0].Type != WalletTypeRedemption {
		t.Fatalf("entries = %+v", entries)
	}
	if entries[0].DeltaUnits != ToUnits(10) || entries[0].BalanceAfterUnits != ToUnits(110) {
		t.Fatalf("redemption entry = %+v", entries[0])
	}
	sum, err := repo.WalletLedgerSumContext(context.Background(), user.ID)
	if err != nil {
		t.Fatal(err)
	}
	if sum != ToUnits(110) {
		t.Fatalf("ledger sum = %d", sum)
	}
}

func TestCompleteRechargeOrderWritesRechargeLedgerOnce(t *testing.T) {
	db, err := OpenDB(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	repo := NewRepository(db)
	user := &UserRecord{Username: "recharge-user", Email: "recharge@example.com", PasswordHash: "x", Role: "user", Status: "active", Balance: 100, BalanceUnits: ToUnits(100), GroupName: "default"}
	if err := repo.CreateUser(user); err != nil {
		t.Fatal(err)
	}
	if err := CreateWalletLedger(db); err != nil {
		t.Fatal(err)
	}
	order := &RechargeOrderRecord{OrderNo: "order-1", Username: user.Username, Amount: 25, Currency: "CNY", Channel: "stripe", Status: "pending"}
	if err := repo.CreateRechargeOrder(order); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.CompleteRechargeOrder("order-1"); err != nil {
		t.Fatalf("complete: %v", err)
	}
	// Webhook retries return the paid order without crediting again.
	if _, err := repo.CompleteRechargeOrder("order-1"); err != nil {
		t.Fatalf("repeat complete: %v", err)
	}

	entries, err := repo.ListWalletTransactions(user.ID, 10, 0)
	if err != nil {
		t.Fatal(err)
	}
	recharges := 0
	for _, entry := range entries {
		if entry.Type == WalletTypeRecharge {
			recharges++
			if entry.DeltaUnits != ToUnits(25) || entry.BalanceAfterUnits != ToUnits(125) {
				t.Fatalf("recharge entry = %+v", entry)
			}
		}
	}
	if recharges != 1 {
		t.Fatalf("recharge entries = %d", recharges)
	}
}

func TestDeductWalletWritesChargeLedger(t *testing.T) {
	db, err := OpenDB(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	repo := NewRepository(db)
	user := &UserRecord{Username: "charge-user", Email: "charge@example.com", PasswordHash: "x", Role: "user", Status: "active", Balance: 100, BalanceUnits: ToUnits(100), GroupName: "default"}
	if err := repo.CreateUser(user); err != nil {
		t.Fatal(err)
	}
	if err := CreateWalletLedger(db); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	tx, err := db.BeginTx(ctx)
	if err != nil {
		t.Fatal(err)
	}
	applied, balance, err := repo.deductWallet(tx, "id = ? AND role != 'admin'", []any{user.ID}, 3)
	if err != nil {
		t.Fatal(err)
	}
	if !applied || balance != ToUnits(97) {
		t.Fatalf("deduct applied=%v balance=%d", applied, balance)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}

	entries, err := repo.ListWalletTransactions(user.ID, 10, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 2 || entries[0].Type != WalletTypeCharge {
		t.Fatalf("entries = %+v", entries)
	}
	if entries[0].DeltaUnits != -ToUnits(3) || entries[0].BalanceAfterUnits != ToUnits(97) {
		t.Fatalf("charge entry = %+v", entries[0])
	}
	sum, err := repo.WalletLedgerSumContext(ctx, user.ID)
	if err != nil {
		t.Fatal(err)
	}
	if sum != ToUnits(97) {
		t.Fatalf("ledger sum = %d", sum)
	}
}
