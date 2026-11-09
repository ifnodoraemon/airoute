package api

import (
	"encoding/json"

	"github.com/ifnodoraemon/airoute/internal/billing"
	"github.com/ifnodoraemon/airoute/internal/model"
)

// estimateTokens is the single token-estimation heuristic used by every audit
// path that has to approximate usage because the upstream did not report it.
// ~4 bytes per token is the widely used rule of thumb for mixed English/JSON
// payloads. Previously each handler divided by 3 or 4 locally, so the same
// request could be billed differently depending on which endpoint served it.
func estimateTokens(chars int) int {
	return model.EstimateTokensFromChars(chars)
}

// modelNameOf returns the requested model name of a chat request.
func modelNameOf(req *model.ChatCompletionRequest) string {
	if req == nil {
		return ""
	}
	return req.Model
}

// observePromptEstimate records an upstream-reported prompt count against our local
// estimate, calibrating the estimator for that model.
func observePromptEstimate(modelName string, localEstimate, upstreamActual int) {
	billing.GlobalCalibrator.Observe(modelName, localEstimate, upstreamActual)
}

// calibratedPromptEstimate applies the learned upstream-tokenizer ratio to a local
// estimate. It is a no-op until enough upstream reports have been observed.
func calibratedPromptEstimate(modelName string, localEstimate int) int {
	return billing.GlobalCalibrator.Adjust(modelName, localEstimate)
}

// streamFallbackPromptTokens is the prompt fallback every streaming handler uses
// when the upstream never reported usage.
//
// It goes through the canonical estimator (message text, multimodal parts and
// tool definitions) exactly like the unary path, so the same request is billed
// identically whether it streamed or not. The chat stream handler used to count
// message text only, which silently dropped the tools block from the bill.
func streamFallbackPromptTokens(req *model.ChatCompletionRequest) int {
	return calibratedPromptEstimate(modelNameOf(req), estimateCanonicalPromptTokens(req))
}

// estimatedStreamCompletionTokens converts accumulated streamed output characters
// into completion tokens.
//
// This must stay on the shared estimator: the Anthropic adapter stream used a
// local "/3" while every other endpoint used the shared "/4" heuristic, so the
// same text was billed one third higher on that endpoint alone.
func estimatedStreamCompletionTokens(chars int) int {
	if chars <= 0 {
		return 0
	}
	tokens := estimateTokens(chars)
	if tokens < 1 {
		return 1
	}
	return tokens
}

// Usage provenance markers recorded on the audit row.
const (
	usageSourceUpstream  = "upstream"
	usageSourceEstimated = "estimated"
)

// estimateCompletionFromChoices approximates completion tokens from the text an
// upstream actually returned. Covers visible content, reasoning content (billed
// as output by providers that stream it) and tool calls.
func estimateCompletionFromChoices(choices []model.ChatCompletionChoice) int {
	chars := 0
	for i := range choices {
		chars += len(choices[i].Message.GetContentString())
		chars += len(choices[i].Message.ReasoningContent)
		for j := range choices[i].Message.ToolCalls {
			if b, err := json.Marshal(choices[i].Message.ToolCalls[j]); err == nil {
				chars += len(b)
			}
		}
	}
	return estimateTokens(chars)
}

// localEmbeddingPromptEstimate counts the local estimate for an embedding input
// without applying the tokenizer calibration. exact reports that the input was
// already a token-id array, so no ratio applies.
func localEmbeddingPromptEstimate(req *model.EmbeddingRequest) (tokens int, exact bool) {
	if req == nil {
		return 0, false
	}
	if inputs := req.GetInputStrings(); len(inputs) > 0 {
		chars := 0
		for _, input := range inputs {
			chars += len(input)
		}
		return estimateTokens(chars), false
	}
	// Token-id form: one array element is one token.
	if ids, ok := req.Input.([]any); ok && len(ids) > 0 {
		return len(ids), true
	}
	return 0, false
}

// estimateCanonicalPromptTokens approximates prompt tokens from the canonical
// request when the upstream did not report usage.
//
// It delegates to the model package's estimator so the API layer and the router
// (which bills canceled requests from the same request object) cannot drift
// apart. Multimodal parts are priced by a fixed per-part budget there: an inline
// base64 image is image data billed per tile, never text (BUG-MULTIMODAL-TOKEN-
// ESTIMATE).
func estimateCanonicalPromptTokens(req *model.ChatCompletionRequest) int {
	return req.EstimatedPromptTokens()
}
