package provider

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/ifnodoraemon/airoute/internal/model"
)

// R-12: every builder must interpret one channel configuration the same way.
// Before this unification the same base URL meant /chat/completions for chat,
// /embeddings for embeddings, /v1/rerank for rerank and /v1beta/... for Gemini.
func TestP2URLBuildersShareAPIRootSemantics(t *testing.T) {
	chatCases := []struct{ base, want string }{
		{"https://h.invalid", "https://h.invalid/v1/chat/completions"},
		{"https://h.invalid/", "https://h.invalid/v1/chat/completions"},
		{"https://h.invalid/v1", "https://h.invalid/v1/chat/completions"},
		{"https://h.invalid/v1/", "https://h.invalid/v1/chat/completions"},
		{"https://h.invalid/v1-openai", "https://h.invalid/v1-openai/chat/completions"},
		{"https://h.invalid/api/paas/v4", "https://h.invalid/api/paas/v4/chat/completions"},
		{"https://h.invalid/custom/api?version=1", "https://h.invalid/custom/api/chat/completions?version=1"},
		{"https://h.invalid/v1/chat/completions", "https://h.invalid/v1/chat/completions"},
	}
	for _, tc := range chatCases {
		if got := buildURL(tc.base); got != tc.want {
			t.Errorf("buildURL(%q) = %q, want %q", tc.base, got, tc.want)
		}
	}

	completionCases := []struct{ base, want string }{
		{"https://h.invalid", "https://h.invalid/v1/completions"},
		{"https://h.invalid/v1", "https://h.invalid/v1/completions"},
		{"https://h.invalid/custom/api", "https://h.invalid/custom/api/completions"},
		{"https://h.invalid/v1/chat/completions", "https://h.invalid/v1/completions"},
	}
	for _, tc := range completionCases {
		if got := buildCompletionURL(tc.base); got != tc.want {
			t.Errorf("buildCompletionURL(%q) = %q, want %q", tc.base, got, tc.want)
		}
	}

	anthropicCases := []struct{ base, want string }{
		{"https://api.anthropic.com", "https://api.anthropic.com/v1/messages"},
		{"https://api.anthropic.com/", "https://api.anthropic.com/v1/messages"},
		{"https://api.anthropic.com/v1", "https://api.anthropic.com/v1/messages"},
		{"https://api.anthropic.com/v1/messages", "https://api.anthropic.com/v1/messages"},
		{"https://proxy.invalid/anthropic", "https://proxy.invalid/anthropic/messages"},
	}
	for _, tc := range anthropicCases {
		if got := BuildAnthropicURL(tc.base, ""); got != tc.want {
			t.Errorf("BuildAnthropicURL(%q) = %q, want %q", tc.base, got, tc.want)
		}
	}
	if got := BuildAnthropicURL("https://api.anthropic.com", "count_tokens"); got != "https://api.anthropic.com/v1/messages/count_tokens" {
		t.Errorf("count_tokens URL = %q", got)
	}

	geminiCases := []struct{ base, want string }{
		{"", "https://generativelanguage.googleapis.com/v1beta/models/gemini-x:generateContent"},
		{"https://generativelanguage.googleapis.com", "https://generativelanguage.googleapis.com/v1beta/models/gemini-x:generateContent"},
		{"https://generativelanguage.googleapis.com/v1beta", "https://generativelanguage.googleapis.com/v1beta/models/gemini-x:generateContent"},
		{"https://generativelanguage.googleapis.com/v1beta/", "https://generativelanguage.googleapis.com/v1beta/models/gemini-x:generateContent"},
		{"https://proxy.invalid/gemini", "https://proxy.invalid/gemini/v1beta/models/gemini-x:generateContent"},
	}
	for _, tc := range geminiCases {
		if got := buildGeminiURL(tc.base, "gemini-x", false); got != tc.want {
			t.Errorf("buildGeminiURL(%q) = %q, want %q", tc.base, got, tc.want)
		}
	}
	if got := buildGeminiURL("https://generativelanguage.googleapis.com", "gemini-x", true); got != "https://generativelanguage.googleapis.com/v1beta/models/gemini-x:streamGenerateContent?alt=sse" {
		t.Errorf("stream Gemini URL = %q", got)
	}
}

// captureP2Transport records the requested URL path and query.
type captureP2Transport struct{ seen []string }

func (c *captureP2Transport) RoundTrip(r *http.Request) (*http.Response, error) {
	c.seen = append(c.seen, r.URL.RequestURI())
	return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": []string{"application/json"}},
		Body: io.NopCloser(strings.NewReader(`{"data":[],"results":[]}`))}, nil
}

// R-12: embeddings and rerank used to disagree about whether the base URL
// includes /v1 (same channel: /embeddings vs /v1/rerank).
func TestP2EmbeddingAndRerankURLsAgree(t *testing.T) {
	for _, tc := range []struct {
		base       string
		wantEmbed  string
		wantRerank string
	}{
		{"https://h.invalid", "/v1/embeddings", "/v1/rerank"},
		{"https://h.invalid/v1", "/v1/embeddings", "/v1/rerank"},
		{"https://h.invalid/api/paas/v4", "/api/paas/v4/embeddings", "/api/paas/v4/rerank"},
	} {
		transport := &captureP2Transport{}
		p := NewOpenAIProvider(&http.Client{Transport: transport})
		ch := &model.ChannelConfig{Name: "c", BaseURL: tc.base, Models: []string{"m"}, APIKey: "k"}

		if _, err := p.Embed(context.Background(), &model.EmbeddingRequest{Model: "m", Input: "hi"}, ch); err != nil {
			t.Fatalf("embed %q: %v", tc.base, err)
		}
		if _, err := p.Rerank(context.Background(), &model.RerankRequest{Model: "m", Query: "q"}, ch); err != nil {
			t.Fatalf("rerank %q: %v", tc.base, err)
		}
		if len(transport.seen) != 2 {
			t.Fatalf("expected 2 upstream calls, got %v", transport.seen)
		}
		if transport.seen[0] != tc.wantEmbed || transport.seen[1] != tc.wantRerank {
			t.Errorf("base %q: embed=%q rerank=%q, want %q and %q",
				tc.base, transport.seen[0], transport.seen[1], tc.wantEmbed, tc.wantRerank)
		}
	}
}
