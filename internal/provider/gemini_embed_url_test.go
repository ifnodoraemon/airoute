package provider

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ifnodoraemon/airoute/internal/model"
)

// F-03 regression: Gemini documents model ids as "models/<id>", and its ListModels
// endpoint returns that form. A channel mapping copied from there must not double
// the "models/" segment in the URL while the body normalises it to one.
func TestGeminiEmbedAcceptsModelsPrefixMapping(t *testing.T) {
	cases := []struct {
		name         string
		mappedModel  string
		wantBodyPath string
	}{
		{"prefixed mapping", "models/gemini-embedding-001", "models/gemini-embedding-001"},
		{"bare mapping", "gemini-embedding-001", "models/gemini-embedding-001"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var gotPath, gotBody string
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				gotPath = r.URL.Path
				body, _ := io.ReadAll(r.Body)
				gotBody = string(body)
				w.Header().Set("Content-Type", "application/json")
				_, _ = io.WriteString(w, `{"embedding":{"values":[0.1,0.2]}}`)
			}))
			defer srv.Close()

			p := NewGeminiProvider(srv.Client())
			channel := &model.ChannelConfig{
				Name:         "gemini-embed",
				Type:         model.ProviderGemini,
				BaseURL:      srv.URL,
				APIKey:       "test-key",
				ModelMapping: map[string]string{"gemini-embed": tc.mappedModel},
			}
			if _, err := p.Embed(context.Background(), &model.EmbeddingRequest{Model: "gemini-embed", Input: "hello"}, channel); err != nil {
				t.Fatalf("embed call: %v", err)
			}

			const wantPath = "/v1beta/models/gemini-embedding-001:embedContent"
			if gotPath != wantPath {
				t.Fatalf("path = %q, want %q", gotPath, wantPath)
			}
			var parsed struct {
				Model string `json:"model"`
			}
			if err := json.Unmarshal([]byte(gotBody), &parsed); err != nil {
				t.Fatalf("parse body %q: %v", gotBody, err)
			}
			if parsed.Model != tc.wantBodyPath {
				t.Fatalf("body model = %q, want %q", parsed.Model, tc.wantBodyPath)
			}
		})
	}
}

// The batch form goes through its own URL; it must follow the same rule.
func TestGeminiBatchEmbedAcceptsModelsPrefixMapping(t *testing.T) {
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"embeddings":[{"values":[0.1]},{"values":[0.2]}]}`)
	}))
	defer srv.Close()

	p := NewGeminiProvider(srv.Client())
	channel := &model.ChannelConfig{
		Name: "gemini-embed", Type: model.ProviderGemini, BaseURL: srv.URL, APIKey: "test-key",
		ModelMapping: map[string]string{"gemini-embed": "models/gemini-embedding-001"},
	}
	if _, err := p.Embed(context.Background(), &model.EmbeddingRequest{Model: "gemini-embed", Input: []any{"a", "b"}}, channel); err != nil {
		t.Fatalf("batch embed call: %v", err)
	}
	if want := "/v1beta/models/gemini-embedding-001:batchEmbedContents"; gotPath != want {
		t.Fatalf("batch path = %q, want %q", gotPath, want)
	}
}

// The chat/stream endpoints share the same convention, so the same mapping must
// produce a single "models/" segment there too.
func TestGeminiChatURLNormalisesModelsPrefix(t *testing.T) {
	const want = "https://generativelanguage.googleapis.com/v1beta/models/gemini-2.5-flash:generateContent"
	if got := buildGeminiURL("https://generativelanguage.googleapis.com", "models/gemini-2.5-flash", false); got != want {
		t.Fatalf("buildGeminiURL = %q, want %q", got, want)
	}
	if got := buildGeminiURL("https://generativelanguage.googleapis.com", "gemini-2.5-flash", false); got != want {
		t.Fatalf("buildGeminiURL(bare) = %q, want %q", got, want)
	}
	if got := buildGeminiURL("https://generativelanguage.googleapis.com", "models/gemini-2.5-flash", true); !strings.Contains(got, "/v1beta/models/gemini-2.5-flash:streamGenerateContent") {
		t.Fatalf("stream URL = %q", got)
	}
}
