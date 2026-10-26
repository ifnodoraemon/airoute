package model

import "encoding/json"

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

// EstimatedPromptTokens approximates the prompt tokens of a chat request
// (messages plus tool definitions) with the shared chars→tokens heuristic.
//
// It is used when the upstream never reported usage and the request was still
// billable — most importantly a client that disconnects after the request has
// already been sent upstream, where the provider has consumed (and will bill us
// for) the prompt even though we received no response.
func (r *ChatCompletionRequest) EstimatedPromptTokens() int {
	if r == nil {
		return 0
	}
	chars := 0
	for i := range r.Messages {
		chars += len(r.Messages[i].GetContentString())
	}
	for i := range r.Tools {
		if encoded, err := json.Marshal(r.Tools[i]); err == nil {
			chars += len(encoded)
		}
	}
	return EstimateTokensFromChars(chars)
}

// EstimatedPromptTokens approximates embedding prompt tokens from the input
// strings, or from the token-id form where each element is one token.
func (r *EmbeddingRequest) EstimatedPromptTokens() int {
	if r == nil {
		return 0
	}
	if inputs := r.GetInputStrings(); len(inputs) > 0 {
		chars := 0
		for _, input := range inputs {
			chars += len(input)
		}
		return EstimateTokensFromChars(chars)
	}
	if ids, ok := r.Input.([]any); ok {
		return len(ids)
	}
	return 0
}

// EstimatedPromptTokens approximates rerank prompt tokens from the query and
// every candidate document.
func (r *RerankRequest) EstimatedPromptTokens() int {
	if r == nil {
		return 0
	}
	chars := len(r.Query)
	for _, doc := range r.GetDocumentTexts() {
		chars += len(doc)
	}
	return EstimateTokensFromChars(chars)
}
