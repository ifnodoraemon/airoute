package governance

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/ifnodoraemon/airoute/internal/budget"
	"github.com/ifnodoraemon/airoute/internal/notify"
	"github.com/ifnodoraemon/airoute/internal/storage"
	"github.com/ifnodoraemon/airoute/internal/telemetry"
)

const (
	defaultRuleTTL = 30 * time.Second
	notifyTimeout  = 10 * time.Second
	checkTimeout   = 500 * time.Millisecond
)

// Subject identifies the entities a request spends against.
type Subject struct {
	KeyID  int64
	UserID int64
	OrgID  int64
}

// Service evaluates budgets on the data plane. Rules are cached briefly because
// they change rarely, while counters are live and shared with the settlement
// path.
type Service struct {
	repo     *storage.Repository
	counter  budget.Counter
	notifier notify.Sender
	now      func() time.Time
	ttl      time.Duration

	mu       sync.RWMutex
	rules    map[int64]*cachedRules
	orgChain map[int64]*cachedOrgChain
	warned   sync.Map // dedupe message key -> time.Time

	pendingMu sync.Mutex
	pending   map[int64]int64 // budget id -> units waiting for the next flush
}

type cachedRules struct {
	rules []budget.Rule
	err   error
	at    time.Time
}

type cachedOrgChain struct {
	ids []int64
	err error
	at  time.Time
}

var global atomic.Pointer[Service]

// SetGlobal wires the process-wide service. It is called once during startup.
func SetGlobal(service *Service) { global.Store(service) }

// GlobalService returns the process-wide service, or nil when budgets are not
// wired (tests, CLI commands).
func GlobalService() *Service { return global.Load() }

// NewService builds the evaluator. A nil counter or repository disables the
// service's effects rather than panicking on the request path.
func NewService(repo *storage.Repository, counter budget.Counter, notifier notify.Sender) *Service {
	return &Service{
		repo:     repo,
		counter:  counter,
		notifier: notifier,
		now:      time.Now,
		ttl:      defaultRuleTTL,
		rules:    make(map[int64]*cachedRules),
		orgChain: make(map[int64]*cachedOrgChain),
		pending:  make(map[int64]int64),
	}
}

// Invalidate clears cached rules. Admin budget writes call it so a change is
// visible immediately instead of after the TTL.
func (s *Service) Invalidate() {
	if s == nil {
		return
	}
	s.mu.Lock()
	s.rules = make(map[int64]*cachedRules)
	s.orgChain = make(map[int64]*cachedOrgChain)
	s.mu.Unlock()
}

// Check answers "may this request start?" using the live counters and the
// persisted usage. It returns blocked=true only for a disable rule that has
// crossed 100%; notify rules never block.
func (s *Service) Check(ctx context.Context, subject Subject) (budget.Decision, bool, error) {
	if s == nil || s.repo == nil || s.counter == nil || subject.KeyID <= 0 {
		return budget.Decision{Allowed: true}, false, nil
	}
	checkCtx, cancel := context.WithTimeout(ctx, checkTimeout)
	defer cancel()
	rules, err := s.rulesFor(checkCtx, subject)
	if err != nil {
		return budget.Decision{Allowed: true}, false, err
	}
	for _, rule := range rules {
		live, err := s.counter.Get(checkCtx, rule.ID, rule.Period, s.now())
		if err != nil {
			return budget.Decision{Allowed: true}, false, err
		}
		decision := budget.Evaluate(rule, live)
		if !decision.Allowed {
			return decision, true, nil
		}
	}
	return budget.Decision{Allowed: true}, false, nil
}

// RecordSpend adds a completed request's cost to every effective period and
// emits threshold events. It is safe to call on a goroutine: failures are logged
// and never affect the already-finished response.
func (s *Service) RecordSpend(ctx context.Context, subject Subject, costUnits int64) {
	if s == nil || s.repo == nil || s.counter == nil || subject.KeyID <= 0 || costUnits <= 0 {
		return
	}
	now := s.now()
	rules, err := s.rulesFor(ctx, subject)
	if err != nil {
		s.warnOnce("rules:"+fmt.Sprint(subject.KeyID), "budget rules unavailable", "error", err.Error(), "key_id", subject.KeyID)
		return
	}
	for _, rule := range rules {
		used, err := s.counter.Add(ctx, rule.ID, rule.Period, now, costUnits)
		if err != nil {
			s.warnOnce(fmt.Sprintf("counter:%d", rule.ID), "budget counter unavailable", "error", err.Error(), "budget_id", rule.ID)
			continue
		}
		s.addPending(rule.ID, costUnits)
		decision := budget.Evaluate(rule, used)
		if decision.Threshold == 0 {
			continue
		}
		periodKey := rule.Period.Key(now)
		inserted, err := s.repo.RecordBudgetEvent(rule.ID, decision.Threshold, periodKey, used, rule.AmountUnits, string(rule.Action))
		if err != nil {
			s.warnOnce(fmt.Sprintf("event:%d:%d", rule.ID, decision.Threshold), "failed to persist budget event", "error", err.Error(), "budget_id", rule.ID)
			continue
		}
		if !inserted || !decision.ShouldAlert {
			continue
		}
		s.sendNotification(rule, decision, used, periodKey)
	}
}

func (s *Service) rulesFor(ctx context.Context, subject Subject) ([]budget.Rule, error) {
	now := s.now()
	s.mu.RLock()
	cached, ok := s.rules[subject.KeyID]
	s.mu.RUnlock()
	if ok && now.Sub(cached.at) < s.ttl {
		return cached.rules, cached.err
	}

	orgIDs, err := s.orgChainFor(ctx, subject.OrgID)
	if err != nil {
		s.storeRules(subject.KeyID, nil, err, now)
		return nil, err
	}
	rules, err := s.repo.EffectiveBudgetRules(subject.KeyID, subject.UserID, orgIDs)
	s.storeRules(subject.KeyID, rules, err, now)
	return rules, err
}

func (s *Service) storeRules(keyID int64, rules []budget.Rule, err error, at time.Time) {
	s.mu.Lock()
	s.rules[keyID] = &cachedRules{rules: rules, err: err, at: at}
	s.mu.Unlock()
}

func (s *Service) orgChainFor(ctx context.Context, orgID int64) ([]int64, error) {
	if orgID <= 0 {
		return nil, nil
	}
	now := s.now()
	s.mu.RLock()
	cached, ok := s.orgChain[orgID]
	s.mu.RUnlock()
	if ok && now.Sub(cached.at) < s.ttl {
		return cached.ids, cached.err
	}
	ancestors, err := s.repo.OrgAncestors(orgID)
	if err != nil {
		s.mu.Lock()
		s.orgChain[orgID] = &cachedOrgChain{err: err, at: now}
		s.mu.Unlock()
		return nil, err
	}
	// OrgAncestors returns root -> self; precedence wants self -> root.
	ids := make([]int64, 0, len(ancestors))
	for i := len(ancestors) - 1; i >= 0; i-- {
		ids = append(ids, ancestors[i].ID)
	}
	s.mu.Lock()
	s.orgChain[orgID] = &cachedOrgChain{ids: ids, at: now}
	s.mu.Unlock()
	return ids, nil
}

// addPending records the amount that the next flush should persist. The counter
// remains the live value; used_units is the durable, restart-safe mirror.
func (s *Service) addPending(budgetID, units int64) {
	if units <= 0 {
		return
	}
	s.pendingMu.Lock()
	s.pending[budgetID] += units
	s.pendingMu.Unlock()
}

// Flush persists pending counter deltas into budgets.used_units. On a database
// error the delta is put back so the next tick retries it; a crash loses at most
// one interval, and the live counter still governs admission in the meantime.
func (s *Service) Flush(ctx context.Context) error {
	if s == nil || s.repo == nil {
		return nil
	}
	s.pendingMu.Lock()
	pending := s.pending
	s.pending = make(map[int64]int64)
	s.pendingMu.Unlock()
	if len(pending) == 0 {
		return nil
	}
	var errs []error
	for budgetID, units := range pending {
		if err := s.repo.IncrementBudgetUsage(budgetID, units); err != nil {
			s.pendingMu.Lock()
			s.pending[budgetID] += units
			s.pendingMu.Unlock()
			errs = append(errs, fmt.Errorf("budget %d: %w", budgetID, err))
		}
	}
	return errors.Join(errs...)
}

// StartFlusher runs Flush on a fixed interval for the lifetime of the process.
func (s *Service) StartFlusher(interval time.Duration) {
	if s == nil || s.repo == nil || interval <= 0 {
		return
	}
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for range ticker.C {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			if err := s.Flush(ctx); err != nil {
				s.warnOnce("flush", "failed to flush budget usage", "error", err.Error())
			}
			cancel()
		}
	}()
}

func (s *Service) warnOnce(key, message string, args ...any) {
	if _, loaded := s.warned.LoadOrStore(key, time.Now()); loaded {
		return
	}
	telemetry.WarnLogger(telemetry.ScopedLogger("budget"), "budget:"+key, 30*time.Second, message, args...)
}

func (s *Service) sendNotification(rule budget.Rule, decision budget.Decision, used int64, periodKey string) {
	if s.notifier == nil {
		return
	}
	level := notify.LevelWarning
	if decision.Threshold >= 100 {
		level = notify.LevelCritical
	}
	message := notify.Message{
		Event: "budget.threshold",
		Level: level,
		Title: fmt.Sprintf("预算告警：%s %d", rule.ScopeType, decision.Threshold),
		Body:  fmt.Sprintf("scope=%s:%d period=%s", rule.ScopeType, rule.ScopeID, periodKey),
		Fields: map[string]string{
			"scope_type": string(rule.ScopeType),
			"scope_id":   fmt.Sprint(rule.ScopeID),
			"period":     string(rule.Period),
			"period_key": periodKey,
			"threshold":  fmt.Sprint(decision.Threshold),
			"used":       fmt.Sprintf("%.4f", storage.FromUnits(used)),
			"amount":     fmt.Sprintf("%.4f", storage.FromUnits(rule.AmountUnits)),
			"percent":    fmt.Sprintf("%.2f", decision.WatermarkPercent),
			"action":     string(rule.Action),
		},
		Timestamp: s.now(),
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), notifyTimeout)
		defer cancel()
		if err := s.notifier.Send(ctx, message); err != nil {
			telemetry.WarnLogger(telemetry.ScopedLogger("budget"), "budget-notify:"+fmt.Sprint(rule.ID), 30*time.Second,
				"budget notification failed", "error", err.Error(), "budget_id", rule.ID)
		}
	}()
}
