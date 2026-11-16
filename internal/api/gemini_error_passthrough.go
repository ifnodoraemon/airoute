package api

import (
	"encoding/json"

	"github.com/gin-gonic/gin"
	"github.com/ifnodoraemon/airoute/internal/provider"
)

// respondGeminiUpstreamError returns a native Gemini upstream error envelope to
// the client verbatim (credential-scrubbed) instead of re-wrapping it in gateway
// prose. A Gemini SDK parses error.status / error.code; the old path rendered the
// upstream body as "upstream <chan> returned status N: {...}" inside a fresh
// envelope, which destroyed that structure and nested the JSON in a string.
//
// The gateway's resolved status code is kept for the HTTP response so the
// security-relevant classification (401/403 -> 502, client abort -> 499) is
// unchanged; only the body is passed through. A non-Gemini upstream body (an
// OpenAI channel serving a converted Gemini request) has no error.status and
// falls back to the standardized envelope.
func respondGeminiUpstreamError(c *gin.Context, statusCode int, err error) {
	if ue, ok := provider.AsUpstreamError(err); ok {
		if body, ok := sanitizeGeminiErrorBody(ue.Body); ok {
			c.Data(statusCode, "application/json", body)
			return
		}
	}
	RespondGeminiError(c, statusCode, sanitizeErrorMessage(err), geminiErrorStatus(statusCode))
}

// sanitizeGeminiErrorBody scrubs credentials from an upstream body but only
// accepts a genuine Gemini error envelope (error.status present). It re-validates
// the scrubbed result is still JSON so a credential pattern that happens to span
// syntax can never emit a broken body to the client.
func sanitizeGeminiErrorBody(raw string) ([]byte, bool) {
	if raw == "" {
		return nil, false
	}
	var envelope struct {
		Error struct {
			Status string `json:"status"`
		} `json:"error"`
	}
	if json.Unmarshal([]byte(raw), &envelope) != nil || envelope.Error.Status == "" {
		return nil, false
	}
	scrubbed := auditErrorMessage(raw)
	if !json.Valid([]byte(scrubbed)) {
		return nil, false
	}
	return []byte(scrubbed), true
}