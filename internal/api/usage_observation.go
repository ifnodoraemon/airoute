package api

import (
	"fmt"

	"github.com/gin-gonic/gin"
	"github.com/ifnodoraemon/airoute/internal/middleware"
	"github.com/ifnodoraemon/airoute/internal/telemetry"
)

func observeUsageConsumption(c *gin.Context, params AuditRecordParams, totTokens int, success bool) {
	// Feed actual consumption back into the per-minute token budget so the next
	// requests from this key are gated by real usage rather than an estimate.
	keyCfg := params.KeyConfig
	if keyCfg == nil {
		keyCfg = getAPIKeyConfig(c)
	}
	if keyCfg != nil && keyCfg.TPM > 0 {
		middleware.GlobalTokenQuota.Consume(keyCfg.Key, keyCfg.TPM, totTokens)
	}

	// A client that hung up is not a gateway failure: counting it as one inflated
	// the 5xx rate and hid real upstream problems behind normal mobile aborts.
	if params.StatusCode == statusClientClosed {
		telemetry.GlobalMetrics.RecordClientCanceled(
			params.Model,
			params.Duration,
			params.PromptTokens,
			params.CompletionTokens,
		)
	} else {
		telemetry.GlobalMetrics.RecordRequestWithModel(
			params.Model,
			success,
			params.Duration,
			params.PromptTokens,
			params.CompletionTokens,
		)
	}

	// The dashboard's live throughput meter is normally fed by the streaming
	// delta hooks. Non-streaming requests have no deltas, so their tokens are
	// added here once the request completes; streaming handlers set LiveTracked
	// so their totals are not counted twice.
	if !params.LiveTracked {
		telemetry.GlobalLiveMetrics.AddTokens(totTokens)
	}

}

func publishUsageOutcome(c *gin.Context, params AuditRecordParams, pricing usagePricing, success bool) {
	// A 499 can live only in the audit trail: on a streaming request the wire
	// already said 200 before the customer hung up, and a unary 2xx is flipped
	// above. Publish the audit status so the access line can show both numbers,
	// and archive the trace the console joins on — a 499 row without a snapshot
	// left operators with an empty Trace ID exactly when they needed it.
	// RecordTrace dedupes per request, so a dispatch failure that already
	// archived its trace is not recorded twice.
	if c != nil && params.StatusCode == statusClientClosed {
		c.Set(middleware.ContextKeyAuditStatus, statusClientClosed)
		RecordTrace(c, params.Model, params.Channel, statusClientClosed, params.ErrorMessage, "", params.Duration)
	}

	// Publish the failure reason for the access log. Every failed dispatch funnels
	// through this recorder, so the same message that lands in usage_logs also
	// lands on the stdout access line instead of forcing an operator to join the
	// database just to learn what the gateway answered.
	if c != nil && params.ErrorMessage != "" {
		c.Set(middleware.ContextKeyErrorMessage, sanitizeErrorMessage(fmt.Errorf("%s", params.ErrorMessage)))
	}

	// Cost headers can only be emitted while the response head is still open.
	// Streaming handlers call RecordUsage after the SSE stream has already been
	// flushed, so writing headers there is a silent no-op; they emit the numbers
	// as a trailing SSE comment instead (see writeStreamUsageTrailer).
	if success && c != nil && !c.Writer.Written() {
		c.Header("X-Airoute-Cost", fmt.Sprintf("%.6f", pricing.cost))
		if pricing.offPeak {
			c.Header("X-Airoute-Off-Peak", "true")
			c.Header("X-Airoute-Off-Peak-Discount", fmt.Sprintf("%.2f", pricing.discount))
		}
		if params.CachedTokens > 0 {
			c.Header("X-Airoute-Cached-Tokens", fmt.Sprintf("%d", params.CachedTokens))
			c.Header("X-Airoute-Saved-Cost", fmt.Sprintf("%.6f", pricing.saved))
		}
	}

}
