package model

import (
	"encoding/json"
	"reflect"
)

// UnmarshalJSON normalizes usage reported by OpenAI-compatible inference
// engines. Cache counts are accepted only when the upstream explicitly reports
// them; a missing cache field must never turn into an invented billing discount.
func (u *Usage) UnmarshalJSON(data []byte) error {
	type canonical Usage
	var raw struct {
		canonical
		Prompt        *int                 `json:"prompt_tokens"`
		Completion    *int                 `json:"completion_tokens"`
		Input         *int                 `json:"input_tokens"`
		Output        *int                 `json:"output_tokens"`
		InputDetails  *PromptTokensDetails `json:"input_tokens_details"`
		Cached        *int                 `json:"cached_tokens"`
		CacheHit      *int                 `json:"prompt_cache_hit_tokens"`
		CacheRead     *int                 `json:"cache_read_input_tokens"`
		CacheCreation int                  `json:"cache_creation_input_tokens"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	*u = Usage(raw.canonical)
	if raw.Completion != nil {
		u.CompletionTokens = *raw.Completion
	} else if raw.Output != nil {
		u.CompletionTokens = *raw.Output
	}
	if raw.Prompt != nil {
		u.PromptTokens = *raw.Prompt
	} else if raw.Input != nil {
		u.PromptTokens = *raw.Input
		// Anthropic's input excludes cache reads/writes; Responses API's input
		// already includes cache and has input_tokens_details instead.
		u.PromptTokens += raw.CacheCreation
		if raw.CacheRead != nil {
			u.PromptTokens += *raw.CacheRead
		}
	} else if u.TotalTokens >= u.CompletionTokens {
		u.PromptTokens = u.TotalTokens - u.CompletionTokens
	}
	if u.PromptTokensDetails == nil {
		u.PromptTokensDetails = raw.InputDetails
		for _, value := range []*int{raw.CacheHit, raw.Cached, raw.CacheRead} {
			if u.PromptTokensDetails == nil && value != nil {
				u.PromptTokensDetails = &PromptTokensDetails{CachedTokens: *value}
			}
		}
	}
	// Some OpenAI-compatible proxies report Anthropic's cache_creation_input_tokens
	// at the top level; keep the write count readable instead of losing it inside
	// the prompt total (the prompt total itself is unchanged on purpose: cache
	// writes are a subset of it, see the comment above).
	if raw.CacheCreation > 0 {
		if u.PromptTokensDetails == nil {
			u.PromptTokensDetails = &PromptTokensDetails{}
		}
		if u.PromptTokensDetails.CacheCreationTokens == 0 {
			u.PromptTokensDetails.CacheCreationTokens = raw.CacheCreation
		}
	}
	if u.TotalTokens == 0 {
		u.TotalTokens = u.PromptTokens + u.CompletionTokens
	}
	// Preserve usage members this struct does not model (for example
	// completion_tokens_details) so they reach the client unchanged.
	u.Extra = captureExtraFields(data, reflect.TypeOf(canonical{}))
	return nil
}
