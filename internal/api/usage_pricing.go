package api

import (
	"time"

	"github.com/ifnodoraemon/airoute/internal/billing"
	"github.com/ifnodoraemon/airoute/internal/storage"
)

// billingModelFor decides which model's rate card to charge.
//
// The upstream invoices us for the model it served, so billing must follow the
// channel's model mapping (e.g. deepseek-v4.1-flash -> volcengine/...). It only
// does so when the served model has an explicitly configured price: replacing an
// operator's alias price with the fallback rate card would silently change what
// every customer pays.
func billingModelFor(params AuditRecordParams, keyGroup string) string {
	if servedModelResolver == nil || params.Channel == "" || params.Model == "" {
		return params.Model
	}
	served := servedModelResolver(params.Channel, params.Model)
	if served == "" || served == params.Model {
		return params.Model
	}
	if billing.GlobalEngine != nil && billing.GlobalEngine.ResolvePriceSource(served, keyGroup) != billing.PriceSourceDefault {
		return served
	}
	return params.Model
}

type usagePricing struct {
	cost     float64
	saved    float64
	snapshot *storage.UsagePricingSnapshot
	offPeak  bool
	discount float64
}

func prepareUsagePricing(params AuditRecordParams, keyGroup string, success bool) usagePricing {
	pricing := usagePricing{discount: 1.0}
	// Billing is decoupled from the audit status: a streaming client that
	// disconnects after the upstream already produced tokens still consumed
	// (and still costs us) those tokens. Recording 499 without charging made
	// "receive the answer, then drop the connection" a free-usage exploit, and
	// the other three streaming implementations already charged in that case.
	// The audit row keeps the truthful 499 status.
	//
	// Fixed-price units count too: an image or a moderation call that the provider
	// already produced (and billed) must be charged even when the customer went
	// away before the bytes arrived, and those requests report no tokens at all.
	billable := success ||
		(params.StatusCode == statusClientClosed &&
			(params.PromptTokens > 0 || params.CompletionTokens > 0 || params.BillableUnits > 0))

	if billable && !params.SkipBilling && billing.GlobalEngine != nil {
		pricing.cost, pricing.saved, _, pricing.offPeak, pricing.discount, pricing.snapshot = billing.GlobalEngine.CalculateCostForUnitsWithCacheCreation(
			billingModelFor(params, keyGroup),
			keyGroup,
			params.PromptTokens,
			params.CompletionTokens,
			params.CachedTokens,
			params.CacheCreationTokens,
			params.BillableUnits,
			time.Now(),
		)
		// Settlement rejects a legacy foreign-currency price and zeroes the audit
		// row. Do the same before the response headers are written, otherwise
		// X-Airoute-Cost advertises money that was never charged.
		if normalizedCost, normalizedSaved, unbillable := normalizeUnbillableCurrency(pricing.cost, pricing.saved, pricing.snapshot); unbillable {
			pricing.cost, pricing.saved = normalizedCost, normalizedSaved
			pricing.offPeak, pricing.discount = false, 1.0
		}
	}

	return pricing
}
