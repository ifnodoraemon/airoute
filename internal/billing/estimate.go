package billing

import (
	"math"
	"strings"

	"github.com/ifnodoraemon/airoute/internal/storage"
)

// tokensPerPriceUnit is the token count the rate card is quoted against:
// ModelPriceRecord.PromptPrice / CompletionPrice are per 1,000,000 tokens.
const tokensPerPriceUnit = 1_000_000.0

// EstimateCostCNY returns a deliberately conservative upper bound of what a
// request may cost, used by the admission gate to pre-authorise wallet funds.
//
// The bound is promptTokens×PromptPrice + maxCompletionTokens×CompletionPrice,
// scaled by the pricing group's multiplier and rounded to 1e-8 CNY. It is
// intentionally pessimistic in three places:
//
//   - The completion side uses the request's maximum generation length, not the
//     tokens the model will actually emit.
//   - Cached prompt tokens are priced at the full PromptPrice. Admission cannot
//     know how much of the prompt the provider will serve from cache, and
//     under-reserving is the failure that lets a burst spend money the wallet
//     does not have.
//   - Time-of-use (off-peak) discounts are ignored: the estimate must not assume
//     a future discount for money that is being committed now.
//
// A nil GlobalEngine, an unpriced model with no fallback rate, or a request with
// no billable tokens all return 0. maxCompletionTokens <= 0 prices the prompt
// only (callers that do not know a generation cap still get a prompt bound).
func EstimateCostCNY(model, group string, promptTokens, maxCompletionTokens int) float64 {
	engine := GlobalEngine
	if engine == nil {
		return 0
	}
	price := engine.GetPriceWithGroup(model, group)
	if price == nil {
		return 0
	}
	if promptTokens < 0 {
		promptTokens = 0
	}
	if maxCompletionTokens < 0 {
		maxCompletionTokens = 0
	}

	tokenCost := float64(promptTokens) * price.PromptPrice
	if maxCompletionTokens > 0 {
		tokenCost += float64(maxCompletionTokens) * price.CompletionPrice
	}
	if tokenCost <= 0 {
		return 0
	}
	cost := tokenCost / tokensPerPriceUnit

	// Mirror CalculateCostForUnitsWithCacheCreation: a group-specific rate override
	// already carries that group's price, so the multiplier only applies when the
	// rate resolved to the base/default card.
	grp := strings.ToLower(strings.TrimSpace(group))
	if grp != "" && grp != "default" {
		if priceGroup := strings.ToLower(strings.TrimSpace(price.GroupName)); priceGroup != grp {
			if multiplier := engine.GetGroupMultiplier(grp); multiplier > 0 {
				cost *= multiplier
			}
		}
	}

	// Same 1e-8 rounding the settlement path applies, so an estimate and the
	// charge it was compared against are expressed in the same precision.
	return roundPrecision(cost)
}

// EstimateCostUnits is EstimateCostCNY expressed in the ledger's integer
// sub-units (1e-9 CNY), which is the representation the distributed in-flight
// reservation and the telemetry gauges use.
//
// The conversion rounds to the nearest sub-unit so that a cost that is a whole
// number of sub-units survives the float round trip exactly; any positive cost
// still yields at least one unit, because a zero-unit reservation would admit an
// unbounded request for free.
func EstimateCostUnits(model, group string, promptTokens, maxCompletionTokens int) int64 {
	cost := EstimateCostCNY(model, group, promptTokens, maxCompletionTokens)
	if cost <= 0 {
		return 0
	}
	units := int64(math.Round(cost * float64(storage.UnitsPerCurrency)))
	if units < 1 {
		units = 1
	}
	return units
}
