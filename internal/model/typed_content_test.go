package model

import (
	"strings"
	"testing"
)

// F-01 / LEAD-01 regression: the protocol adapters build ChatMessage.Content as a
// typed []ContentPart. ParseMessageContent must reuse those parts instead of
// marshalling the value into one opaque text part, or the token estimate prices a
// base64 image as text and the outbound converters send the picture as prose.
func TestParseMessageContentReusesTypedParts(t *testing.T) {
	want := []ContentPart{
		{Type: ContentPartText, Text: "describe this"},
		{Type: ContentPartImageURL, ImageURL: &ImageURLPart{URL: "data:image/png;base64,iVBORw0KGgo="}},
	}

	for name, raw := range map[string]any{
		"[]ContentPart":  want,
		"ContentPart":    want[0],
		"*ContentPart":   &want[0],
		"[]*ContentPart": []*ContentPart{&want[0], &want[1]},
	} {
		got := ParseMessageContent(raw)
		if name == "ContentPart" || name == "*ContentPart" {
			if len(got) != 1 || got[0].Type != ContentPartText || got[0].Text != "describe this" {
				t.Fatalf("%s: got %#v", name, got)
			}
			continue
		}
		if len(got) != len(want) {
			t.Fatalf("%s: got %d parts, want %d", name, len(got), len(want))
		}
		for i := range want {
			if got[i].Type != want[i].Type {
				t.Fatalf("%s: part %d type = %q, want %q", name, i, got[i].Type, want[i].Type)
			}
		}
		if got[1].ImageURL == nil || got[1].ImageURL.URL != want[1].ImageURL.URL {
			t.Fatalf("%s: image URL lost: %#v", name, got[1])
		}
	}

	// An implicit part type is inferred from the payload rather than dropped.
	inferred := ParseMessageContent([]ContentPart{{ImageURL: &ImageURLPart{URL: "https://example.com/a.png"}}})
	if len(inferred) != 1 || inferred[0].Type != ContentPartImageURL {
		t.Fatalf("implicit image part not inferred: %#v", inferred)
	}

	// The JSON fallback remains the boundary for representations the gateway does
	// not produce itself (e.g. a slice of strings).
	fallback := ParseMessageContent([]string{"a", "b"})
	if len(fallback) != 1 || fallback[0].Type != ContentPartText || !strings.Contains(fallback[0].Text, "a") {
		t.Fatalf("unexpected fallback: %#v", fallback)
	}
}

// F-01 regression for the billing side: the same picture must cost the same
// whether the request carries the []any wire form or the typed form the adapters
// produce. Before the fix the typed form was charged by base64 length (~100x).
func TestTypedContentEstimateMatchesMapContent(t *testing.T) {
	const imageBytes = 300 * 1024
	dataURI := "data:image/png;base64," + strings.Repeat("A", imageBytes)
	text := "what is in this picture?"

	anyContent := []any{
		map[string]any{"type": "text", "text": text},
		map[string]any{"type": "image_url", "image_url": map[string]any{"url": dataURI}},
	}
	typedContent := []ContentPart{
		{Type: ContentPartText, Text: text},
		{Type: ContentPartImageURL, ImageURL: &ImageURLPart{URL: dataURI}},
	}

	anyTokens := EstimateContentTokens(anyContent)
	typedTokens := EstimateContentTokens(typedContent)
	if typedTokens != anyTokens {
		t.Fatalf("typed content estimate = %d, []any = %d; the two representations must agree", typedTokens, anyTokens)
	}
	// The documented budget: one text part plus one high-detail image.
	if want := EstimateTokensFromChars(len(text)) + ImageTokensDefault; typedTokens > want {
		t.Fatalf("estimate %d exceeds the multimodal budget %d (base64 counted as text?)", typedTokens, want)
	}

	typedReq := &ChatCompletionRequest{Model: "claude-x", Messages: []ChatMessage{{Role: "user", Content: typedContent}}}
	anyReq := &ChatCompletionRequest{Model: "claude-x", Messages: []ChatMessage{{Role: "user", Content: anyContent}}}
	if got, want := typedReq.EstimatedPromptTokens(), anyReq.EstimatedPromptTokens(); got != want {
		t.Fatalf("request estimate: typed=%d []any=%d", got, want)
	}
}
