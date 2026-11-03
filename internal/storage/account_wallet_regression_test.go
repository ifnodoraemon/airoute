package storage

import (
	"context"
	"database/sql"
	"errors"
	"math"
	"sync"
	"testing"
)

func accountWalletRepo(t *testing.T) *Repository {
	t.Helper()
	db, err := OpenDB(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return NewRepository(db)
}

func walletTestUser(t *testing.T, r *Repository, username, email string) *UserRecord {
	t.Helper()
	u := &UserRecord{Username: username, Email: email, PasswordHash: "synthetic", Balance: 10}
	if err := r.CreateUser(u); err != nil {
		t.Fatal(err)
	}
	return u
}

func TestRedemptionAndIdentityNeverMatchAnotherUsersEmail(t *testing.T) {
	r := accountWalletRepo(t)
	other := walletTestUser(t, r, "other-account", "customer@example.invalid")
	owner := walletTestUser(t, r, "customer@example.invalid", "owner@example.invalid")
	if u, err := r.GetUserByUsername(owner.Username); err != nil || u.ID != owner.ID {
		t.Fatalf("canonical identity resolved another account: %+v %v", u, err)
	}
	if u, err := r.GetUserByLogin(owner.Username); err != nil || u.ID != owner.ID {
		t.Fatalf("login did not prioritize exact username: %+v %v", u, err)
	}
	if u, err := r.GetUserByLogin(owner.Email); err != nil || u.ID != owner.ID {
		t.Fatalf("email login no longer works: %+v %v", u, err)
	}
	card := &RedemptionCodeRecord{Code: "CARD-OWNER", Amount: 19.99}
	if err := r.CreateRedemptionCodes([]*RedemptionCodeRecord{card}); err != nil {
		t.Fatal(err)
	}
	if _, err := r.RedeemCode(card.Code, owner.Username); err != nil {
		t.Fatal(err)
	}
	for id, want := range map[int64]float64{owner.ID: 29.99, other.ID: 10} {
		u, err := r.GetUserByID(id)
		if err != nil || u.BalanceUnits != ToUnits(want) {
			t.Fatalf("user %d balance: %+v %v", id, u, err)
		}
	}
	if err := r.DeleteRedemptionCode(card.ID); !errors.Is(err, ErrRedemptionUsed) {
		t.Fatalf("redeemed evidence was removed: %v", err)
	}
}

func TestRedemptionBatchRollsBackAndCannotDoubleCredit(t *testing.T) {
	r := accountWalletRepo(t)
	u := walletTestUser(t, r, "card-owner", "")
	if err := r.CreateRedemptionCodes([]*RedemptionCodeRecord{{Code: "duplicate", Amount: 20}, {Code: "duplicate", Amount: 20}}); err == nil {
		t.Fatal("duplicate batch unexpectedly succeeded")
	}
	if cards, err := r.ListRedemptionCodes(); err != nil || len(cards) != 0 {
		t.Fatalf("partial batch persisted: %+v %v", cards, err)
	}
	card := &RedemptionCodeRecord{Code: "single-use", Amount: 20}
	if err := r.CreateRedemptionCodes([]*RedemptionCodeRecord{card}); err != nil {
		t.Fatal(err)
	}
	results := make(chan error, 8)
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); _, err := r.RedeemCode(card.Code, u.Username); results <- err }()
	}
	wg.Wait()
	close(results)
	successes := 0
	for err := range results {
		if err == nil {
			successes++
		}
	}
	if successes != 1 {
		t.Fatalf("successful redemptions = %d, want 1", successes)
	}
	u, _ = r.GetUserByID(u.ID)
	if u.Balance != 30 {
		t.Fatalf("balance = %v, want 30", u.Balance)
	}
}

func TestDeleteUserRevokesAllKeysAndPreservesIdentity(t *testing.T) {
	r := accountWalletRepo(t)
	u := walletTestUser(t, r, "closed-account", "closed@example.invalid")
	key := &APIKeyRecord{Key: "sk-closed", UserID: u.ID, TenantID: u.Username}
	legacy := &APIKeyRecord{Key: "sk-legacy", UserID: u.ID, TenantID: u.Username}
	for _, k := range []*APIKeyRecord{key, legacy} {
		if err := r.CreateAPIKey(k); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := r.db.Exec(`UPDATE api_keys SET user_id = 0 WHERE id = ?`, legacy.ID); err != nil {
		t.Fatal(err)
	}
	if err := r.DeleteUser(u.Username); err != nil {
		t.Fatal(err)
	}
	for _, k := range []*APIKeyRecord{key, legacy} {
		current, err := r.GetAPIKeyByKeyContext(context.Background(), k.Key)
		if err != nil || current.Status != "deleted" {
			t.Fatalf("credential still active: %+v %v", current, err)
		}
	}
	if users, err := r.ListUsers(); err != nil || len(users) != 0 {
		t.Fatalf("closed account still listed: %+v %v", users, err)
	}
	if has, err := r.HasRegisteredAPIKeys(); err != nil || !has {
		t.Fatalf("auth gate lost history: %v %v", has, err)
	}
	closed, err := r.GetUserByUsername(u.Username)
	if err != nil || closed.Status != "deleted" || closed.Balance != 10 {
		t.Fatalf("lost account history: %+v %v", closed, err)
	}
	if err := r.CreateUser(&UserRecord{Username: u.Username}); err == nil {
		t.Fatal("deleted identity was reusable")
	}
	if err := r.UpdateUserStatus(u.Username, "active"); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("closed account could be revived: %v", err)
	}
	if err := r.DeleteUser(u.Username); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("repeated deletion reported success: %v", err)
	}
}

func TestDeleteUserRollsBackWhenKeyRevocationFails(t *testing.T) {
	r := accountWalletRepo(t)
	u := walletTestUser(t, r, "rollback-account", "")
	key := &APIKeyRecord{Key: "sk-rollback", UserID: u.ID}
	if err := r.CreateAPIKey(key); err != nil {
		t.Fatal(err)
	}
	if _, err := r.db.Exec(`CREATE TRIGGER reject_key_revocation BEFORE UPDATE OF status ON api_keys BEGIN SELECT RAISE(ABORT, 'injected failure'); END`); err != nil {
		t.Fatal(err)
	}
	if err := r.DeleteUser(u.Username); err == nil {
		t.Fatal("expected revocation failure")
	}
	u, _ = r.GetUserByID(u.ID)
	if u.Status != "active" {
		t.Fatal("account closed despite failed key revocation")
	}
	if _, err := r.GetAPIKeyByKey(key.Key); err != nil {
		t.Fatal("rollback lost key")
	}
}

func TestUserBalanceBoundsAndMissingAccount(t *testing.T) {
	r := accountWalletRepo(t)
	u := walletTestUser(t, r, "balance-account", "")
	for _, amount := range []float64{math.Inf(1), math.NaN(), 1e15, -1e15} {
		if !errors.Is(r.SetUserBalance(u.Username, amount), ErrBalanceOutOfRange) || !errors.Is(r.UpdateUserBalance(u.Username, amount), ErrBalanceOutOfRange) {
			t.Fatalf("accepted unsafe amount: %v", amount)
		}
	}
	if err := r.SetUserBalance(u.Username, MaxUserBalance); err != nil {
		t.Fatal(err)
	}
	if err := r.UpdateUserBalance(u.Username, .01); !errors.Is(err, ErrBalanceOutOfRange) {
		t.Fatalf("allowed overflow: %v", err)
	}
	if err := r.UpdateUserBalance(u.Username, -1); err != nil {
		t.Fatal(err)
	}
	if err := r.SetUserBalance("absent", 0); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("missing balance update succeeded: %v", err)
	}
	if err := r.UpdateUserBalance("absent", 10); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("missing delta update succeeded: %v", err)
	}
}
