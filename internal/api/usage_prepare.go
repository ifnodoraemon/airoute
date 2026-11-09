package api

import (
	"time"

	"github.com/gin-gonic/gin"
	"github.com/ifnodoraemon/airoute/internal/middleware"
	"github.com/ifnodoraemon/airoute/internal/model"
	"github.com/ifnodoraemon/airoute/internal/storage"
)

// prepareUsage does everything except moving money.
func prepareUsage(c *gin.Context, params AuditRecordParams) (*storage.UsageLogRecord, float64, float64, int) {
	keyGroup := params.KeyGroup
	if keyGroup == "" {
		keyGroup = getKeyGroup(c)
	}
	params = normalizeUsageStatus(c, params)
	success := params.StatusCode >= 200 && params.StatusCode < 400
	pricing := prepareUsagePricing(params, keyGroup, success)
	totTokens := params.TotalTokens
	if totTokens == 0 {
		totTokens = params.PromptTokens + params.CompletionTokens
	}
	observeUsageConsumption(c, params, totTokens, success)
	record := newUsageRecord(c, params, totTokens, pricing)
	publishUsageOutcome(c, params, pricing, success)
	return record, pricing.cost, pricing.saved, params.CachedTokens
}

func normalizeUsageStatus(c *gin.Context, params AuditRecordParams) AuditRecordParams {
	// A unary request whose customer has gone away is still billed (the provider
	// did the work) but recorded as 499 rather than a success: the status must say
	// what actually happened, independent of the billing decision.
	if c != nil && c.Request != nil && c.Request.Context().Err() != nil &&
		params.StatusCode >= 200 && params.StatusCode < 400 {
		params.StatusCode = statusClientClosed
	}
	// A 499 audit row without a message is unreadable: 499 exists only in the
	// gateway's audit trail, so the row itself has to say who caused it and what
	// the provider had already done. Synthesize the explanation here rather than
	// in each handler, because both the ctx-canceled flip above and handlers that
	// pass 499 directly (a drained stream, a dispatch cancellation) end up in
	// this recorder.
	if params.StatusCode == statusClientClosed && params.ErrorMessage == "" {
		params.ErrorMessage = clientClosedAuditReason(params)
	}
	return params
}

func newUsageRecord(c *gin.Context, params AuditRecordParams, totTokens int, pricing usagePricing) *storage.UsageLogRecord {
	tenantID := params.TenantID
	if tenantID == "" && c != nil {
		tenantID = c.GetString("tenant_id")
		if tenantID == "" {
			tenantID = c.GetString(middleware.ContextKeyTenant)
		}
	}
	traceID := params.TraceID
	if traceID == "" {
		traceID = middleware.GetTraceID(c)
	}
	apiKey := params.APIKey
	if apiKey == "" {
		apiKey = getRequestAPIKey(c)
	}
	var thinkingObservation *model.ThinkingObservation
	if c != nil && c.Request != nil {
		thinkingObservation = model.ThinkingTraceFromContext(c.Request.Context()).Snapshot()
	}
	record := &storage.UsageLogRecord{
		Thinking:         thinkingObservation,
		BillingID:        params.BillingID,
		ReservationID:    params.ReservationID,
		TraceID:          traceID,
		ChatID:           params.ChatID,
		Channel:          params.Channel,
		SessionID:        params.SessionID,
		APIKey:           apiKey,
		TenantID:         tenantID,
		Model:            params.Model,
		PromptTokens:     params.PromptTokens,
		CompletionTokens: params.CompletionTokens,
		CachedTokens:     params.CachedTokens,
		TotalTokens:      totTokens,
		Cost:             pricing.cost,
		PricingSnapshot:  pricing.snapshot,
		IsOffPeak:        pricing.offPeak,
		OffPeakDiscount:  pricing.discount,
		DurationMs:       ceilMillis(params.Duration),
		TTFTMs:           ceilMillis(params.TTFT),
		StatusCode:       params.StatusCode,
		UsageSource:      params.UsageSource,
		// Persist the same scrubbed text the client receives. Upstream failures
		// routinely echo the offending credential back ("Incorrect API key
		// provided: sk-..."), and usage_logs is read by operators and support
		// staff, so the raw provider message must never land in the audit trail.
		ErrorMessage: auditErrorMessage(params.ErrorMessage),
		// Stamp the moment the request finished. Leaving it zero made the writer
		// use the flush time, so a batch of slow requests aborted together all
		// shared one timestamp and their real order was lost.
		CreatedAt: time.Now(),
	}

	return record
}
