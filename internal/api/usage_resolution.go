package api

import (
	"github.com/ifnodoraemon/airoute/internal/model"
)

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
		estimated := estimateCompletionFromChoices(resp.Choices)
		if estimated == 0 {
			estimated = model.EstimateTokensFromChars(resp.NativeOutputChars)
		}
		if estimated > 0 {
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
	if req == nil {
		if usage.PromptTokens > 0 {
			return usage.PromptTokens, usageSourceUpstream
		}
		return 0, usageSourceUpstream
	}
	local, exact := localEmbeddingPromptEstimate(req)
	if usage.PromptTokens > 0 && !usage.Estimated {
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

// resolveRerankTokens approximates prompt tokens for a rerank request (query plus
// every candidate document) when the upstream returned no usage.
//
// C-04: same calibration contract as embedding and chat — estimate, learn from a
// real upstream count, otherwise apply the learned ratio.
func resolveRerankTokens(req *model.RerankRequest, usage model.RerankUsage) (int, string) {
	if req == nil {
		if usage.TotalTokens > 0 {
			return usage.TotalTokens, usageSourceUpstream
		}
		return 0, usageSourceUpstream
	}
	// One estimator for the API layer and the router: the model package owns it so
	// the two cannot drift apart.
	local := req.EstimatedPromptTokens()
	if usage.TotalTokens > 0 {
		observePromptEstimate(req.Model, local, usage.TotalTokens)
		return usage.TotalTokens, usageSourceUpstream
	}
	if local <= 0 {
		return 0, usageSourceUpstream
	}
	return calibratedPromptEstimate(req.Model, local), usageSourceEstimated
}
