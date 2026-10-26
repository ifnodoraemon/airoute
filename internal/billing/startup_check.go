package billing

import (
	"time"

	"github.com/ifnodoraemon/airoute/internal/storage"
)

// UnpricedModelsInUse returns models that served traffic recently but resolve to
// the hardcoded fallback rate card instead of a configured price rule.
//
// The fallback rate (¥2/¥8 per 1M tokens with no discount) is a guess: for one
// observed model it overcharged customers 2x, and for any model cheaper than the
// fallback it undercharges. Neither is visible anywhere today, so the gateway
// reports it once at startup where an operator will actually see it.
func UnpricedModelsInUse(repo *storage.Repository, engine *BillingEngine, window time.Duration) ([]string, error) {
	if repo == nil || engine == nil {
		return nil, nil
	}
	models, err := repo.DistinctRecentModels(time.Now().Add(-window), 200)
	if err != nil {
		return nil, err
	}
	unpriced := make([]string, 0, len(models))
	for _, model := range models {
		if engine.ResolvePriceSource(model, "default") == PriceSourceDefault {
			unpriced = append(unpriced, model)
		}
	}
	return unpriced, nil
}

// UnpricedServedModelsInUse reports upstream models that served traffic but have
// no configured price.
//
// Aligning our charges with the provider invoice means pricing the model the
// provider actually served. A channel can rewrite the client-facing alias (罗总
// turns deepseek-v4.1-flash into volcengine/deepseek-v4-1-flash-261017), and if
// that upstream model is unpriced the charge silently falls back to the alias
// price or the default rate card - either way it is a guess.
func UnpricedServedModelsInUse(repo *storage.Repository, engine *BillingEngine, window time.Duration, resolve func(channel, model string) string) ([]string, error) {
	if repo == nil || engine == nil || resolve == nil {
		return nil, nil
	}
	pairs, err := repo.DistinctRecentModelChannels(time.Now().Add(-window), 200)
	if err != nil {
		return nil, err
	}
	seen := make(map[string]struct{})
	unpriced := make([]string, 0, len(pairs))
	for _, pair := range pairs {
		requested, channel := pair[0], pair[1]
		served := resolve(channel, requested)
		if served == "" || served == requested {
			continue
		}
		if engine.ResolvePriceSource(served, "default") != PriceSourceDefault {
			continue
		}
		entry := served + " (served for " + requested
		if channel != "" {
			entry += " on " + channel
		}
		entry += ")"
		if _, duplicate := seen[entry]; duplicate {
			continue
		}
		seen[entry] = struct{}{}
		unpriced = append(unpriced, entry)
	}
	return unpriced, nil
}
