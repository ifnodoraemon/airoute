package storage

import (
	"testing"
)

// A zero integer ledger must never resurrect stale display amounts.
func TestResolveMoneyPrefersLedger(t *testing.T) {
	legacy := &APIKeyRecord{Budget: 10, UsedCost: 2.5}
	legacy.resolveMoney()
	if legacy.Budget != 0 || legacy.UsedCost != 0 {
		t.Fatalf("zero units must override stale decimal amounts: %+v", legacy)
	}

	// Only the mirror is populated: the float is derived from it.
	modern := &APIKeyRecord{BudgetUnits: ToUnits(7), UsedCostUnits: ToUnits(1.25)}
	modern.resolveMoney()
	if modern.Budget != 7 || modern.UsedCost != 1.25 {
		t.Fatalf("expected floats from ledger, got budget=%v used=%v", modern.Budget, modern.UsedCost)
	}

	// Both populated: the ledger wins.
	both := &APIKeyRecord{
		Budget: 1, BudgetUnits: ToUnits(2),
		UsedCost: 0.1, UsedCostUnits: ToUnits(0.3),
	}
	both.resolveMoney()
	if both.Budget != 2 || both.UsedCost != 0.3 {
		t.Fatalf("expected ledger values to win, got budget=%v used=%v", both.Budget, both.UsedCost)
	}

	// Zero row stays zero in both representations.
	zero := &APIKeyRecord{}
	zero.resolveMoney()
	if zero.Budget != 0 || zero.UsedCost != 0 || zero.BudgetUnits != 0 || zero.UsedCostUnits != 0 {
		t.Fatalf("expected zero row to stay zero, got %+v", zero)
	}
}

func TestUserBalanceResolvesFromLedger(t *testing.T) {
	u := &UserRecord{BalanceUnits: ToUnits(12.345)}
	u.resolveBalanceUnits()
	if u.Balance != 12.345 {
		t.Fatalf("balance = %v, want 12.345", u.Balance)
	}

	legacy := &UserRecord{Balance: 3.5}
	legacy.resolveBalanceUnits()
	if legacy.Balance != 0 {
		t.Fatalf("zero units must override stale decimal balance: %+v", legacy)
	}
}
