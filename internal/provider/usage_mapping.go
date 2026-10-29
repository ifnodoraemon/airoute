package provider

import "github.com/ifnodoraemon/airoute/internal/model"

// anthropicUsageToModel converts Anthropic usage into canonical OpenAI-style usage.
//
// Anthropic reports prompt-cache reads and writes separately from input_tokens,
// so the total prompt token count is the sum of all three; cache reads are the
// tokens that were served from the prompt cache, and cache writes are the tokens
// billed at the cache-write premium. Both stay inside promptTokens (that is how
// the provider composes its invoice) and are additionally exposed as breakdown
// fields so billing can split the prompt into its three priced buckets (C-02).
func anthropicUsageToModel(u *AnthropicUsage) *model.Usage {
	if u == nil {
		return nil
	}
	prompt := u.InputTokens + u.CacheReadInputTokens + u.CacheCreationInputTokens
	return &model.Usage{
		PromptTokens:        prompt,
		CompletionTokens:    u.OutputTokens,
		TotalTokens:         prompt + u.OutputTokens,
		PromptTokensDetails: cacheTokenDetails(u.CacheReadInputTokens, u.CacheCreationInputTokens),
	}
}

// cacheTokenDetails builds the prompt-cache breakdown shared by the unary and
// streaming Anthropic adapters. Both buckets live inside PromptTokens; billing
// prices the read bucket at the cache-read rate and the write bucket at the
// cache-write premium, so dropping either one silently misprices the request
// (F-05: the streaming path used to report only the read count).
func cacheTokenDetails(cacheReadTokens, cacheCreationTokens int) *model.PromptTokensDetails {
	if cacheReadTokens <= 0 && cacheCreationTokens <= 0 {
		return nil
	}
	return &model.PromptTokensDetails{
		CachedTokens:        cacheReadTokens,
		CacheCreationTokens: cacheCreationTokens,
	}
}

// geminiUsageToModel converts Gemini usageMetadata into canonical usage.
// Gemini's promptTokenCount already includes cached content, so it is used as-is.
// The Gemini API reports no cache-write count, so CacheCreationTokens stays 0
// rather than being invented.
//
// Completion tokens are candidatesTokenCount PLUS thoughtsTokenCount: on thinking
// models the candidates count covers only the visible answer while the upstream
// bills (and its total includes) the reasoning tokens as output. Folding them into
// CompletionTokens is what makes `total == prompt + completion` hold again and
// what the billing engine prices (it consumes PromptTokens/CompletionTokens only,
// so no double counting with the upstream total is possible).
func geminiUsageToModel(u *GeminiUsageMetadata) *model.Usage {
	if u == nil {
		return nil
	}
	usage := &model.Usage{
		PromptTokens:     u.PromptTokenCount,
		CompletionTokens: u.CandidatesTokenCount + u.ThoughtsTokenCount,
		TotalTokens:      u.TotalTokenCount,
	}
	if usage.TotalTokens < usage.PromptTokens+usage.CompletionTokens {
		// Some proxies omit or truncate totalTokenCount; never report a total that
		// contradicts its own parts.
		usage.TotalTokens = usage.PromptTokens + usage.CompletionTokens
	}
	if u.CachedContentTokenCount > 0 {
		usage.PromptTokensDetails = &model.PromptTokensDetails{CachedTokens: u.CachedContentTokenCount}
	}
	return usage
}
