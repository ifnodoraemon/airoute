package storage

import (
	"strings"
	"testing"
)

// Regression for the credential redaction gap: the value pattern stopped at the
// first whitespace, so an Authorization value of the form "Bearer <token>" kept
// the token in the archived snippet.
func TestTruncateSnippetRedactsCredentialForms(t *testing.T) {
	const secret = "sk-live-should-never-be-archived-1234567890"
	const basicBlob = "dXNlcjpwYXNzd29yZA=="

	cases := []struct {
		name string
		in   string
		// substrings that must survive the redaction untouched
		keep []string
	}{
		{
			name: "json authorization bearer",
			in:   `{"headers":{"authorization":"Bearer ` + secret + `"}}`,
			keep: []string{"headers"},
		},
		{
			name: "json authorization bearer with spaces",
			in:   `{"authorization": "Bearer ` + secret + `"}`,
			keep: []string{"authorization"},
		},
		{
			name: "json api_key",
			in:   `{"model":"gpt","api_key":"` + secret + `","messages":[]}`,
			keep: []string{"model", "messages"},
		},
		{
			name: "json x-api-key",
			in:   `{"x-api-key":"` + secret + `"}`,
			keep: []string{"x-api-key"},
		},
		{
			name: "json password with escaped quote",
			in:   `{"password":"p\"` + secret + `"}`,
			keep: []string{"password"},
		},
		{
			name: "json client secret",
			in:   `{"client_secret":"` + secret + `"}`,
			keep: []string{"client_secret"},
		},
		{
			name: "header bearer",
			in:   "GET /v1/chat/completions\nAuthorization: Bearer " + secret + "\nX-Request-Id: abc",
			keep: []string{"X-Request-Id: abc"},
		},
		{
			name: "header basic",
			in:   "authorization: Basic " + basicBlob,
			keep: []string{"authorization"},
		},
		{
			name: "query string api key",
			in:   "/v1/models?api_key=" + secret + "&model=gpt-4",
			keep: []string{"model=gpt-4"},
		},
		{
			name: "query string token",
			in:   "/v1/models?access_token=" + secret + "&limit=5",
			keep: []string{"limit=5"},
		},
		{
			name: "bare scheme in error text",
			in:   "upstream returned 401 for Bearer " + secret,
			keep: []string{"upstream returned 401"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := TruncateSnippet(tc.in, 4096)
			if strings.Contains(got, secret) {
				t.Fatalf("credential survived redaction: %q", got)
			}
			if strings.Contains(got, basicBlob) {
				t.Fatalf("basic credentials survived redaction: %q", got)
			}
			if !strings.Contains(got, "[REDACTED]") {
				t.Fatalf("expected a redaction marker, got %q", got)
			}
			for _, keep := range tc.keep {
				if !strings.Contains(got, keep) {
					t.Fatalf("non-credential content %q was lost: %q", keep, got)
				}
			}
		})
	}
}

// Redaction must not rewrite ordinary payload fields that merely contain a
// credential-ish word in their value.
func TestTruncateSnippetLeavesNonCredentialPayloadsAlone(t *testing.T) {
	in := `{"model":"gpt-4","max_tokens":1024,"messages":[{"role":"user","content":"what is a token budget?"}]}`
	if got := TruncateSnippet(in, 4096); got != in {
		t.Fatalf("non-credential payload rewritten:\n in=%q\ngot=%q", in, got)
	}
}
