package api

import (
	"encoding/json"
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
	TotalTokens      int
	Duration         time.Duration
	TTFT             time.Duration
	StatusCode       int
	UsageSource      string // "upstream" (default) or "estimated" when tokens are locally approximated
	LiveTracked      bool   // true when streaming deltas already fed the live token meter
	ErrorMessage     string // populated for failed requests so the audit log explains the failure
	BillableUnits    int
	SkipBilling      bool
}

// RecordUsage executes the cross-cutting concern of calculating billing cost (with off-peak discount and cached tokens),
// emitting telemetry metrics, injecting billing headers, and asynchronously recording audit logs.
func RecordUsage(c *gin.Context, params AuditRecordParams) float64 {
	cost, _, _ := recordUsage(c, params)
	return cost
}

// statusClientClosed is the de-facto "client closed request" status recorded in
// the audit trail when the downstream disconnects mid-stream. It is not part of
// net/http's status constants because no server in the Go standard library
// generates it.
const statusClientClosed = 499

// recordUsage is the full-fidelity variant, also returning the saved cost and
// cached token count so streaming handlers can emit them as a trailer.
func recordUsage(c *gin.Context, params AuditRecordParams) (float64, float64, int) {
	var cost, savedCost float64
	var pricingSnapshot *storage.UsagePricingSnapshot
	var isOffPeak bool
	var offPeakDiscount float64 = 1.0
	keyGroup := getKeyGroup(c)
	success := params.StatusCode >= 200 && params.StatusCode < 400

	// Billing is decoupled from the audit status: a streaming client that
	// disconnects after the upstream already produced tokens still consumed
	// (and still costs us) those tokens. Recording 499 without charging made
	// "receive the answer, then drop the connection" a free-usage exploit, and
	// the other three streaming implementations already charged in that case.
	// The audit row keeps the truthful 499 status.
	billable := success ||
		(params.StatusCode == statusClientClosed && (params.PromptTokens > 0 || params.CompletionTokens > 0))

	if billable && !params.SkipBilling && billing.GlobalEngine != nil {
		cost, savedCost, _, isOffPeak, offPeakDiscount, pricingSnapshot = billing.GlobalEngine.CalculateCostForUnits(
			params.Model,
			keyGroup,
			params.PromptTokens,
			params.CompletionTokens,
			params.CachedTokens,
			params.BillableUnits,
			time.Now(),
		)
	}

	totTokens := params.TotalTokens
	if totTokens == 0 {
		totTokens = params.PromptTokens + params.CompletionTokens
	}

	// Feed actual consumption back into the per-minute token budget so the next
	// requests from this key are gated by real usage rather than an estimate.
	if cfg := getAPIKeyConfig(c); cfg != nil && cfg.TPM > 0 {
		middleware.GlobalTokenQuota.Consume(cfg.Key, cfg.TPM, totTokens)
	}

	telemetry.GlobalMetrics.RecordRequestWithModel(
		params.Model,
		success,
		params.Duration,
		params.PromptTokens,
		params.CompletionTokens,
	)

	// The dashboard's live throughput meter is normally fed by the streaming
	// delta hooks. Non-streaming requests have no deltas, so their tokens are
	// added here once the request completes; streaming handlers set LiveTracked
	// so their totals are not counted twice.
	if !params.LiveTracked {
		telemetry.GlobalLiveMetrics.AddTokens(totTokens)
	}

	if storage.GlobalAsyncLogger != nil {
		tenantID := c.GetString("tenant_id")
		if tenantID == "" {
			tenantID = c.GetString(middleware.ContextKeyTenant)
		}
		storage.GlobalAsyncLogger.Record(&storage.UsageLogRecord{
			TraceID:          middleware.GetTraceID(c),
			ChatID:           params.ChatID,
			Channel:          params.Channel,
			SessionID:        params.SessionID,
			APIKey:           getRequestAPIKey(c),
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
			ErrorMessage:     params.ErrorMessage,
		})
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

	return cost, savedCost, params.CachedTokens
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
	RecordUsage(c, AuditRecordParams{
		SessionID:    sessionID,
		Model:        modelName,
		Channel:      channel,
		Duration:     dur,
		StatusCode:   statusCode,
		ErrorMessage: msg,
	})
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
	if resp != nil && resp.Usage != nil {
		promptTokens = resp.Usage.PromptTokens
		completionTokens = resp.Usage.CompletionTokens
		cachedTokens = resp.Usage.GetCachedTokens()
	}
	if promptTokens == 0 {
		if estimated := estimateCanonicalPromptTokens(req); estimated > 0 {
			promptTokens = estimated
			source = usageSourceEstimated
		}
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
func resolveEmbeddingTokens(req *model.EmbeddingRequest, usage model.EmbeddingUsage) (int, string) {
	if usage.PromptTokens > 0 {
		return usage.PromptTokens, usageSourceUpstream
	}
	if req == nil {
		return 0, usageSourceUpstream
	}
	if inputs := req.GetInputStrings(); len(inputs) > 0 {
		chars := 0
		for _, input := range inputs {
			chars += len(input)
		}
		return estimateTokens(chars), usageSourceEstimated
	}
	// Token-id form: one array element is one token.
	if ids, ok := req.Input.([]any); ok && len(ids) > 0 {
		return len(ids), usageSourceEstimated
	}
	return 0, usageSourceUpstream
}

// resolveRerankTokens approximates prompt tokens for a rerank request (query plus
// every candidate document) when the upstream returned no usage.
func resolveRerankTokens(req *model.RerankRequest, usage model.RerankUsage) (int, string) {
	if usage.TotalTokens > 0 {
		return usage.TotalTokens, usageSourceUpstream
	}
	if req == nil {
		return 0, usageSourceUpstream
	}
	chars := len(req.Query)
	for _, doc := range req.GetDocumentTexts() {
		chars += len(doc)
	}
	return estimateTokens(chars), usageSourceEstimated
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
