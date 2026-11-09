package storage

import (
	"errors"
	"github.com/ifnodoraemon/airoute/internal/telemetry"
	"time"
)

// reportUnattributedCharge publishes a settled charge that no wallet could absorb.
//
// The settlement ledger deliberately still records it: the delivery is
// at-least-once, so leaving the row out would re-deliver and re-audit the charge
// forever, and the audit row exists with its cost. What was missing was any
// signal: without this counter and log an operator could not tell that money was
// being served but never collected.
func reportUnattributedCharge(apiKey, tenant string, cost float64) {
	telemetry.GlobalMetrics.BillingUnattributed.Add(1)
	telemetry.WarnLogger(telemetry.ScopedLogger("billing"), "billing-charge-unattributed", 30*time.Second,
		"charge settled with no billable wallet: the amount is uncollected",
		"api_key", apiKey,
		"tenant", tenant,
		"cost_cny", cost)
}

// rejectUnbillableCharge closes out a record whose charge cannot be applied in
// the wallet's currency (a legacy foreign-currency price row).
//
// It is deliberately not an error. The record keeps its real token counts, its
// pricing snapshot and the rejection reason, so the audit trail still explains
// what happened; only the money columns are zeroed, which keeps the settlement
// ledger, the key counters and the audit trail in agreement. The alternative —
// charging a USD number as if it were CNY — overcharges the customer by the
// exchange rate, and returning an error (the previous behaviour) discarded the
// whole batch.
func rejectUnbillableCharge(log *UsageLogRecord, reason error) {
	rejectedCost := log.Cost
	currency := ""
	if log.PricingSnapshot != nil {
		currency = log.PricingSnapshot.Currency
	}
	log.Cost = 0
	log.Settled = true
	if log.ErrorMessage == "" {
		log.ErrorMessage = "billing rejected: " + reason.Error()
	}

	telemetry.GlobalMetrics.BillingChargesRejected.Add(1)
	telemetry.WarnLogger(telemetry.ScopedLogger("billing"), "billing-charge-rejected", 30*time.Second,
		"charge rejected: the pricing snapshot is not settled in CNY; the request is audited but not billed",
		"reason", reason.Error(),
		"currency", currency,
		"model", log.Model,
		"trace_id", log.TraceID,
		"rejected_cost_cny", rejectedCost,
		"total_tokens", log.TotalTokens)
}

// countDeferredSettlement is called when synchronous settlement could not run;
// the charge is still delivered through the async pipeline.
func countDeferredSettlement(err error) {
	telemetry.GlobalMetrics.BillingSettlementsDeferred.Add(1)
	if errors.Is(err, ErrSettlementNotInitialized) {
		return
	}
	telemetry.WarnLogger(telemetry.ScopedLogger("billing"), "settle-deferred", 30*time.Second,
		"synchronous settlement failed; the charge is deferred to the async pipeline",
		"error", err.Error())
}
