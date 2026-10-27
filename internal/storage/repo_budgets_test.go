package storage

import (
	"testing"

	"github.com/ifnodoraemon/airoute/internal/budget"
)

func newBudgetRepo(t *testing.T) (*Repository, *DB) {
	t.Helper()
	db, err := OpenDB(":memory:")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := addBudgets(db); err != nil {
		t.Fatalf("add budgets schema: %v", err)
	}
	return NewRepository(db), db
}

func TestAddBudgetsIsIdempotent(t *testing.T) {
	repo, db := newBudgetRepo(t)
	_ = repo
	if err := addBudgets(db); err != nil {
		t.Fatalf("second addBudgets: %v", err)
	}
	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM budgets`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("budgets = %d", count)
	}
}

func TestBudgetUpsertPreservesUsageAndValidates(t *testing.T) {
	repo, _ := newBudgetRepo(t)

	rec := &BudgetRecord{ScopeType: "org", ScopeID: 10, Period: "monthly", AmountUnits: 1000}
	if err := repo.UpsertBudget(rec); err != nil {
		t.Fatalf("create budget: %v", err)
	}
	if rec.ID <= 0 || rec.Action != "notify" || rec.WarnPercent != 80 || rec.Status != "active" {
		t.Fatalf("defaults not applied: %+v", rec)
	}
	if err := repo.IncrementBudgetUsage(rec.ID, 400); err != nil {
		t.Fatal(err)
	}

	update := &BudgetRecord{ScopeType: "org", ScopeID: 10, Period: "monthly", AmountUnits: 2000, Action: "disable", WarnPercent: 90}
	if err := repo.UpsertBudget(update); err != nil {
		t.Fatalf("update budget: %v", err)
	}
	stored, err := repo.GetBudget(rec.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.AmountUnits != 2000 || stored.Action != "disable" || stored.WarnPercent != 90 {
		t.Fatalf("update not applied: %+v", stored)
	}
	if stored.UsedUnits != 400 {
		t.Fatalf("used_units was reset by an edit: %+v", stored)
	}

	for _, invalid := range []*BudgetRecord{
		{ScopeType: "team", ScopeID: 1, Period: "monthly", AmountUnits: 10},
		{ScopeType: "key", ScopeID: 0, Period: "monthly", AmountUnits: 10},
		{ScopeType: "key", ScopeID: 1, Period: "weekly", AmountUnits: 10},
		{ScopeType: "key", ScopeID: 1, Period: "monthly", AmountUnits: 0},
		{ScopeType: "key", ScopeID: 1, Period: "monthly", AmountUnits: 10, Action: "degrade"},
	} {
		if err := repo.UpsertBudget(invalid); err == nil {
			t.Fatalf("expected validation error for %+v", invalid)
		}
	}
}

func TestBudgetEventsAreDedupedPerPeriod(t *testing.T) {
	repo, _ := newBudgetRepo(t)
	rec := &BudgetRecord{ScopeType: "key", ScopeID: 9, Period: "daily", AmountUnits: 1000}
	if err := repo.UpsertBudget(rec); err != nil {
		t.Fatal(err)
	}

	inserted, err := repo.RecordBudgetEvent(rec.ID, 80, "2026-10-27", 800, 1000, "notify")
	if err != nil || !inserted {
		t.Fatalf("first event: inserted=%v err=%v", inserted, err)
	}
	inserted, err = repo.RecordBudgetEvent(rec.ID, 80, "2026-10-27", 900, 1000, "notify")
	if err != nil || inserted {
		t.Fatalf("duplicate event: inserted=%v err=%v", inserted, err)
	}
	inserted, err = repo.RecordBudgetEvent(rec.ID, 100, "2026-10-27", 1000, 1000, "disable")
	if err != nil || !inserted {
		t.Fatalf("different threshold: inserted=%v err=%v", inserted, err)
	}
	inserted, err = repo.RecordBudgetEvent(rec.ID, 80, "2026-10-28", 800, 1000, "notify")
	if err != nil || !inserted {
		t.Fatalf("next period should notify again: inserted=%v err=%v", inserted, err)
	}

	events, err := repo.ListBudgetEvents(rec.ID, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 3 {
		t.Fatalf("events = %d, want 3", len(events))
	}
}

func TestEffectiveBudgetRulesPrecedence(t *testing.T) {
	repo, _ := newBudgetRepo(t)

	keyRule := &BudgetRecord{ScopeType: "key", ScopeID: 9, Period: "daily", AmountUnits: 100}
	userRule := &BudgetRecord{ScopeType: "user", ScopeID: 7, Period: "monthly", AmountUnits: 300}
	orgNear := &BudgetRecord{ScopeType: "org", ScopeID: 30, Period: "monthly", AmountUnits: 500}
	orgRoot := &BudgetRecord{ScopeType: "org", ScopeID: 10, Period: "total", AmountUnits: 2000}
	for _, rec := range []*BudgetRecord{keyRule, userRule, orgNear, orgRoot} {
		if err := repo.UpsertBudget(rec); err != nil {
			t.Fatalf("upsert %+v: %v", rec, err)
		}
	}

	rules, err := repo.EffectiveBudgetRules(9, 7, []int64{30, 20, 10})
	if err != nil {
		t.Fatalf("effective rules: %v", err)
	}
	if len(rules) != 3 {
		t.Fatalf("rules = %+v", rules)
	}
	if rules[0].ScopeType != budget.ScopeKey || rules[0].Period != budget.PeriodDaily {
		t.Fatalf("daily should be the key budget: %+v", rules[0])
	}
	// Monthly: the user rule (300) beats the department (500) because user is
	// more specific.
	if rules[1].ScopeType != budget.ScopeUser || rules[1].AmountUnits != 300 {
		t.Fatalf("monthly should be the user budget: %+v", rules[1])
	}
	if rules[2].ScopeType != budget.ScopeOrg || rules[2].AmountUnits != 2000 {
		t.Fatalf("total should be the unit budget: %+v", rules[2])
	}

	// A disabled rule is not effective.
	if err := repo.SetBudgetStatus(keyRule.ID, "disabled"); err != nil {
		t.Fatal(err)
	}
	rules, err = repo.EffectiveBudgetRules(9, 7, []int64{30, 20, 10})
	if err != nil {
		t.Fatal(err)
	}
	for _, rule := range rules {
		if rule.ScopeType == budget.ScopeKey {
			t.Fatalf("disabled key rule leaked into effective rules: %+v", rules)
		}
	}
}
