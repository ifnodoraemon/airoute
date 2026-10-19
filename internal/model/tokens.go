package model

// CharsPerToken is the single token-estimation ratio used across the gateway
// whenever an upstream does not report usage. ~4 bytes/token is the common rule
// of thumb for mixed English/JSON payloads.
//
// This lives in the model package so the API layer and every provider share one
// number; previously handlers divided by 3 and providers by 4, so the same
// request was billed differently depending on which path served it.
const CharsPerToken = 4

// EstimateTokensFromChars converts a character count into an approximate token
// count. It returns 0 for non-positive input and never returns 0 for non-empty
// input, so callers do not need their own "at least 1" guard.
func EstimateTokensFromChars(chars int) int {
	if chars <= 0 {
		return 0
	}
	tokens := chars / CharsPerToken
	if tokens < 1 {
		return 1
	}
	return tokens
}
