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
