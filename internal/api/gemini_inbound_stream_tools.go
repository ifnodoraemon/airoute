package api

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/ifnodoraemon/airoute/internal/model"
)

// recordGeminiStreamEnd estimates missing usage (mutating the caller's counters)
// and enqueues the audit record, returning the deferred settlement closure.
// Extracted from handleGeminiStream to keep it under the size red line.
func recordGeminiStreamEnd(c *gin.Context, statusCode int, promptTokens, compTokens *int, usageSource *string, accumulatedChars int, canonicalReq *model.ChatCompletionRequest, params AuditRecordParams) func() {
	// A client abort (499) still sent the prompt upstream; estimate it too.
	if statusCode == http.StatusOK || statusCode == statusClientClosed {
		canonicalEstimate := estimateCanonicalPromptTokens(canonicalReq)
		if *promptTokens == 0 {
			*promptTokens = streamFallbackPromptTokens(canonicalReq)
			*usageSource = "estimated"
		} else {
			observePromptEstimate(canonicalReq.Model, canonicalEstimate, *promptTokens)
		}
		if *compTokens == 0 && accumulatedChars > 0 {
			*compTokens = estimatedStreamCompletionTokens(accumulatedChars)
			*usageSource = "estimated"
		}
	}
	params.PromptTokens = *promptTokens
	params.CompletionTokens = *compTokens
	params.UsageSource = *usageSource
	_, _, _, settle := RecordUsageDeferred(c, params)
	return settle
}