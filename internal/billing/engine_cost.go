package billing

import (
	"math"
	"strings"
	"time"

	"github.com/ifnodoraemon/airoute/internal/storage"
)

// Off-peak evaluation and the single cost-calculation entry point, extracted from
// engine.go to keep the price-resolution file under the size red line.

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