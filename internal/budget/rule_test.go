package budget

import (
	"testing"
	"time"
)

func shanghaiTime(year int, month time.Month, day, hour int) time.Time {
	return time.Date(year, month, day, hour, 0, 0, 0, time.FixedZone("CST", 8*3600))
}

func TestPeriodKeyAndStartUseBusinessTimezone(t *testing.T) {
	at := shanghaiTime(2026, time.October, 27, 1)
	if got := PeriodDaily.Key(at); got != "2026-10-27" {
		t.Fatalf("daily key = %q", got)
	}
	if got := PeriodMonthly.Key(at); got != "2026-10" {
		t.Fatalf("monthly key = %q", got)
	}
	if got := PeriodTotal.Key(at); got != "total" {
		t.Fatalf("total key = %q", got)
	}

	// 2026-10-27 23:30 UTC is already 10-28 07:30 in Shanghai.
	utcLate := time.Date(2026, time.October, 27, 23, 30, 0, 0, time.UTC)
	if got := PeriodDaily.Key(utcLate); got != "2026-10-28" {
		t.Fatalf("daily key across UTC midnight = %q, want 2026-10-28", got)
	}

	start := PeriodMonthly.Start(at)
	want := time.Date(2026, time.October, 1, 0, 0, 0, 0, start.Location())
	if !start.Equal(want) {
		t.Fatalf("monthly start = %v, want %v", start, want)
	}
	if !PeriodTotal.Start(at).IsZero() {
		t.Fatal("total period must not have a start")
	}

	if _, err := ParsePeriod("weekly"); err == nil {
		t.Fatal("expected invalid period")
	}
	if p, err := ParsePeriod("monthly"); err != nil || p != PeriodMonthly {
		t.Fatalf("parse monthly: %v %v", p, err)
	}
}

func TestRuleValidate(t *testing.T) {
	valid := Rule{ScopeType: ScopeKey, ScopeID: 1, Period: PeriodMonthly, AmountUnits: 100, Action: ActionNotify}
	if err := valid.Validate(); err != nil {
		t.Fatalf("valid rule rejected: %v", err)
	}
	cases := []Rule{
		{ScopeType: "team", ScopeID: 1, Period: PeriodMonthly, AmountUnits: 100, Action: ActionNotify},
		{ScopeType: ScopeKey, ScopeID: 0, Period: PeriodMonthly, AmountUnits: 100, Action: ActionNotify},
		{ScopeType: ScopeKey, ScopeID: 1, Period: "weekly", AmountUnits: 100, Action: ActionNotify},
		{ScopeType: ScopeKey, ScopeID: 1, Period: PeriodMonthly, AmountUnits: 0, Action: ActionNotify},
		{ScopeType: ScopeKey, ScopeID: 1, Period: PeriodMonthly, AmountUnits: 100, Action: "throttle"},
		{ScopeType: ScopeKey, ScopeID: 1, Period: PeriodMonthly, AmountUnits: 100, Action: ActionNotify, WarnPercent: 101},
	}
	for i, rule := range cases {
		if err := rule.Validate(); err == nil {
			t.Fatalf("case %d: expected validation error", i)
		}
	}
}

func TestEvaluateThresholdsAndActions(t *testing.T) {
	rule := Rule{ScopeType: ScopeOrg, ScopeID: 1, Period: PeriodMonthly, AmountUnits: 1000, Action: ActionDisable, WarnPercent: 80}

	if got := Evaluate(rule, 100); !got.Allowed || got.ShouldAlert || got.Threshold != 0 {
		t.Fatalf("10%%: %+v", got)
	}
	if got := Evaluate(rule, 500); !got.Allowed || got.ShouldAlert || got.Threshold != 50 {
		t.Fatalf("50%%: %+v", got)
	}
	if got := Evaluate(rule, 800); !got.Allowed || !got.ShouldAlert || got.Threshold != 80 {
		t.Fatalf("80%%: %+v", got)
	}
	if got := Evaluate(rule, 1000); got.Allowed || !got.ShouldAlert || got.Threshold != 100 || got.Reason != "budget_exhausted" {
		t.Fatalf("100%% disable: %+v", got)
	}

	notify := rule
	notify.Action = ActionNotify
	if got := Evaluate(notify, 1200); !got.Allowed || !got.ShouldAlert || got.Threshold != 100 {
		t.Fatalf("notify at 120%%: %+v", got)
	}

	// The larger of persisted and live usage wins.
	stored := Rule{ScopeType: ScopeKey, ScopeID: 1, Period: PeriodDaily, AmountUnits: 1000, UsedUnits: 900, Action: ActionDisable}
	if got := Evaluate(stored, 100); got.WatermarkPercent != 90 || got.Threshold != 80 {
		t.Fatalf("stored usage ignored: %+v", got)
	}
	if got := Evaluate(stored, 1100); got.Allowed || got.Threshold != 100 {
		t.Fatalf("live usage ignored: %+v", got)
	}

	// Zero amount means "no budget" rather than "everything is over budget".
	if got := Evaluate(Rule{AmountUnits: 0}, 100); !got.Allowed || got.ShouldAlert {
		t.Fatalf("unconfigured budget: %+v", got)
	}

	// A custom warn percent moves the alert threshold.
	custom := Rule{ScopeType: ScopeKey, ScopeID: 1, Period: PeriodDaily, AmountUnits: 1000, Action: ActionNotify, WarnPercent: 60}
	if got := Evaluate(custom, 650); !got.ShouldAlert || got.Threshold != 60 {
		t.Fatalf("custom warn: %+v", got)
	}
}

func TestResolvePrefersKeyThenUserThenNearestOrg(t *testing.T) {
	// Org chain: project (id 30, rank 0), department (20, rank 1), unit (10, rank 2).
	orgChain := []int64{30, 20, 10}
	candidates := []Rule{
		{ScopeType: ScopeOrg, ScopeID: 10, Period: PeriodMonthly, AmountUnits: 1000},
		{ScopeType: ScopeOrg, ScopeID: 20, Period: PeriodMonthly, AmountUnits: 500},
		{ScopeType: ScopeOrg, ScopeID: 10, Period: PeriodDaily, AmountUnits: 25},
		{ScopeType: ScopeOrg, ScopeID: 20, Period: PeriodDaily, AmountUnits: 50},
		{ScopeType: ScopeUser, ScopeID: 7, Period: PeriodMonthly, AmountUnits: 300},
		{ScopeType: ScopeKey, ScopeID: 9, Period: PeriodDaily, AmountUnits: 100},
		// Wrong key/user must be ignored.
		{ScopeType: ScopeKey, ScopeID: 99, Period: PeriodTotal, AmountUnits: 999},
		{ScopeType: ScopeUser, ScopeID: 77, Period: PeriodTotal, AmountUnits: 999},
		// Nearest org for the total period: unit only.
		{ScopeType: ScopeOrg, ScopeID: 10, Period: PeriodTotal, AmountUnits: 2000},
	}

	effective := Resolve(candidates, 9, 7, orgChain)
	if len(effective) != 3 {
		t.Fatalf("effective = %+v", effective)
	}
	// Daily: the key budget wins.
	if effective[0].ScopeType != ScopeKey || effective[0].ScopeID != 9 || effective[0].Period != PeriodDaily {
		t.Fatalf("daily should be the key budget: %+v", effective[0])
	}
	// Monthly: user budget (300) overrides department/unit.
	if effective[1].ScopeType != ScopeUser || effective[1].ScopeID != 7 {
		t.Fatalf("monthly should be the user budget: %+v", effective[1])
	}
	// Total: only unit has one.
	if effective[2].ScopeType != ScopeOrg || effective[2].ScopeID != 10 || effective[2].Period != PeriodTotal {
		t.Fatalf("total should be the unit budget: %+v", effective[2])
	}

	// A key budget for one period must not hide user/org budgets for the others.
	if effective[0].Period != PeriodDaily {
		t.Fatalf("period order = %+v", effective)
	}

	// Without a key budget, daily falls back to the nearest configured org
	// (department 20), not the unit 10.
	fallback := Resolve(candidates, 999, 7, orgChain)
	if len(fallback) != 3 || fallback[0].ScopeType != ScopeOrg || fallback[0].ScopeID != 20 {
		t.Fatalf("daily org fallback = %+v", fallback)
	}
}

func TestResolveKeyOverridesAllPeriods(t *testing.T) {
	candidates := []Rule{
		{ScopeType: ScopeKey, ScopeID: 9, Period: PeriodDaily, AmountUnits: 10},
		{ScopeType: ScopeKey, ScopeID: 9, Period: PeriodMonthly, AmountUnits: 20},
		{ScopeType: ScopeKey, ScopeID: 9, Period: PeriodTotal, AmountUnits: 30},
		{ScopeType: ScopeUser, ScopeID: 7, Period: PeriodDaily, AmountUnits: 100},
		{ScopeType: ScopeOrg, ScopeID: 10, Period: PeriodDaily, AmountUnits: 200},
	}
	effective := Resolve(candidates, 9, 7, []int64{10})
	if len(effective) != 3 {
		t.Fatalf("effective = %+v", effective)
	}
	for _, rule := range effective {
		if rule.ScopeType != ScopeKey || rule.ScopeID != 9 {
			t.Fatalf("key must win every period: %+v", effective)
		}
	}
}

func TestNormalizeAction(t *testing.T) {
	if action, err := NormalizeAction(" DISABLE "); err != nil || action != ActionDisable {
		t.Fatalf("normalize disable: %v %v", action, err)
	}
	if _, err := NormalizeAction("degrade"); err == nil {
		t.Fatal("degrade is not implemented in v1.1")
	}
}
