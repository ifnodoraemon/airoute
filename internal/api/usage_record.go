package api

import (
	"context"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/ifnodoraemon/airoute/internal/middleware"
	"github.com/ifnodoraemon/airoute/internal/storage"
)

// RecordUsage executes the cross-cutting concern of calculating billing cost (with off-peak discount and cached tokens),
// emitting telemetry metrics, injecting billing headers, and asynchronously recording audit logs.
func RecordUsage(c *gin.Context, params AuditRecordParams) float64 {
	cost, _, _ := recordUsage(c, params)
	return cost
}

// servedModelResolver maps (serving channel, requested model) to the upstream
// model name that was actually called. Installed by the server at startup.
var servedModelResolver func(channel, model string) string

// SetServedModelResolver installs the served-model lookup used for billing.
func SetServedModelResolver(resolver func(channel, model string) string) {
	servedModelResolver = resolver
}

// recordUsage is the full-fidelity variant, also returning the saved cost and
// cached token count so streaming handlers can emit them as a trailer.
func recordUsage(c *gin.Context, params AuditRecordParams) (float64, float64, int) {
	record, cost, savedCost, cached := prepareUsage(c, params)
	protectUsageQuota(c, params.Quota, record)
	settleUsage(c, record)
	return cost, savedCost, cached
}

// RecordUsageDeferred is RecordUsage for streaming responses.
//
// It calculates cost and headers and journals reserved usage before returning a
// settle closure. Handlers flush the usage trailer and terminal frame before
// calling settle, so waiting for the settlement writer does not delay [DONE].
func RecordUsageDeferred(c *gin.Context, params AuditRecordParams) (cost float64, savedCost float64, cachedTokens int, settle func()) {
	record, cost, savedCost, cachedTokens := prepareUsage(c, params)
	protectUsageQuota(c, params.Quota, record)
	return cost, savedCost, cachedTokens, func() { settleUsage(c, record) }
}

// settleUsage enqueues audit evidence and submits the charge. Its named
// reservation is released in the same transaction as the debit.
func settleUsage(c *gin.Context, record *storage.UsageLogRecord) {
	if record == nil {
		return
	}

	// Pin the idempotency key before anything can duplicate the record: the audit
	// copy below and the ledger row must carry the same key, otherwise a redelivery
	// would settle under a different one and charge twice.
	if record.BillingID == "" {
		record.BillingID = storage.NewBillingID()
	}

	// Audit evidence first. The pipeline is the at-least-once durability path; if
	// the process dies between here and the settlement below, the replayed record
	// settles the charge and the audit row is never lost. The copy matters: both
	// the pipeline and the batcher mutate the record they receive, and the request
	// goroutine must keep ownership of its own.
	if storage.GlobalAsyncLogger != nil {
		storage.GlobalAsyncLogger.Record(record.CloneForPipeline())
	}

	// Zero-cost records (failed dispatches, 4xx denials, free reads) have no money
	// to move; the audit row above is the evidence.
	if record.Cost <= 0 && record.TotalTokens <= 0 && record.ReservationID == "" {
		return
	}

	// Group-commit settlement: the wait is short on purpose. When it expires the
	// charge is already on the audit pipeline and is settled there; only the timing
	// changes, never the amount or the idempotency guarantee. The ledger's unique
	// billing_id makes this settlement and the pipeline's retry apply exactly once,
	// whichever runs first.
	settleCtx, cancelSettle := context.WithTimeout(context.Background(), storage.SettlementWaitTimeout())
	_ = storage.SettleChargeBatched(settleCtx, record)
	cancelSettle()
}

// ceilMillis converts a duration to milliseconds, rounding up so that a
// sub-millisecond exchange still shows as 1ms instead of a misleading 0.
func ceilMillis(d time.Duration) int64 {
	if d <= 0 {
		return 0
	}
	ms := d.Milliseconds()
	if ms == 0 {
		return 1
	}
	return ms
}

func protectUsageQuota(c *gin.Context, quota *middleware.RequestQuota, record *storage.UsageLogRecord) {
	if quota == nil {
		quota = middleware.RequestQuotaFrom(c)
	}
	if quota != nil && record.ReservationID != "" && record.ReservationID != quota.Reservation.ID {
		quota = nil
	}
	quota.Pend(record)
	if quota == nil && record.ReservationID != "" && storage.GetGlobalRepository() != nil {
		ctx, cancel := context.WithTimeout(context.Background(), storage.SettlementWaitTimeout())
		defer cancel()
		_ = storage.GetGlobalRepository().PendBillingRecord(ctx, record)
	}
}
