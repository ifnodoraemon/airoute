package billing

import (
	"math"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/ifnodoraemon/airoute/internal/storage"
	"github.com/ifnodoraemon/airoute/internal/telemetry"
)

// GlobalEngine is the singleton billing calculator used by all data plane handlers.
var GlobalEngine *BillingEngine

// BillingEngine manages model pricing rates and real-time cost calculation.
type BillingEngine struct {
	repo   *storage.Repository
	mu     sync.RWMutex
	prices map[string]*storage.ModelPriceRecord
	// wildcard lists every "*"-suffixed pattern as "group:prefix", sorted by
	// prefix length descending so the most specific rule always wins instead of
	// depending on Go's randomized map iteration order.
	wildcard []wildcardRule
	deflt    *storage.ModelPriceRecord
}

// wildcardRule is a pre-sorted wildcard pricing pattern.
type wildcardRule struct {
	group  string
	prefix string
	price  *storage.ModelPriceRecord
}

// NewEngine creates and initializes a BillingEngine with persisted pricing rates.
func NewEngine(repo *storage.Repository) *BillingEngine {
	e := &BillingEngine{
		repo:   repo,
		prices: make(map[string]*storage.ModelPriceRecord),
		deflt: &storage.ModelPriceRecord{
			Model:           "*",
			PromptPrice:     2.0, // Default benchmark ¥2.00 / 1M prompt
			CompletionPrice: 8.0, // Default benchmark ¥8.00 / 1M completion
			CacheReadPrice:  0.2, // Default benchmark ¥0.20 / 1M cached prompt (90% discount)
			FixedPrice:      0.0,
			Currency:        "CNY",
			OffPeakEnabled:  true,
			OffPeakMode:     "custom",
			OffPeakDiscount: 0.5,
			WeekendAllDay:   true,
			OffPeakStart:    "00:00",
			OffPeakEnd:      "08:30",
		},
	}
	if repo != nil {
		_ = repo.SeedDefaultModelPrices()
		_ = e.ReloadPrices()
	}
	return e
}

// InitGlobalEngine sets the global billing engine instance.
func InitGlobalEngine(repo *storage.Repository) {
	GlobalEngine = NewEngine(repo)
}

// ReloadPrices loads all pricing rules from the repository into memory.
func (e *BillingEngine) ReloadPrices() error {
	if e.repo == nil {
		return nil
	}
	list, err := e.repo.ListModelPrices()
	if err != nil {
		return err
	}

	m := make(map[string]*storage.ModelPriceRecord)
	for _, rec := range list {
		grp := strings.ToLower(strings.TrimSpace(rec.GroupName))
		if grp == "" {
			grp = "default"
		}
		m[grp+":"+strings.ToLower(rec.Model)] = rec
		if grp == "default" {
			m[strings.ToLower(rec.Model)] = rec
		}
	}

	// Build a deterministic wildcard index: longest prefix first.
	wildcards := make([]wildcardRule, 0)
	for _, rec := range list {
		grp := strings.ToLower(strings.TrimSpace(rec.GroupName))
		if grp == "" {
			grp = "default"
		}
		modelLower := strings.ToLower(strings.TrimSpace(rec.Model))
		if strings.HasSuffix(modelLower, "*") {
			wildcards = append(wildcards, wildcardRule{
				group:  grp,
				prefix: strings.TrimSuffix(modelLower, "*"),
				price:  rec,
			})
		}
	}
	sort.SliceStable(wildcards, func(i, j int) bool {
		if len(wildcards[i].prefix) != len(wildcards[j].prefix) {
			return len(wildcards[i].prefix) > len(wildcards[j].prefix)
		}
		return wildcards[i].prefix < wildcards[j].prefix
	})

	e.mu.Lock()
	e.prices = m
	e.wildcard = wildcards
	e.mu.Unlock()

	telemetry.Logger.Info("reloaded model pricing engine", "rules_count", len(list))
	return nil
}

// GetPrice finds the applicable pricing rate for a requested model in default group.
func (e *BillingEngine) GetPrice(modelName string) *storage.ModelPriceRecord {
	return e.GetPriceWithGroup(modelName, "default")
}

// GetPriceWithGroup finds the applicable pricing rate for a requested model within a specific pricing tier/group.
func (e *BillingEngine) GetPriceWithGroup(modelName, groupName string) *storage.ModelPriceRecord {
	modelLower := strings.ToLower(strings.TrimSpace(modelName))
	grp := strings.ToLower(strings.TrimSpace(groupName))
	if grp == "" {
		grp = "default"
	}

	e.mu.RLock()
	defer e.mu.RUnlock()

	// 1. Exact match in requested group
	if p, ok := e.prices[grp+":"+modelLower]; ok {
		return p
	}

	// 2. Exact match in default group fallback
	if grp != "default" {
		if p, ok := e.prices["default:"+modelLower]; ok {
			return p
		}
		if p, ok := e.prices[modelLower]; ok {
			return p
		}
	} else {
		if p, ok := e.prices[modelLower]; ok {
			return p
		}
	}

	// 3. Wildcard prefix match in group (e.g. "vip:gpt-4o-*" matching "gpt-4o-mini").
	// The rule list is pre-sorted longest-prefix-first, so the result is stable
	// across requests and processes.
	//
	// NOTE: this closure must NOT re-acquire e.mu. The caller already holds the
	// read lock, and sync.RWMutex is not reentrant: if a writer (price reload)
	// queues between the outer RLock and this one, the inner RLock blocks
	// forever while the writer waits for the outer lock to be released.
	prefixMatch := func(g string) *storage.ModelPriceRecord {
		for _, rule := range e.wildcard {
			if rule.group != g {
				continue
			}
			if strings.HasPrefix(modelLower, rule.prefix) {
				return rule.price
			}
		}
		return nil
	}

	if p := prefixMatch(grp); p != nil {
		return p
	}
	if grp != "default" {
		if p := prefixMatch("default"); p != nil {
			return p
		}
	}

	// 4. Group wildcard or global wildcard "*"
	if p, ok := e.prices[grp+":*"]; ok {
		return p
	}
	if p, ok := e.prices["*"]; ok {
		return p
	}

	// 5. Default standard fallback
	return e.deflt
}

// IsOffPeak evaluates whether the specified time is within the model's off-peak time window.
// Uses Strategy Pattern across modes: "custom", "night", "none", and single-window default.
func (e *BillingEngine) IsOffPeak(p *storage.ModelPriceRecord, t time.Time) (bool, float64) {
	if p == nil || !p.OffPeakEnabled {
		return false, 1.0
	}

	discount := p.OffPeakDiscount
	if discount <= 0 {
		// Legacy rows without an explicit discount keep the historical 50% default.
		discount = 0.5
	}
	if discount > 1.0 {
		// A discount factor above 1.0 would mean a surcharge; clamp to no discount.
		discount = 1.0
	}

	localT := t.In(shanghaiLoc)
	weekday := localT.Weekday()
	isoDay := int(weekday)
	if isoDay == 0 {
		isoDay = 7
	}

	evalCtx := &OffPeakEvaluationContext{
		LocalTime: localT,
		Weekday:   weekday,
		IsWeekend: (weekday == time.Saturday || weekday == time.Sunday),
		ISODay:    isoDay,
		CurrentM:  localT.Hour()*60 + localT.Minute(),
		Discount:  discount,
	}

	mode := strings.ToLower(strings.TrimSpace(p.OffPeakMode))
	strategy, exists := offPeakStrategies[mode]
	if !exists {
		strategy = &WindowOffPeakStrategy{}
	}

	return strategy.Evaluate(p, evalCtx)
}

// CalculateCostDetailedWithGroup computes total request cost, total saved amount, cache savings, and off-peak discount metrics for a specified pricing group.
func (e *BillingEngine) CalculateCostDetailedWithGroup(modelName, groupName string, promptTokens, completionTokens, cachedTokens int, t time.Time) (cost float64, totalSaved float64, cacheSaved float64, isOffPeak bool, discount float64) {
	if e == nil {
		return 0, 0, 0, false, 1.0
	}

	p := e.GetPriceWithGroup(modelName, groupName)
	if p == nil {
		return 0, 0, 0, false, 1.0
	}

	if promptTokens < 0 {
		promptTokens = 0
	}
	if completionTokens < 0 {
		completionTokens = 0
	}
	if cachedTokens < 0 {
		cachedTokens = 0
	}
	if cachedTokens > promptTokens {
		cachedTokens = promptTokens
	}

	uncachedPrompt := promptTokens - cachedTokens

	// Standard benchmark token costs
	promptCost := (float64(uncachedPrompt) * p.PromptPrice) / 1_000_000.0
	cachedCost := (float64(cachedTokens) * p.CacheReadPrice) / 1_000_000.0
	compCost := (float64(completionTokens) * p.CompletionPrice) / 1_000_000.0
	tokenCost := promptCost + cachedCost + compCost

	if cachedTokens > 0 && p.PromptPrice > p.CacheReadPrice {
		cacheSaved = (float64(cachedTokens) * (p.PromptPrice - p.CacheReadPrice)) / 1_000_000.0
	}

	// Time-of-Use Off-Peak Discount Evaluation
	isOffPeak, discount = e.IsOffPeak(p, t)
	if isOffPeak {
		discountedTokenCost := tokenCost * discount
		discountedFixedCost := p.FixedPrice * discount
		offPeakSaved := (tokenCost - discountedTokenCost) + (p.FixedPrice - discountedFixedCost)
		cost = discountedTokenCost + discountedFixedCost
		totalSaved = cacheSaved + offPeakSaved
	} else {
		cost = tokenCost + p.FixedPrice
		totalSaved = cacheSaved
		discount = 1.0
	}

	cost = roundPrecision(cost)
	totalSaved = roundPrecision(totalSaved)
	cacheSaved = roundPrecision(cacheSaved)

	return cost, totalSaved, cacheSaved, isOffPeak, discount
}

// roundPrecision rounds monetary amounts to 8 decimal places (fraction of micro-cents)
// to eliminate IEEE-754 binary floating point precision drift.
func roundPrecision(val float64) float64 {
	return math.Round(val*1e8) / 1e8
}

// CalculateCostDetailed computes total request cost, total saved amount, cache savings, and off-peak discount metrics using the default group.
func (e *BillingEngine) CalculateCostDetailed(modelName string, promptTokens, completionTokens, cachedTokens int, t time.Time) (cost float64, totalSaved float64, cacheSaved float64, isOffPeak bool, discount float64) {
	return e.CalculateCostDetailedWithGroup(modelName, "default", promptTokens, completionTokens, cachedTokens, t)
}

// CalculateCostWithGroup computes total request cost and savings using current server time and the specified pricing group.
func (e *BillingEngine) CalculateCostWithGroup(modelName, groupName string, promptTokens, completionTokens, cachedTokens int) (cost float64, savedCost float64) {
	cost, totalSaved, _, _, _ := e.CalculateCostDetailedWithGroup(modelName, groupName, promptTokens, completionTokens, cachedTokens, time.Now())
	return cost, totalSaved
}

// CalculateCost computes total request cost and savings using current server time and default group.
func (e *BillingEngine) CalculateCost(modelName string, promptTokens, completionTokens, cachedTokens int) (cost float64, savedCost float64) {
	return e.CalculateCostWithGroup(modelName, "default", promptTokens, completionTokens, cachedTokens)
}
