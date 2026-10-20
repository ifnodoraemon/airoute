package provider

import "github.com/ifnodoraemon/airoute/internal/model"

// anthropicUsageToModel converts Anthropic usage into canonical OpenAI-style usage.
//
// Anthropic reports prompt-cache reads and writes separately from input_tokens,
// so the total prompt token count is the sum of all three; cache reads are the
// tokens that were served from the prompt cache.
func anthropicUsageToModel(u *AnthropicUsage) *model.Usage {
	if u == nil {
		return nil
	}
	prompt := u.InputTokens + u.CacheReadInputTokens + u.CacheCreationInputTokens
	usage := &model.Usage{
		PromptTokens:     prompt,
		CompletionTokens: u.OutputTokens,
		TotalTokens:      prompt + u.OutputTokens,
	}
	if u.CacheReadInputTokens > 0 {
		usage.PromptTokensDetails = &model.PromptTokensDetails{CachedTokens: u.CacheReadInputTokens}
	}
	return usage
}

// geminiUsageToModel converts Gemini usageMetadata into canonical usage.
// Gemini's promptTokenCount already includes cached content, so it is used as-is.
func geminiUsageToModel(u *GeminiUsageMetadata) *model.Usage {
	if u == nil {
		return nil
	}
	usage := &model.Usage{
		PromptTokens:     u.PromptTokenCount,
		CompletionTokens: u.CandidatesTokenCount,
		TotalTokens:      u.TotalTokenCount,
	}
	if usage.TotalTokens == 0 {
		usage.TotalTokens = usage.PromptTokens + usage.CompletionTokens
	}
	if u.CachedContentTokenCount > 0 {
		usage.PromptTokensDetails = &model.PromptTokensDetails{CachedTokens: u.CachedContentTokenCount}
	}
	return usage
}
