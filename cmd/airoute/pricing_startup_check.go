package main

import (
	"strings"
	"time"

	"github.com/ifnodoraemon/airoute/internal/billing"
	"github.com/ifnodoraemon/airoute/internal/router"
	"github.com/ifnodoraemon/airoute/internal/storage"
	"github.com/ifnodoraemon/airoute/internal/telemetry"
)

// pricingAuditWindow is how far back the startup pricing-coverage checks look.
const pricingAuditWindow = 7 * 24 * time.Hour

// warnUnpricedModels reports models that served traffic recently but have no price
// rule: the fallback rate card bills them, which is a guess in both directions.
//
// It is deliberately independent from warnUnpricedServedModels: the two checks
// answer different questions ("is the alias priced?" vs "is the model the upstream
// provider actually served priced?"), and chaining them behind a shared early
// return is what kept the second one from ever running - the alias being priced is
// the normal case, so `len(unpriced) == 0` returned first every time.
func warnUnpricedModels(repo *storage.Repository, window time.Duration) {
	if repo == nil {
		return
	}
	unpriced, err := billing.UnpricedModelsInUse(repo, billing.GlobalEngine, window)
	if err != nil {
		telemetry.ScopedLogger("billing").Warn("pricing coverage check failed", "error", err.Error())
		return
	}
	if len(unpriced) == 0 {
		return
	}
	shown := unpriced
	if len(shown) > 20 {
		shown = shown[:20]
	}
	telemetry.ScopedLogger("billing").Warn("models served without a configured price; the fallback rate card is in use",
		"count", len(unpriced),
		"models", strings.Join(shown, ","))
}

// warnUnpricedServedModels reports the upstream models the provider actually served
// that have no price of their own.
//
// Billing aligns with the provider invoice only if the served model is priced:
// channel model mappings can turn a priced alias into an unpriced upstream model,
// and that is precisely the distortion the by-served-model billing was introduced
// to catch.
func warnUnpricedServedModels(repo *storage.Repository, dispatcher *router.Dispatcher, window time.Duration) {
	if repo == nil || dispatcher == nil {
		return
	}
	servedUnpriced, err := billing.UnpricedServedModelsInUse(repo, billing.GlobalEngine, window, dispatcher.ResolveServedModel)
	if err != nil {
		telemetry.ScopedLogger("billing").Warn("served-model pricing check failed", "error", err.Error())
		return
	}
	if len(servedUnpriced) == 0 {
		return
	}
	shown := servedUnpriced
	if len(shown) > 10 {
		shown = shown[:10]
	}
	telemetry.ScopedLogger("billing").Warn("upstream models served without their own price; charges fall back to the alias rate",
		"count", len(servedUnpriced),
		"served_models", strings.Join(shown, "; "))
}
