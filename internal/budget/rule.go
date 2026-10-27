package budget

import (
	"fmt"
	"math"
	"strings"
)

// ScopeType is the entity a budget belongs to. Resolution walks key -> user ->
// nearest organization ancestor, so a department budget applies to every key of
// every member unless a more specific budget overrides it.
type ScopeType string

const (
	ScopeKey  ScopeType = "key"
	ScopeUser ScopeType = "user"
	ScopeOrg  ScopeType = "org"
)

// Action is what the data plane does when the budget is exhausted.
type Action string

const (
	// ActionNotify lets the request through and emits alert events.
	ActionNotify Action = "notify"
	// ActionDisable rejects new spending once the budget is exhausted.
	ActionDisable Action = "disable"
)

// Rule is one configured budget, detached from storage so the resolution and
// evaluation logic can be unit-tested without a database.
type Rule struct {
	ID          int64
	ScopeType   ScopeType
	ScopeID     int64
	Period      Period
	AmountUnits int64
	UsedUnits   int64
	Action      Action
	WarnPercent int
}

// Validate rejects rules that could silently disable or undercharge traffic.
func (r Rule) Validate() error {
	switch r.ScopeType {
	case ScopeKey, ScopeUser, ScopeOrg:
	default:
		return fmt.Errorf("invalid budget scope %q", r.ScopeType)
	}
	if r.ScopeID <= 0 {
		return fmt.Errorf("budget scope id is required")
	}
	if !r.Period.Valid() {
		return fmt.Errorf("invalid budget period %q", r.Period)
	}
	if r.AmountUnits <= 0 {
		return fmt.Errorf("budget amount must be positive")
	}
	if r.Action != ActionNotify && r.Action != ActionDisable {
		return fmt.Errorf("invalid budget action %q", r.Action)
	}
	if r.WarnPercent < 0 || r.WarnPercent > 100 {
		return fmt.Errorf("warn percent must be between 0 and 100")
	}
	return nil
}

// Decision is the outcome of evaluating one effective rule.
type Decision struct {
	Allowed          bool
	ShouldAlert      bool
	Threshold        int // highest crossed threshold (50, warn percent or 100); 0 when none
	WatermarkPercent float64
	Action           Action
	Reason           string
}

// Evaluate combines the persisted used amount with the caller's live counter.
// The larger value wins: a stale counter must not resurrect spending that the
// database already recorded, and a fresh counter must not be ignored while the
// periodic flush catches up.
func Evaluate(rule Rule, liveUsedUnits int64) Decision {
	decision := Decision{Allowed: true, Action: rule.Action}
	if rule.AmountUnits <= 0 {
		return decision
	}
	used := rule.UsedUnits
	if liveUsedUnits > used {
		used = liveUsedUnits
	}
	if used < 0 {
		used = 0
	}
	percent := float64(used) / float64(rule.AmountUnits) * 100
	decision.WatermarkPercent = math.Round(percent*100) / 100

	warn := rule.WarnPercent
	if warn <= 0 || warn > 100 {
		warn = 80
	}
	thresholds := []int{50, warn, 100}
	for _, threshold := range thresholds {
		if percent >= float64(threshold) && threshold > decision.Threshold {
			decision.Threshold = threshold
		}
	}
	if decision.Threshold >= warn || decision.Threshold >= 100 {
		decision.ShouldAlert = true
	}
	if percent >= 100 && rule.Action == ActionDisable {
		decision.Allowed = false
		decision.Reason = "budget_exhausted"
		decision.Threshold = 100
	} else if decision.ShouldAlert {
		decision.Reason = "budget_threshold_reached"
	}
	return decision
}

// Resolve picks the effective rule per period. Precedence is key -> user ->
// nearest organization ancestor; the three periods are independent, so a key
// can have a monthly cap while its department has a total cap.
func Resolve(candidates []Rule, keyID, userID int64, orgChain []int64) []Rule {
	orgRank := make(map[int64]int, len(orgChain))
	for rank, id := range orgChain {
		orgRank[id] = rank
	}

	byKey := make(map[Period]Rule)
	byUser := make(map[Period]Rule)
	orgChoice := make(map[Period]struct {
		rule Rule
		rank int
	})
	for _, rule := range candidates {
		switch rule.ScopeType {
		case ScopeKey:
			if rule.ScopeID == keyID {
				byKey[rule.Period] = rule
			}
		case ScopeUser:
			if rule.ScopeID == userID {
				byUser[rule.Period] = rule
			}
		case ScopeOrg:
			rank, ok := orgRank[rule.ScopeID]
			if !ok {
				continue
			}
			current, exists := orgChoice[rule.Period]
			if !exists || rank < current.rank {
				orgChoice[rule.Period] = struct {
					rule Rule
					rank int
				}{rule: rule, rank: rank}
			}
		}
	}

	effective := make([]Rule, 0, 3)
	for _, period := range []Period{PeriodDaily, PeriodMonthly, PeriodTotal} {
		if rule, ok := byKey[period]; ok {
			effective = append(effective, rule)
			continue
		}
		if rule, ok := byUser[period]; ok {
			effective = append(effective, rule)
			continue
		}
		if choice, ok := orgChoice[period]; ok {
			effective = append(effective, choice.rule)
		}
	}
	return effective
}

// NormalizeAction trims and lower-cases an operator-provided action.
func NormalizeAction(raw string) (Action, error) {
	action := Action(strings.ToLower(strings.TrimSpace(raw)))
	if action != ActionNotify && action != ActionDisable {
		return "", fmt.Errorf("invalid budget action %q (want notify or disable)", raw)
	}
	return action, nil
}
