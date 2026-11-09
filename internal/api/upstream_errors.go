package api

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/ifnodoraemon/airoute/internal/provider"
	"github.com/ifnodoraemon/airoute/internal/router"
)

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
	setUpstreamRetryAfter(c, err)
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
	var unsupported *provider.UnsupportedTranslationError
	if errors.As(err, &unsupported) {
		return http.StatusBadRequest, "invalid_request_error", "unsupported_translation", channelFromError(err)
	}
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

	if status, kind, code, resolvedChannel, ok := typedUpstreamFailureStatus(err, channel); ok {
		return status, kind, code, resolvedChannel
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

func typedUpstreamFailureStatus(err error, channel string) (int, string, string, string, bool) {
	// 1. Direct or wrapped UpstreamError extraction
	if ue, ok := provider.AsUpstreamError(err); ok {
		if channel == "" && ue.Channel != "" {
			channel = ue.Channel
		}
		if ue.Status > 0 {
			switch ue.Status {
			case http.StatusPaymentRequired:
				return http.StatusPaymentRequired, "billing_error", "upstream_payment_required", channel, true
			case http.StatusNotFound:
				return http.StatusNotFound, "invalid_request_error", "model_not_found", channel, true
			case http.StatusTooManyRequests:
				return http.StatusTooManyRequests, "rate_limit_error", "upstream_rate_limited", channel, true
			case http.StatusUnauthorized:
				return http.StatusBadGateway, "gateway_error", "upstream_unauthorized", channel, true
			case http.StatusForbidden:
				return http.StatusBadGateway, "gateway_error", "upstream_forbidden", channel, true
			case http.StatusBadRequest:
				return http.StatusBadRequest, "invalid_request_error", "invalid_payload", channel, true
			case http.StatusUnprocessableEntity:
				return http.StatusUnprocessableEntity, "invalid_request_error", "unprocessable_entity", channel, true
			default:
				if ue.Status >= 500 {
					return ue.Status, "gateway_error", "upstream_server_error", channel, true
				}
			}
		}
	}

	return 0, "", "", channel, false
}
