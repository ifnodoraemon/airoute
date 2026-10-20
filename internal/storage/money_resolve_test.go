package storage

import (
	"testing"
)

// TestResolveMoneyPrefersLedger covers the migration window where some rows only
// have the legacy float columns and others only have the integer mirror.
func TestResolveMoneyPrefersLedger(t *testing.T) {
	// Only the float side is populated: the mirror is derived from it.
	legacy := &APIKeyRecord{Budget: 10, UsedCost: 2.5}
	legacy.resolveMoney()
	if legacy.BudgetUnits != ToUnits(10) || legacy.UsedCostUnits != ToUnits(2.5) {
		t.Fatalf("expected mirrored integer-unit values, got budget=%d used=%d", legacy.BudgetUnits, legacy.UsedCostUnits)
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
	if legacy.BalanceUnits != ToUnits(3.5) {
		t.Fatalf("balance_units = %d, want %d", legacy.BalanceUnits, ToUnits(3.5))
	}
}
