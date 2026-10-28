package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/ifnodoraemon/airoute/internal/billing"
	"github.com/ifnodoraemon/airoute/internal/middleware"
	"github.com/ifnodoraemon/airoute/internal/model"
	"github.com/ifnodoraemon/airoute/internal/provider"
	"github.com/ifnodoraemon/airoute/internal/router"
	"github.com/ifnodoraemon/airoute/internal/storage"
	"github.com/ifnodoraemon/airoute/internal/telemetry"
)

// AuditRecordParams encapsulates parameters required to record usage, calculate billing,
// and update system telemetry. Applying Separation of Concerns (SoC) and DRY.
type AuditRecordParams struct {
	SessionID        string
	ChatID           string
	Channel          string
	Model            string
	PromptTokens     int
	CompletionTokens int
	CachedTokens     int
	// CacheCreationTokens is how many of PromptTokens were written into the
	// provider's prompt cache (Anthropic's cache_creation_input_tokens). They are
	// billed at the cache-write premium instead of the plain prompt rate.
	CacheCreationTokens int
	TotalTokens         int
	Duration            time.Duration
	TTFT                time.Duration
	StatusCode          int
	UsageSource         string // "upstream" (default) or "estimated" when tokens are locally approximated
	LiveTracked         bool   // true when streaming deltas already fed the live token meter
	ErrorMessage        string // populated for failed requests so the audit log explains the failure
	BillableUnits       int
	SkipBilling         bool

	// Caller attribution overrides for non-HTTP entrypoints.
	//
	// The MCP JSON-RPC transport has no gin context to derive the identity from,
	// yet its usage must be attributed (and settled) exactly like /v1/* traffic:
	// the old MCP path wrote a hand-built record with a fixed "mcp-session" key and
	// no pricing snapshot. Empty values fall back to the gin context, so HTTP
	// callers stay unchanged.
	APIKey    string
	TenantID  string
	KeyGroup  string
	TraceID   string
	KeyConfig *model.APIKeyConfig // carries the per-key TPM budget for consumption
}

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

// settleCostReporter reports the cost actually settled for a request back to the
// admission layer, so the in-flight wallet pre-authorisation is released at the
// real amount instead of the conservative estimate. Installed by the server at
// startup (the api package does not own the reservation bookkeeping).
var settleCostReporter func(c *gin.Context, cost float64)

// SetSettledCostReporter installs the callback used by settleUsage.
func SetSettledCostReporter(reporter func(c *gin.Context, cost float64)) {
	settleCostReporter = reporter
}

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

// statusClientClosed is the de-facto "client closed request" status recorded in
// the audit trail when the downstream disconnects mid-stream. It is not part of
// net/http's status constants because no server in the Go standard library
// generates it.
const statusClientClosed = 499

// clientClosedAuditReasonPrefix is the stable opening of every synthesized 499
// explanation. Operators and the console key on the phrase, so it must not be
// reworded without updating that contract.
const clientClosedAuditReasonPrefix = "client closed request"

// clientClosedAuditReason explains a 499 audit row in plain English.
//
// A 499 is synthesized by Airoute: no upstream provider returns it, and a
// canceled downstream request can still have produced (and billed) a full
// provider response. Persisting a bare 499 with an empty error_message made the
// console blame the provider for a client-side abort and left operators unable
// to tell whether the gateway, the provider, or the customer caused it. The text
// this returns states the gateway origin, reports what the provider had already
// produced, and never claims an upstream failure.
//
// The message must stay credential-free and survive auditErrorMessage: callers
// persist it through that scrubber.
func clientClosedAuditReason(params AuditRecordParams) string {
	if params.StatusCode != statusClientClosed {
		return ""
	}
	var b strings.Builder
	b.WriteString(clientClosedAuditReasonPrefix)
	b.WriteString(": 499 is synthesized by the gateway for a downstream disconnect and is not a status code returned by the upstream provider")

	// Prompt tokens count as usage too: a dispatch-time cancellation bills the
	// estimated prompt the provider already accepted, and reporting "no usage"
	// there would contradict the bill.
	hasTokens := params.PromptTokens > 0 || params.CompletionTokens > 0
	hasUnits := params.BillableUnits > 0
	switch {
	case hasTokens && hasUnits:
		fmt.Fprintf(&b, "; the provider had already produced %d completion tokens and %d prompt tokens (plus %d billable units) before the client left, and the usage was billed",
			params.CompletionTokens, params.PromptTokens, params.BillableUnits)
	case hasTokens:
		fmt.Fprintf(&b, "; the provider had already produced %d completion tokens and %d prompt tokens before the client left, and the usage was billed",
			params.CompletionTokens, params.PromptTokens)
	case hasUnits:
		fmt.Fprintf(&b, "; the provider had already produced %d billable units before the client left, and the usage was billed",
			params.BillableUnits)
	default:
		b.WriteString("; the downstream disconnected before any provider usage was received")
	}
	if hasTokens && params.UsageSource == usageSourceEstimated {
		// The upstream never reported usage, so the counts above come from the
		// local estimator. Saying so keeps an estimated bill auditable.
		b.WriteString(" (the token counts are estimated because the upstream did not report usage)")
	}
	return b.String()
}

// recordUsage is the full-fidelity variant, also returning the saved cost and
// cached token count so streaming handlers can emit them as a trailer.
func recordUsage(c *gin.Context, params AuditRecordParams) (float64, float64, int) {
	record, cost, savedCost, cached := prepareUsage(c, params)
	settleUsage(c, record)
	return cost, savedCost, cached
}

// RecordUsageDeferred is RecordUsage for streaming responses.
//
// It performs every part of the accounting that the client can observe (cost
// calculation, X-Airoute-* headers, audit enqueue) and returns a settle
// closure. Handlers write the usage trailer and the protocol's terminal frame,
// flush them, and only then call settle. The money movement still happens on
// the request goroutine, but after the last byte reached the client, so a
// saturated settlement writer can no longer delay [DONE]. That tail latency is
// what the audit measured (up to 1.5 s when the old synchronous settlement
// timed out).
func RecordUsageDeferred(c *gin.Context, params AuditRecordParams) (cost float64, savedCost float64, cachedTokens int, settle func()) {
	record, cost, savedCost, cachedTokens := prepareUsage(c, params)
	return cost, savedCost, cachedTokens, func() { settleUsage(c, record) }
}

// prepareUsage does everything except moving money.
func prepareUsage(c *gin.Context, params AuditRecordParams) (*storage.UsageLogRecord, float64, float64, int) {
	var cost, savedCost float64
	var pricingSnapshot *storage.UsagePricingSnapshot
	var isOffPeak bool
	var offPeakDiscount float64 = 1.0
	keyGroup := params.KeyGroup
	if keyGroup == "" {
		keyGroup = getKeyGroup(c)
	}
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
	success := params.StatusCode >= 200 && params.StatusCode < 400

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
		cost, savedCost, _, isOffPeak, offPeakDiscount, pricingSnapshot = billing.GlobalEngine.CalculateCostForUnitsWithCacheCreation(
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
		if normalizedCost, normalizedSaved, unbillable := normalizeUnbillableCurrency(cost, savedCost, pricingSnapshot); unbillable {
			cost, savedCost = normalizedCost, normalizedSaved
			isOffPeak, offPeakDiscount = false, 1.0
		}
	}

	totTokens := params.TotalTokens
	if totTokens == 0 {
		totTokens = params.PromptTokens + params.CompletionTokens
	}

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
	record := &storage.UsageLogRecord{
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
		Cost:             cost,
		PricingSnapshot:  pricingSnapshot,
		IsOffPeak:        isOffPeak,
		OffPeakDiscount:  offPeakDiscount,
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
		c.Header("X-Airoute-Cost", fmt.Sprintf("%.6f", cost))
		if isOffPeak {
			c.Header("X-Airoute-Off-Peak", "true")
			c.Header("X-Airoute-Off-Peak-Discount", fmt.Sprintf("%.2f", offPeakDiscount))
		}
		if params.CachedTokens > 0 {
			c.Header("X-Airoute-Cached-Tokens", fmt.Sprintf("%d", params.CachedTokens))
			c.Header("X-Airoute-Saved-Cost", fmt.Sprintf("%.6f", savedCost))
		}
	}

	return record, cost, savedCost, params.CachedTokens
}

// settleUsage moves the prepared record's money and reports the actual cost to
// the admission layer so its in-flight pre-authorisation can be released at the
// real amount instead of the conservative estimate.
func settleUsage(c *gin.Context, record *storage.UsageLogRecord) {
	if record == nil {
		return
	}
	if settleCostReporter != nil {
		settleCostReporter(c, record.Cost)
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
	if record.Cost <= 0 && record.TotalTokens <= 0 {
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

// topUpQuotaForRequest refines the admission-time wallet pre-authorisation with a
// conservative cost estimate, now that the request body has been parsed.
//
// Authentication runs before the body is read, so it can only reserve a coarse
// constant. At dispatch the model, prompt size and max_tokens are known, and the
// database advertises the prompt's true cost; topping the reservation up closes
// the gap between "the wallet is positive" and "the wallet covers this request".
// It is best-effort: a failed top-up leaves the admission-time amount in place
// and never fails the request.
func topUpQuotaForRequest(c *gin.Context, req *model.ChatCompletionRequest) {
	if c == nil || req == nil {
		return
	}
	maxTokens := 0
	if req.MaxTokens != nil {
		maxTokens = *req.MaxTokens
	}
	estimate := billing.EstimateCostCNY(req.Model, getKeyGroup(c), req.EstimatedPromptTokens(), maxTokens)
	if estimate <= 0 {
		return
	}
	middleware.TopUpUserQuota(c, estimate)
}

// writeStreamUsageTrailer appends the billing metadata as an SSE comment so
// streaming clients still receive it after the response head was flushed.
// SSE comment lines start with ':' and are ignored by every compliant client.
func writeStreamUsageTrailer(w io.Writer, flusher http.Flusher, cost, savedCost float64, cachedTokens int) {
	if w == nil {
		return
	}
	line := fmt.Sprintf(": airoute-cost=%.6f saved-cost=%.6f cached-tokens=%d\n\n", cost, savedCost, cachedTokens)
	if _, err := io.WriteString(w, line); err != nil {
		return
	}
	if flusher != nil {
		flusher.Flush()
	}
}

// RespondOpenAIError returns a standardized OpenAI API error JSON response.
func RespondOpenAIError(c *gin.Context, statusCode int, message, errType, code string) {
	if errType == "" {
		errType = "invalid_request_error"
	}
	c.JSON(statusCode, gin.H{
		"error": gin.H{
			"message": message,
			"type":    errType,
			"code":    code,
		},
	})
}

// RespondAnthropicError returns a standardized Anthropic API error JSON response.
func RespondAnthropicError(c *gin.Context, statusCode int, message, errType string) {
	if errType == "" {
		errType = "invalid_request_error"
	}
	c.JSON(statusCode, gin.H{
		"type": "error",
		"error": gin.H{
			"type":    errType,
			"message": message,
		},
	})
}

// RespondGeminiError returns a standardized Google Gemini API error JSON response.
func RespondGeminiError(c *gin.Context, statusCode int, message, status string) {
	c.JSON(statusCode, gin.H{
		"error": gin.H{
			"code":    statusCode,
			"message": message,
			"status":  status,
		},
	})
}

// InitSSEStream configures standard Server-Sent Events headers and flushes the writer.
func InitSSEStream(c *gin.Context) (http.Flusher, bool) {
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("X-Accel-Buffering", "no")
	c.Status(http.StatusOK)

	// Remove WriteTimeout deadline for SSE streaming so long-running responses
	// (> 180s) won't be terminated mid-stream by http.Server.WriteTimeout.
	rc := http.NewResponseController(c.Writer)
	_ = rc.SetWriteDeadline(time.Time{})

	flusher, ok := c.Writer.(http.Flusher)
	if ok {
		flusher.Flush()
	}
	return flusher, ok
}

// getKeyGroup extracts key group name from context.
func getKeyGroup(c *gin.Context) string {
	if c == nil {
		return "default"
	}
	if kAny, exists := c.Get(middleware.ContextKeyAPIKeyConfig); exists {
		if k, ok := kAny.(*model.APIKeyConfig); ok && k.GroupName != "" {
			return k.GroupName
		}
	}
	return "default"
}

// getRequestAPIKey extracts incoming API key token string.
func getRequestAPIKey(c *gin.Context) string {
	if c == nil {
		return ""
	}
	return c.GetString(middleware.ContextKeyAPIKey)
}

// channelFromError recovers the channel chain behind a dispatch failure from
// structured error types only.
//
// The router already tracks this fact: a served request reports the chain in
// Response.Channel, and a terminal failure reports it in router.DispatchError.
// Scraping Error() text is not a substitute for either - the message is prose,
// and a phrase like "no upstream provider available for requested model 'x'"
// would be read back as a channel named "provider".
func channelFromError(err error) string {
	if err == nil {
		return ""
	}
	// A dispatch failure owns the full routing chain, including channels tried
	// by earlier hops of a cross-model fallback.
	if de, ok := router.AsDispatchError(err); ok && de.Trail != "" {
		return de.Trail
	}
	// A single upstream rejection names the channel that produced it.
	if ue, ok := provider.AsUpstreamError(err); ok {
		return ue.Channel
	}
	return ""
}

// recordFailedRequestWithError audits an unsuccessful API dispatch: why it
// failed and which upstream channel was involved. Callers must always pass the
// dispatch error — recording only a status code left operators staring at a 502
// row with an empty message.
func recordFailedRequestWithError(c *gin.Context, sessionID, modelName string, dur time.Duration, statusCode int, err error) {
	msg := ""
	channel := ""
	if err != nil {
		msg = err.Error()
		channel = channelFromError(err)
	}
	params := AuditRecordParams{
		SessionID:    sessionID,
		Model:        modelName,
		Channel:      channel,
		Duration:     dur,
		StatusCode:   statusCode,
		ErrorMessage: msg,
	}
	// A client that disconnects during dispatch already had its prompt accepted
	// by the upstream. The dispatcher attaches the request's estimated prompt
	// size to the cancellation so the provider's cost is not absorbed by us.
	if statusCode == statusClientClosed {
		if de, ok := router.AsDispatchError(err); ok && de.PromptTokens > 0 {
			// The dispatcher can only estimate from the request body; route it
			// through the same calibrator the other estimated charges use, or an
			// aborted request is billed on a tokenizer guess the rest of the system
			// has already learned to correct.
			estimated := calibratedPromptEstimate(modelName, de.PromptTokens)
			params.PromptTokens = estimated
			params.TotalTokens = estimated
			params.UsageSource = usageSourceEstimated
		}
	}
	RecordUsage(c, params)
}

// getAPIKeyConfig returns APIKeyConfig metadata bound to the context.
func getAPIKeyConfig(c *gin.Context) *model.APIKeyConfig {
	if c == nil {
		return nil
	}
	if val, exists := c.Get(middleware.ContextKeyAPIKeyConfig); exists {
		if cfg, ok := val.(*model.APIKeyConfig); ok {
			return cfg
		}
	}
	return nil
}

// estimateTokens is the single token-estimation heuristic used by every audit
// path that has to approximate usage because the upstream did not report it.
// ~4 bytes per token is the widely used rule of thumb for mixed English/JSON
// payloads. Previously each handler divided by 3 or 4 locally, so the same
// request could be billed differently depending on which endpoint served it.
func estimateTokens(chars int) int {
	return model.EstimateTokensFromChars(chars)
}

// modelNameOf returns the requested model name of a chat request.
func modelNameOf(req *model.ChatCompletionRequest) string {
	if req == nil {
		return ""
	}
	return req.Model
}

// observePromptEstimate records an upstream-reported prompt count against our local
// estimate, calibrating the estimator for that model.
func observePromptEstimate(modelName string, localEstimate, upstreamActual int) {
	billing.GlobalCalibrator.Observe(modelName, localEstimate, upstreamActual)
}

// calibratedPromptEstimate applies the learned upstream-tokenizer ratio to a local
// estimate. It is a no-op until enough upstream reports have been observed.
func calibratedPromptEstimate(modelName string, localEstimate int) int {
	return billing.GlobalCalibrator.Adjust(modelName, localEstimate)
}

// streamFallbackPromptTokens is the prompt fallback every streaming handler uses
// when the upstream never reported usage.
//
// It goes through the canonical estimator (message text, multimodal parts and
// tool definitions) exactly like the unary path, so the same request is billed
// identically whether it streamed or not. The chat stream handler used to count
// message text only, which silently dropped the tools block from the bill.
func streamFallbackPromptTokens(req *model.ChatCompletionRequest) int {
	return calibratedPromptEstimate(modelNameOf(req), estimateCanonicalPromptTokens(req))
}

// estimatedStreamCompletionTokens converts accumulated streamed output characters
// into completion tokens.
//
// This must stay on the shared estimator: the Anthropic adapter stream used a
// local "/3" while every other endpoint used the shared "/4" heuristic, so the
// same text was billed one third higher on that endpoint alone.
func estimatedStreamCompletionTokens(chars int) int {
	if chars <= 0 {
		return 0
	}
	tokens := estimateTokens(chars)
	if tokens < 1 {
		return 1
	}
	return tokens
}

// Usage provenance markers recorded on the audit row.
const (
	usageSourceUpstream  = "upstream"
	usageSourceEstimated = "estimated"
)

// estimateCompletionFromChoices approximates completion tokens from the text an
// upstream actually returned. Covers visible content, reasoning content (billed
// as output by providers that stream it) and tool calls.
func estimateCompletionFromChoices(choices []model.ChatCompletionChoice) int {
	chars := 0
	for i := range choices {
		chars += len(choices[i].Message.GetContentString())
		chars += len(choices[i].Message.ReasoningContent)
		for j := range choices[i].Message.ToolCalls {
			if b, err := json.Marshal(choices[i].Message.ToolCalls[j]); err == nil {
				chars += len(b)
			}
		}
	}
	return estimateTokens(chars)
}

// resolveChatUsage returns the token counts to bill for a unary chat-shaped
// response.
//
// Some upstreams omit the usage block entirely. Billing such a request at zero
// means the gateway pays the provider and charges the customer nothing (148 such
// HTTP-200 rows were observed, ~¥2.2). The streaming handlers already fall back
// to estimation; unary responses now use the same heuristic and mark the audit
// row usage_source=estimated so the approximation stays visible.
func resolveChatUsage(req *model.ChatCompletionRequest, resp *model.ChatCompletionResponse) (promptTokens, completionTokens, cachedTokens int, source string) {
	source = usageSourceUpstream
	localEstimate := estimateCanonicalPromptTokens(req)
	if resp != nil && resp.Usage != nil {
		promptTokens = resp.Usage.PromptTokens
		completionTokens = resp.Usage.CompletionTokens
		cachedTokens = resp.Usage.GetCachedTokens()
	}
	if promptTokens == 0 {
		if estimated := calibratedPromptEstimate(modelNameOf(req), localEstimate); estimated > 0 {
			promptTokens = estimated
			source = usageSourceEstimated
		}
	} else {
		// Learn how the upstream tokenizer counts this prompt so later estimates
		// (client aborts, upstreams that omit usage) converge on its accounting.
		observePromptEstimate(modelNameOf(req), localEstimate, promptTokens)
	}
	if completionTokens == 0 && resp != nil {
		if estimated := estimateCompletionFromChoices(resp.Choices); estimated > 0 {
			completionTokens = estimated
			source = usageSourceEstimated
		}
	}
	return promptTokens, completionTokens, cachedTokens, source
}

// resolveEmbeddingTokens approximates prompt tokens for an embedding request when
// the upstream returned no usage. String inputs are measured by length; token-id
// arrays are already token counts.
//
// C-04: the estimate goes through the per-model calibrator like every chat
// estimate does, and a real upstream count feeds the calibrator instead of only
// being billed. Before this the two modality estimators were the only prompt
// estimates in the gateway that ignored the learned tokenizer ratio, so the same
// text was billed differently depending on whether the upstream reported usage.
func resolveEmbeddingTokens(req *model.EmbeddingRequest, usage model.EmbeddingUsage) (int, string) {
	local, exact := localEmbeddingPromptEstimate(req)
	if usage.PromptTokens > 0 {
		if !exact {
			observePromptEstimate(req.Model, local, usage.PromptTokens)
		}
		return usage.PromptTokens, usageSourceUpstream
	}
	if local <= 0 {
		return 0, usageSourceUpstream
	}
	if exact {
		// Token-id input is already an exact count; calibrating it would
		// double-apply the tokenizer ratio.
		return local, usageSourceEstimated
	}
	return calibratedPromptEstimate(req.Model, local), usageSourceEstimated
}

// localEmbeddingPromptEstimate counts the local estimate for an embedding input
// without applying the tokenizer calibration. exact reports that the input was
// already a token-id array, so no ratio applies.
func localEmbeddingPromptEstimate(req *model.EmbeddingRequest) (tokens int, exact bool) {
	if req == nil {
		return 0, false
	}
	if inputs := req.GetInputStrings(); len(inputs) > 0 {
		chars := 0
		for _, input := range inputs {
			chars += len(input)
		}
		return estimateTokens(chars), false
	}
	// Token-id form: one array element is one token.
	if ids, ok := req.Input.([]any); ok && len(ids) > 0 {
		return len(ids), true
	}
	return 0, false
}

// resolveRerankTokens approximates prompt tokens for a rerank request (query plus
// every candidate document) when the upstream returned no usage.
//
// C-04: same calibration contract as embedding and chat — estimate, learn from a
// real upstream count, otherwise apply the learned ratio.
func resolveRerankTokens(req *model.RerankRequest, usage model.RerankUsage) (int, string) {
	local := localRerankPromptEstimate(req)
	if usage.TotalTokens > 0 {
		observePromptEstimate(req.Model, local, usage.TotalTokens)
		return usage.TotalTokens, usageSourceUpstream
	}
	if local <= 0 {
		return 0, usageSourceUpstream
	}
	return calibratedPromptEstimate(req.Model, local), usageSourceEstimated
}

// localRerankPromptEstimate counts the local estimate for a rerank request.
func localRerankPromptEstimate(req *model.RerankRequest) int {
	if req == nil {
		return 0
	}
	chars := len(req.Query)
	for _, doc := range req.GetDocumentTexts() {
		chars += len(doc)
	}
	return estimateTokens(chars)
}

// estimateCanonicalPromptTokens approximates prompt tokens from the canonical
// request when the upstream did not report usage. Only used for audit display.
func estimateCanonicalPromptTokens(req *model.ChatCompletionRequest) int {
	if req == nil {
		return 0
	}
	chars := 0
	for _, m := range req.Messages {
		chars += len(m.GetContentString())
	}
	for _, t := range req.Tools {
		if b, err := json.Marshal(t); err == nil {
			chars += len(b)
		}
	}
	return estimateTokens(chars)
}

// Credential patterns scrubbed from upstream error bodies before they are
// returned to a downstream client. Upstream 401/400 responses routinely echo the
// offending credential back ("Incorrect API key provided: sk-abc..."), and the
// old key=value-only regex missed those entirely.
//
// Deliberately explicit rather than "any long random-looking word": error
// messages also contain model names and chat IDs, which must stay readable.
var (
	sensitivePattern = regexp.MustCompile(`(?i)\b(api[-_]?key|access[-_]?token|secret|password|passwd)(["']?\s*[:=]\s*["']?)([^\s"',}\]]{4,})`)
	bearerPattern    = regexp.MustCompile(`(?i)\b(bearer)\s+([A-Za-z0-9_\-\.\+=/]{8,})`)
	providerKeyRegex = regexp.MustCompile(`\b(sk-[A-Za-z0-9_\-]{8,}|gsk_[A-Za-z0-9]{8,}|xai-[A-Za-z0-9]{8,}|hf_[A-Za-z0-9]{8,}|AIza[0-9A-Za-z_\-]{20,})`)
)

// auditErrorMessage is the audit-trail counterpart of sanitizeErrorMessage. The
// error text persisted in usage_logs.error_message must be scrubbed exactly like
// the text the client receives: it is read by operators, exported to CSV and
// shown in the console, so upstream providers that echo the offending credential
// must not leak it through the billing trail.
func auditErrorMessage(message string) string {
	if message == "" {
		return ""
	}
	return sanitizeErrorMessage(fmt.Errorf("%s", message))
}

// sanitizeErrorMessage strips sensitive tokens/keys and credentials from error strings before returning to clients.
func sanitizeErrorMessage(err error) string {
	if err == nil {
		return ""
	}
	msg := err.Error()
	// Credential-shaped tokens first: once a label like "Authorization:" has been
	// rewritten, the token after it would no longer be recognisable.
	msg = bearerPattern.ReplaceAllString(msg, "$1 [REDACTED]")
	msg = providerKeyRegex.ReplaceAllStringFunc(msg, func(s string) string {
		switch {
		case strings.HasPrefix(strings.ToLower(s), "sk-"):
			return "sk-[REDACTED]"
		case strings.HasPrefix(s, "AIza"):
			return "AIza[REDACTED]"
		default:
			// gsk_ / xai- / hf_ keep their recognizable prefix.
			if i := strings.IndexAny(s, "_-"); i > 0 {
				return s[:i+1] + "[REDACTED]"
			}
			return "[REDACTED]"
		}
	})
	msg = sensitivePattern.ReplaceAllString(msg, "$1$2[REDACTED]")
	return msg
}

// resolveUpstreamFailureStatus extracts the authentic underlying status code, error types,
// and failing channel from an upstream dispatch error, preventing misleading 502 masks.
func resolveUpstreamFailureStatus(err error) (statusCode int, errType string, errCode string, channel string) {
	statusCode = http.StatusBadGateway
	errType = "gateway_error"
	errCode = "upstream_failure"
	channel = channelFromError(err)

	if err == nil {
		return statusCode, errType, errCode, channel
	}

	// The gateway's own total budget ran out. This is a gateway-side deadline,
	// but reporting it as a plain upstream_timeout hides that no single provider
	// failed: every candidate was simply still running when we stopped waiting.
	if errors.Is(err, router.ErrRequestBudgetExceeded) {
		return http.StatusGatewayTimeout, "gateway_error", "request_budget_exceeded", channel
	}

	// The request context being canceled means the *downstream* went away (browser
	// tab closed, mobile network drop, client timeout): the dispatcher returns
	// ctx.Err() as soon as it observes it. Reporting that as a 502 upstream
	// failure blamed the provider for the client's behaviour and made every abort
	// count against the gateway's error budget.
	if errors.Is(err, context.Canceled) {
		return statusClientClosed, "client_closed_request", "client_canceled", channel
	}
	// A deadline that we or the transport imposed, on the other hand, is a
	// gateway-side timeout.
	if errors.Is(err, context.DeadlineExceeded) {
		return http.StatusGatewayTimeout, "gateway_error", "upstream_timeout", channel
	}

	// Routing knows for certain when a model maps to no candidate channel at
	// all. That is a client-visible "model not found", not an upstream health
	// problem, and it must not be inferred from the message text.
	if de, ok := router.AsDispatchError(err); ok && de.NoRoute {
		return http.StatusNotFound, "invalid_request_error", "model_not_found", channel
	}

	// 1. Direct or wrapped UpstreamError extraction
	if ue, ok := provider.AsUpstreamError(err); ok {
		if channel == "" && ue.Channel != "" {
			channel = ue.Channel
		}
		if ue.Status > 0 {
			switch ue.Status {
			case http.StatusNotFound:
				return http.StatusNotFound, "invalid_request_error", "model_not_found", channel
			case http.StatusTooManyRequests:
				return http.StatusTooManyRequests, "rate_limit_error", "upstream_rate_limited", channel
			case http.StatusUnauthorized:
				return http.StatusBadGateway, "gateway_error", "upstream_unauthorized", channel
			case http.StatusForbidden:
				return http.StatusBadGateway, "gateway_error", "upstream_forbidden", channel
			case http.StatusBadRequest:
				return http.StatusBadRequest, "invalid_request_error", "invalid_payload", channel
			case http.StatusUnprocessableEntity:
				return http.StatusUnprocessableEntity, "invalid_request_error", "unprocessable_entity", channel
			default:
				if ue.Status >= 500 {
					return ue.Status, "gateway_error", "upstream_server_error", channel
				}
			}
		}
	}

	// 2. Text heuristics for errors the dispatcher did not produce (third-party
	// adapters and unexpected wrappers); routing failures are typed above.
	msg := strings.ToLower(err.Error())
	if strings.Contains(msg, "status 404") || strings.Contains(msg, "model not found") {
		return http.StatusNotFound, "invalid_request_error", "model_not_found", channel
	}
	if strings.Contains(msg, "status 429") || strings.Contains(msg, "rate limit") {
		return http.StatusTooManyRequests, "rate_limit_error", "upstream_rate_limited", channel
	}

	return statusCode, errType, errCode, channel
}

// respondUpstreamDispatchError uniformly formats and audits upstream routing failures,
// preserving the authentic underlying status code and failing channel.
func respondUpstreamDispatchError(c *gin.Context, sessionID, modelName string, dur time.Duration, err error) {
	statusCode, errType, errCode, channel := resolveUpstreamFailureStatus(err)
	RecordTrace(c, modelName, channel, statusCode, err.Error(), "", dur)
	recordFailedRequestWithError(c, sessionID, modelName, dur, statusCode, err)
	safeErr := sanitizeErrorMessage(err)
	RespondOpenAIError(c, statusCode, safeErr, errType, errCode)
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

// normalizeUnbillableCurrency zeroes a computed charge whose pricing snapshot
// cannot settle in the wallet currency. The settlement path rejects such a
// record (and zeroes its audit row); the response header must agree with it.
func normalizeUnbillableCurrency(cost, savedCost float64, snapshot *storage.UsagePricingSnapshot) (float64, float64, bool) {
	if snapshot == nil {
		return cost, savedCost, false
	}
	currency := strings.TrimSpace(snapshot.Currency)
	if currency == "" || strings.EqualFold(currency, "CNY") {
		return cost, savedCost, false
	}
	return 0, 0, true
}

// The wallet settles only CNY. Reject a legacy foreign rate before work starts.
//
// C-14: the gate covers every model settlement may price, not only the requested
// one. When a channel maps the alias to a served model that has its own rate card,
// billing charges that rate card (see billingModelFor), so a non-CNY legacy row
// there would sail past a requested-model-only check and only surface as a zeroed
// charge after the upstream work was already paid for.
func validateModelCurrency(c *gin.Context, modelName string) bool {
	if billing.GlobalEngine == nil {
		return true
	}
	keyGroup := getKeyGroup(c)
	for _, candidate := range currencyModelsForBilling(modelName, keyGroup) {
		if err := storage.ValidatePriceCurrency(billing.GlobalEngine.GetPriceWithGroup(candidate, keyGroup)); err != nil {
			RespondOpenAIError(c, http.StatusUnprocessableEntity, err.Error(), "invalid_request_error", "unsupported_pricing_currency")
			return false
		}
	}
	return true
}
