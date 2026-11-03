package billing

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
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
	repo             *storage.Repository
	mu               sync.RWMutex
	prices           map[string]*storage.ModelPriceRecord
	groupMultipliers map[string]float64
	// wildcard lists every pattern containing "*", sorted most specific first (see
	// patternSpecificity) so the most specific rule always wins instead of
	// depending on Go's randomized map iteration order.
	wildcard []wildcardRule
	deflt    *storage.ModelPriceRecord
	// lastSignature identifies the rate set currently in effect. The periodic
	// sync reloads prices every 10s per replica; without this the log carried an
	// identical "reloaded model pricing engine" line every 10s (about 17k lines
	// per day per replica), which drowned out real billing events.
	lastSignature string
}

// wildcardRule is a pre-sorted wildcard pricing pattern.
type wildcardRule struct {
	group   string
	pattern string
	price   *storage.ModelPriceRecord
}

// NewEngine creates and initializes a BillingEngine with persisted pricing rates.
func NewEngine(repo *storage.Repository) *BillingEngine {
	e := &BillingEngine{
		repo:             repo,
		prices:           make(map[string]*storage.ModelPriceRecord),
		groupMultipliers: map[string]float64{"default": 1.0, "vip": 0.8, "enterprise": 1.5},
		deflt: &storage.ModelPriceRecord{
			Model:           "*",
			PromptPrice:     2.0, // Default benchmark ¥2.00 / 1M prompt
			CompletionPrice: 8.0, // Default benchmark ¥8.00 / 1M completion
			CacheReadPrice:  0.2, // Default benchmark ¥0.20 / 1M cached prompt (90% discount)
			FixedPrice:      0.0,
			Currency:        "CNY",
			// A model with no configured price must not also receive a time-of-use
			// discount: the fallback rate is already an estimate, and halving it
			// (00:00-08:30 plus all weekend for "custom" mode) silently gave away
			// 50% of the revenue for every unpriced model that appeared. Discounts
			// are opt-in per price rule.
			OffPeakEnabled:  false,
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

	// Build a deterministic wildcard index: most specific pattern first. Any
	// pattern containing "*" is a wildcard rule - trailing only ("gpt-4o-*") used
	// to be the sole supported form, which made "*o1" and "gpt-4o*mini" savable but
	// permanently unmatched.
	wildcards := make([]wildcardRule, 0)
	for _, rec := range list {
		grp := strings.ToLower(strings.TrimSpace(rec.GroupName))
		if grp == "" {
			grp = "default"
		}
		pattern := strings.ToLower(strings.TrimSpace(rec.Model))
		if strings.Contains(pattern, "*") {
			wildcards = append(wildcards, wildcardRule{
				group:   grp,
				pattern: pattern,
				price:   rec,
			})
		}
	}
	sort.SliceStable(wildcards, func(i, j int) bool {
		literalI, starsI := patternSpecificity(wildcards[i].pattern)
		literalJ, starsJ := patternSpecificity(wildcards[j].pattern)
		if literalI != literalJ {
			return literalI > literalJ
		}
		if starsI != starsJ {
			return starsI < starsJ
		}
		return wildcards[i].pattern < wildcards[j].pattern
	})

	multipliers := map[string]float64{"default": 1.0, "vip": 0.8, "enterprise": 1.5}
	if e.repo != nil {
		raw := e.repo.GetSetting("billing_group_multipliers", "")
		if raw != "" {
			_ = json.Unmarshal([]byte(raw), &multipliers)
		}
	}
	if _, ok := multipliers["default"]; !ok {
		multipliers["default"] = 1.0
	}

	signature := rateSignature(list, multipliers)

	e.mu.Lock()
	e.prices = m
	e.wildcard = wildcards
	e.groupMultipliers = multipliers
	changed := e.lastSignature != signature
	e.lastSignature = signature
	e.mu.Unlock()

	logger := telemetry.ScopedLogger("billing")
	if changed {
		logger.Info("reloaded model pricing engine", "rules_count", len(list), "groups_count", len(multipliers))
	} else {
		// Periodic reload with identical rates: keep it observable at debug level
		// instead of writing the same INFO line every 10 seconds.
		logger.Debug("model pricing unchanged; reload was a no-op", "rules_count", len(list))
	}
	return nil
}

// rateSignature is a stable digest of the loaded rate card, used only to decide
// whether a periodic reload actually changed anything. Ordering is canonical
// because the rules are sorted before hashing and JSON sorts map keys.
func rateSignature(list []*storage.ModelPriceRecord, multipliers map[string]float64) string {
	hasher := sha256.New()
	encoder := json.NewEncoder(hasher)
	_ = encoder.Encode(list)
	_ = encoder.Encode(multipliers)
	return hex.EncodeToString(hasher.Sum(nil))
}

// GetGroupMultiplier returns the billing rate multiplier for a group (defaults to 1.0).
func (e *BillingEngine) GetGroupMultiplier(groupName string) float64 {
	if e == nil {
		return 1.0
	}
	e.mu.RLock()
	defer e.mu.RUnlock()
	grp := strings.ToLower(strings.TrimSpace(groupName))
	if grp == "" || grp == "default" {
		return 1.0
	}
	if mult, ok := e.groupMultipliers[grp]; ok && mult > 0 {
		return mult
	}
	return 1.0
}

// GetGroupMultipliers returns a copy of all configured group multipliers.
func (e *BillingEngine) GetGroupMultipliers() map[string]float64 {
	if e == nil {
		return map[string]float64{"default": 1.0}
	}
	e.mu.RLock()
	defer e.mu.RUnlock()
	res := make(map[string]float64, len(e.groupMultipliers))
	for k, v := range e.groupMultipliers {
		res[k] = v
	}
	if _, ok := res["default"]; !ok {
		res["default"] = 1.0
	}
	return res
}

// SetGroupMultipliers updates in-memory group multipliers and persists to repository.
func (e *BillingEngine) SetGroupMultipliers(multipliers map[string]float64) error {
	if e == nil {
		return nil
	}
	clean := make(map[string]float64)
	clean["default"] = 1.0
	for k, v := range multipliers {
		grp := strings.ToLower(strings.TrimSpace(k))
		if grp != "" && v > 0 {
			clean[grp] = v
		}
	}
	e.mu.Lock()
	e.groupMultipliers = clean
	e.mu.Unlock()

	if e.repo != nil {
		b, err := json.Marshal(clean)
		if err == nil {
			return e.repo.SetSetting("billing_group_multipliers", string(b))
		}
		return err
	}
	return nil
}

// Price resolution sources reported by ResolvePriceSource. "default" means no
// rule matched and the hardcoded fallback rate was applied — the case the startup
// self-check warns about, because that rate is a guess rather than a decision.
const (
	PriceSourceExact    = "exact"
	PriceSourceWildcard = "wildcard"
	PriceSourceDefault  = "default"
)

// GetPriceWithGroup finds the applicable pricing rate for a requested model within a specific pricing tier/group.
func (e *BillingEngine) GetPriceWithGroup(modelName, groupName string) *storage.ModelPriceRecord {
	price, _ := e.resolvePriceWithSource(modelName, groupName)
	return price
}

// ResolvePriceSource reports how a model's rate was resolved: an exact rule, a
// wildcard rule, or the hardcoded default fallback. Operators use it to find
// models that are being served (and billed) at an unconfigured price.
func (e *BillingEngine) ResolvePriceSource(modelName, groupName string) string {
	if e == nil {
		return PriceSourceDefault
	}
	_, source := e.resolvePriceWithSource(modelName, groupName)
	return source
}

// resolvePriceWithSource is the single price-resolution implementation; source
// identifies which rule matched so callers can report it.
func (e *BillingEngine) resolvePriceWithSource(modelName, groupName string) (*storage.ModelPriceRecord, string) {
	modelLower := strings.ToLower(strings.TrimSpace(modelName))
	grp := strings.ToLower(strings.TrimSpace(groupName))
	if grp == "" {
		grp = "default"
	}

	e.mu.RLock()
	defer e.mu.RUnlock()

	// 1. Exact match in requested group
	if p, ok := e.prices[grp+":"+modelLower]; ok {
		return p, PriceSourceExact
	}

	// 2. Exact match in default group fallback
	if grp != "default" {
		if p, ok := e.prices["default:"+modelLower]; ok {
			return p, PriceSourceExact
		}
		if p, ok := e.prices[modelLower]; ok {
			return p, PriceSourceExact
		}
	} else {
		if p, ok := e.prices[modelLower]; ok {
			return p, PriceSourceExact
		}
	}

	// 3. Wildcard pattern match in group (e.g. "vip:gpt-4o-*" matching "gpt-4o-mini",
	// or "*o1" matching "openai/o1"). The rule list is pre-sorted most-specific
	// first, so the result is stable across requests and processes.
	//
	// NOTE: this closure must NOT re-acquire e.mu. The caller already holds the
	// read lock, and sync.RWMutex is not reentrant: if a writer (price reload)
	// queues between the outer RLock and this one, the inner RLock blocks
	// forever while the writer waits for the outer lock to be released.
	patternMatch := func(g string) *storage.ModelPriceRecord {
		for _, rule := range e.wildcard {
			if rule.group != g {
				continue
			}
			if MatchModelPattern(rule.pattern, modelLower) {
				return rule.price
			}
		}
		return nil
	}

	if p := patternMatch(grp); p != nil {
		return p, PriceSourceWildcard
	}
	if grp != "default" {
		if p := patternMatch("default"); p != nil {
			return p, PriceSourceWildcard
		}
	}

	// 4. Default standard fallback
	return e.deflt, PriceSourceDefault
}

// IsOffPeak evaluates whether the specified time is within the model's off-peak time window.
// Uses Strategy Pattern across modes: "custom", "night", "none", and single-window default.
func (e *BillingEngine) IsOffPeak(p *storage.ModelPriceRecord, t time.Time) (bool, float64) {
	if p == nil || !p.OffPeakEnabled {
		return false, 1.0
	}

	discount := p.OffPeakDiscount
	if discount <= 0 {
		// Zero/negative means "no discount", not "use a legacy 50% default".
		// Silently halving the price for an unset factor under-charged every
		// enabled window an operator created without an explicit coefficient.
		discount = 1.0
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

// cacheWritePriceMultiplier is the premium a provider charges for writing a
// prompt to its cache, relative to the plain input rate (Anthropic: 1.25x).
//
// Cache writes used to be billed at the plain prompt rate, which under-charged
// every cache miss by 20% and made a heavily cache-writing workload look cheaper
// than the provider invoice. The multiplier is applied to the resolved prompt
// price, so no new price column (and no migration) is needed.
const cacheWritePriceMultiplier = 1.25

// CalculateCostForUnitsWithCacheCreation is the single cost-calculation entry
// point: every billable request goes through it.
//
// It applies one resolved price and its discounts to every fixed-price unit,
// charges the request's token usage once, and bills cache writes at their
// premium. The narrower CalculateCost* variants that used to wrap it (group/no
// group, with/without a timestamp, with/without cache creation) are gone: they
// were reachable only from tests, and picking the wrong one silently dropped
// fixed-price units or the cache-write premium.
//
// promptTokens must include the cache-read and cache-write tokens (that is how
// Anthropic's input_tokens, cache_read_input_tokens and
// cache_creation_input_tokens compose); this function splits them into three
// priced buckets so each is billed at its own rate.
func (e *BillingEngine) CalculateCostForUnitsWithCacheCreation(modelName, groupName string, promptTokens, completionTokens, cachedTokens, cacheCreationTokens, units int, t time.Time) (cost float64, totalSaved float64, cacheSaved float64, isOffPeak bool, discount float64, snapshot *storage.UsagePricingSnapshot) {
	if e == nil {
		return 0, 0, 0, false, 1.0, nil
	}

	p := e.GetPriceWithGroup(modelName, groupName)
	if p == nil {
		return 0, 0, 0, false, 1.0, nil
	}
	if units < 1 {
		units = 1
	}
	fixedCost := p.FixedPrice * float64(units)

	if promptTokens < 0 {
		promptTokens = 0
	}
	if completionTokens < 0 {
		completionTokens = 0
	}
	if cachedTokens < 0 {
		cachedTokens = 0
	}
	if cacheCreationTokens < 0 {
		cacheCreationTokens = 0
	}
	if cachedTokens > promptTokens {
		cachedTokens = promptTokens
	}
	// Cache writes and cache reads are disjoint and both live inside promptTokens;
	// clamping the write count after the read count keeps the three buckets from
	// exceeding the prompt.
	if cacheCreationTokens > promptTokens-cachedTokens {
		cacheCreationTokens = promptTokens - cachedTokens
	}

	uncachedPrompt := promptTokens - cachedTokens - cacheCreationTokens
	cacheWritePrice := p.PromptPrice * cacheWritePriceMultiplier

	// Standard benchmark token costs
	promptCost := (float64(uncachedPrompt) * p.PromptPrice) / 1_000_000.0
	cachedCost := (float64(cachedTokens) * p.CacheReadPrice) / 1_000_000.0
	cacheWriteCost := (float64(cacheCreationTokens) * cacheWritePrice) / 1_000_000.0
	compCost := (float64(completionTokens) * p.CompletionPrice) / 1_000_000.0
	baseTokenCost := promptCost + cachedCost + cacheWriteCost + compCost

	if cachedTokens > 0 && p.PromptPrice > p.CacheReadPrice {
		cacheSaved = (float64(cachedTokens) * (p.PromptPrice - p.CacheReadPrice)) / 1_000_000.0
	}

	// Group Multiplier Evaluation:
	// A pricing group scales base rates by a multiplier (e.g. vip: 0.8x, enterprise: 1.5x).
	// If the price record is not a group-specific override (i.e. it resolved to the
	// default base rate or standard fallback), apply the group multiplier.
	grp := strings.ToLower(strings.TrimSpace(groupName))
	multiplier := 1.0
	if grp != "" && grp != "default" {
		pGrp := strings.ToLower(strings.TrimSpace(p.GroupName))
		if pGrp != grp {
			multiplier = e.GetGroupMultiplier(grp)
			if multiplier <= 0 {
				multiplier = 1.0
			}
		}
	}

	groupedTokenCost := baseTokenCost * multiplier
	groupedFixedCost := fixedCost * multiplier

	groupSaved := 0.0
	if multiplier < 1.0 {
		groupSaved = (baseTokenCost + fixedCost) * (1.0 - multiplier)
	}

	// Time-of-Use Off-Peak Discount Evaluation
	isOffPeak, discount = e.IsOffPeak(p, t)
	if isOffPeak {
		discountedTokenCost := groupedTokenCost * discount
		discountedFixedCost := groupedFixedCost * discount
		offPeakSaved := (groupedTokenCost - discountedTokenCost) + (groupedFixedCost - discountedFixedCost)
		cost = discountedTokenCost + discountedFixedCost
		totalSaved = cacheSaved + groupSaved + offPeakSaved
	} else {
		cost = groupedTokenCost + groupedFixedCost
		totalSaved = cacheSaved + groupSaved
		discount = 1.0
	}

	cost = roundPrecision(cost)
	totalSaved = roundPrecision(totalSaved)
	cacheSaved = roundPrecision(cacheSaved)

	snapshot = &storage.UsagePricingSnapshot{
		Model: p.Model, GroupName: groupName, PriceGroup: p.GroupName, Currency: p.Currency,
		PromptPrice: p.PromptPrice, CompletionPrice: p.CompletionPrice, CacheReadPrice: p.CacheReadPrice,
		CacheWritePrice: cacheWritePrice, CacheCreationTokens: cacheCreationTokens,
		FixedPrice: p.FixedPrice, GroupMultiplier: multiplier, OffPeakDiscount: discount, BillableUnits: units,
	}
	return cost, totalSaved, cacheSaved, isOffPeak, discount, snapshot
}

// roundPrecision rounds monetary amounts to 8 decimal places (fraction of micro-cents)
// to eliminate IEEE-754 binary floating point precision drift.
func roundPrecision(val float64) float64 {
	return math.Round(val*1e8) / 1e8
}
