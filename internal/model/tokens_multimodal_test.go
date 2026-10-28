package model

import (
	"strings"
	"testing"
)

// Regression tests for BUG-MULTIMODAL-TOKEN-ESTIMATE (fixed).
//
// Prompt-token estimation is used for real money whenever the upstream did not
// report usage (and for a canceled request, whose dispatcher error carries
// EstimatedPromptTokens). Counting a message's content by marshalling it meant an
// inline base64 image was charged as text: a 683 KiB PNG became 232,529 prompt
// tokens instead of under a thousand.
//
// Non-text parts are now priced by a fixed budget (images per tile, other binary
// parts per part) and never by their encoded length.

// inlineImageRequest builds a chat request whose content carries a text part plus
// one image part with the given URL.
func inlineImageRequest(imageURL string, extraImages int) *ChatCompletionRequest {
	content := []any{
		map[string]any{"type": "text", "text": "what is in this picture?"},
		map[string]any{"type": "image_url", "image_url": map[string]any{"url": imageURL}},
	}
	for i := 0; i < extraImages; i++ {
		content = append(content, map[string]any{
			"type":      "image_url",
			"image_url": map[string]any{"url": "https://example.com/extra.png"},
		})
	}
	return &ChatCompletionRequest{
		Model:    "vision-model",
		Messages: []ChatMessage{{Role: "user", Content: content}},
	}
}

// TestInlineImageIsNotChargedByBase64Length is the original reproduction: a
// 930k-character inline image must estimate at the fixed image budget, not at
// base64Length/4.
func TestInlineImageIsNotChargedByBase64Length(t *testing.T) {
	const b64Len = 930_000 // ≈683 KiB decoded: an ordinary 1024x1024 screenshot
	imageURL := "data:image/png;base64," + strings.Repeat("A", b64Len)

	got := inlineImageRequest(imageURL, 0).EstimatedPromptTokens()

	// 930k/4 would be 232,500; the fixed budget keeps it under a thousand.
	if got > 1000 {
		t.Fatalf("inline-image estimate = %d prompt tokens, want <= 1000 (base64 payload must not be counted as text)", got)
	}
	if got < ImageTokensDefault {
		t.Fatalf("inline-image estimate = %d, want at least one image budget (%d)", got, ImageTokensDefault)
	}
}

// TestImageBudgetIsPerPartAndLinear checks that extra images add a fixed budget
// each instead of scaling with any payload length.
func TestImageBudgetIsPerPartAndLinear(t *testing.T) {
	one := inlineImageRequest("https://example.com/a.png", 0).EstimatedPromptTokens()
	two := inlineImageRequest("https://example.com/a.png", 1).EstimatedPromptTokens()
	three := inlineImageRequest("https://example.com/a.png", 2).EstimatedPromptTokens()

	if delta := two - one; delta != ImageTokensDefault {
		t.Fatalf("second image added %d tokens, want the fixed budget %d", delta, ImageTokensDefault)
	}
	if delta := three - two; delta != ImageTokensDefault {
		t.Fatalf("third image added %d tokens, want the fixed budget %d", delta, ImageTokensDefault)
	}
}

// TestRemoteImageUsesTheSameBudget pins the deliberate change: a remote-URL image
// costs the provider the same as an inline one, so it is priced identically. It
// used to contribute only the URL's characters (~30 tokens for the whole
// request), which under-estimated by an order of magnitude and made the same
// image cost differently depending on how it was attached.
func TestRemoteImageUsesTheSameBudget(t *testing.T) {
	remote := inlineImageRequest("https://example.com/a.png", 0).EstimatedPromptTokens()
	inline := inlineImageRequest("data:image/png;base64,"+strings.Repeat("A", 930_000), 0).EstimatedPromptTokens()

	// Same request, same cost basis: only the text differs by a few tokens.
	if diff := inline - remote; diff < -50 || diff > 50 {
		t.Fatalf("inline (%d) and remote (%d) image estimates differ by %d tokens; the transport must not change the cost basis",
			inline, remote, diff)
	}
	if remote < ImageTokensDefault {
		t.Fatalf("remote-image estimate = %d, want at least the image budget %d", remote, ImageTokensDefault)
	}
}

// TestLowDetailImageUsesTheReducedBudget covers detail:"low" (85 tokens).
func TestLowDetailImageUsesTheReducedBudget(t *testing.T) {
	content := []any{
		map[string]any{"type": "text", "text": "hi"},
		map[string]any{"type": "image_url", "image_url": map[string]any{
			"url": "https://example.com/a.png", "detail": "low",
		}},
	}
	req := &ChatCompletionRequest{Messages: []ChatMessage{{Role: "user", Content: content}}}
	got := req.EstimatedPromptTokens()
	if got != EstimateTokensFromChars(2)+ImageTokensLowDetail {
		t.Fatalf("low-detail estimate = %d, want text(%d) + %d", got, EstimateTokensFromChars(2), ImageTokensLowDetail)
	}
}

// TestInlineAudioIsNotChargedByBase64Length covers the other binary part type.
func TestInlineAudioIsNotChargedByBase64Length(t *testing.T) {
	content := []any{
		map[string]any{"type": "text", "text": "transcribe"},
		map[string]any{"type": "input_audio", "input_audio": map[string]any{
			"data": strings.Repeat("A", 500_000), "format": "wav",
		}},
	}
	req := &ChatCompletionRequest{Messages: []ChatMessage{{Role: "user", Content: content}}}
	if got := req.EstimatedPromptTokens(); got > 2000 {
		t.Fatalf("inline-audio estimate = %d prompt tokens, want the fixed per-part budget (500k/4 would be 125000)", got)
	}
}

// TestPlainTextEstimateUnchanged pins the text-only behaviour: 4 characters per
// token, exactly as before the fix.
func TestPlainTextEstimateUnchanged(t *testing.T) {
	req := &ChatCompletionRequest{
		Messages: []ChatMessage{{Role: "user", Content: strings.Repeat("a", 400)}},
	}
	if got, want := req.EstimatedPromptTokens(), 100; got != want {
		t.Fatalf("plain-text estimate = %d, want %d", got, want)
	}

	// Tools keep being counted on the text basis.
	withTools := &ChatCompletionRequest{
		Messages: []ChatMessage{{Role: "user", Content: strings.Repeat("a", 400)}},
		Tools: []Tool{{
			Type:     "function",
			Function: map[string]any{"name": "lookup", "description": strings.Repeat("t", 400)},
		}},
	}
	if got := withTools.EstimatedPromptTokens(); got <= 100 {
		t.Fatalf("tool definitions must still add prompt tokens, got %d", got)
	}
}

// TestNilAndEmptyRequests guards the boundary cases the dispatcher relies on.
func TestNilAndEmptyRequests(t *testing.T) {
	var nilReq *ChatCompletionRequest
	if got := nilReq.EstimatedPromptTokens(); got != 0 {
		t.Fatalf("nil request estimate = %d, want 0", got)
	}
	if got := (&ChatCompletionRequest{}).EstimatedPromptTokens(); got != 0 {
		t.Fatalf("empty request estimate = %d, want 0", got)
	}
}
