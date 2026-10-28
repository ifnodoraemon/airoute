package model

import (
	"encoding/json"
	"strings"
)

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

// Token budgets for non-text content parts.
//
// Prompt-token estimation has to approximate the counts a provider would report,
// and for non-text parts that count is NOT a function of the encoded payload
// length. An image is billed per tile (OpenAI: 85 tokens for a 1024x1024
// low-detail image, ~765 for high detail); audio is billed per second of audio.
// A `data:` URI carries the image as base64, so charging it by characters
// inflated one screenshot to hundreds of thousands of tokens (BUG-MULTIMODAL-
// TOKEN-ESTIMATE: a 683 KiB PNG estimated at 232,529 prompt tokens instead of
// under a thousand).
//
// These budgets deliberately err low: an under-estimate only loses margin on the
// rare billable-without-usage path, while an over-estimate charges the customer
// hundreds of times the real cost.
const (
	// ImageTokensLowDetail is the documented low-detail image budget.
	ImageTokensLowDetail = 85
	// ImageTokensDefault covers high/auto detail images and any image whose
	// detail is unspecified.
	ImageTokensDefault = 765
	// NonTextPartTokens is the per-part budget for other binary parts (inline
	// audio, files, video). One audio minute is roughly 600 tokens, so this is a
	// conservative ceiling for whatever a single request part carries.
	NonTextPartTokens = 765
)

// EstimateContentTokens approximates the prompt tokens of one message content
// value: text parts use the shared chars→tokens heuristic, non-text parts use the
// fixed per-part budgets above and never contribute their base64 payload length.
func EstimateContentTokens(raw any) int {
	parts := ParseMessageContent(raw)
	if len(parts) == 0 {
		return 0
	}
	tokens := 0
	for _, part := range parts {
		switch part.Type {
		case ContentPartText:
			tokens += EstimateTokensFromChars(len(part.Text))
		case ContentPartImageURL:
			if part.ImageURL != nil && strings.EqualFold(part.ImageURL.Detail, "low") {
				tokens += ImageTokensLowDetail
			} else {
				tokens += ImageTokensDefault
			}
		default:
			// input_audio and any part type without a textual representation: a
			// fixed conservative budget, never the encoded length.
			tokens += NonTextPartTokens
		}
	}
	return tokens
}

// EstimatedPromptTokens approximates the prompt tokens of a chat request
// (messages plus tool definitions) with the shared chars→tokens heuristic for
// text and fixed per-part budgets for images/audio/video.
//
// It is used when the upstream never reported usage and the request was still
// billable — most importantly a client that disconnects after the request has
// already been sent upstream, where the provider has consumed (and will bill us
// for) the prompt even though we received no response.
//
// Multimodal parts are priced, not measured: a `data:` URI's base64 payload is
// image data the provider charges by tile, so it must never be counted as text
// (see the budgets above).
func (r *ChatCompletionRequest) EstimatedPromptTokens() int {
	if r == nil {
		return 0
	}
	tokens := 0
	for i := range r.Messages {
		tokens += EstimateContentTokens(r.Messages[i].Content)
	}
	for i := range r.Tools {
		if encoded, err := json.Marshal(r.Tools[i]); err == nil {
			tokens += EstimateTokensFromChars(len(encoded))
		}
	}
	return tokens
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
