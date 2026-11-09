package api

import (
	"fmt"
	"strings"
)

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
