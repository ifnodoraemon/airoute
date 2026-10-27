package storage

import (
	"regexp"
	"strings"
	"time"
)

// TraceCaptureConfig controls how much request/response detail is archived.
type TraceCaptureConfig struct {
	Mode          string  // "off" | "errors" | "all"
	SamplePercent float64 // 0-100, applied to successful requests in "all" mode
	TTL           time.Duration
	MaxSnippet    int
}

// RequestTraceRecord is a redacted snapshot of a single gateway request, used
// for troubleshooting by trace_id / chat_id / session_id.
type RequestTraceRecord struct {
	ID              int64     `json:"id"`
	TraceID         string    `json:"trace_id"`
	ChatID          string    `json:"chat_id,omitempty"`
	SessionID       string    `json:"session_id,omitempty"`
	APIKey          string    `json:"api_key,omitempty"`
	TenantID        string    `json:"tenant_id,omitempty"`
	Model           string    `json:"model,omitempty"`
	Channel         string    `json:"channel,omitempty"`
	StatusCode      int       `json:"status_code"`
	ErrorMessage    string    `json:"error_message,omitempty"`
	RequestSnippet  string    `json:"request_snippet,omitempty"`
	ResponseSnippet string    `json:"response_snippet,omitempty"`
	DurationMs      int64     `json:"duration_ms"`
	CreatedAt       time.Time `json:"created_at"`
}

// TraceFilter narrows a trace lookup.
type TraceFilter struct {
	TraceID   string
	ChatID    string
	SessionID string
	APIKey    string
	APIKeys   []string // when set, restricts results to these keys (tenant scoping)
	Limit     int
}

// RecordRequestTrace persists one redacted trace snapshot.
func (r *Repository) RecordRequestTrace(rec *RequestTraceRecord) error {
	if rec == nil {
		return nil
	}
	if rec.Limit0() {
		return nil
	}
	_, err := r.db.Exec(`INSERT INTO request_traces
		(trace_id, chat_id, session_id, api_key, tenant_id, model, channel, status_code, error_message, request_snippet, response_snippet, duration_ms)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		rec.TraceID, rec.ChatID, rec.SessionID, rec.APIKey, rec.TenantID, rec.Model, rec.Channel,
		rec.StatusCode, rec.ErrorMessage, rec.RequestSnippet, rec.ResponseSnippet, rec.DurationMs)
	return err
}

// Limit0 reports whether the record has nothing worth writing.
func (rec *RequestTraceRecord) Limit0() bool {
	return rec == nil || (rec.TraceID == "" && rec.ChatID == "" && rec.SessionID == "")
}

// ListRequestTraces returns trace snapshots matching the filter, newest first.
func (r *Repository) ListRequestTraces(f TraceFilter) ([]*RequestTraceRecord, error) {
	if f.Limit <= 0 || f.Limit > 200 {
		f.Limit = 50
	}
	query := `SELECT id, trace_id, COALESCE(chat_id,''), COALESCE(session_id,''), COALESCE(api_key,''), COALESCE(tenant_id,''),
		COALESCE(model,''), COALESCE(channel,''), status_code, COALESCE(error_message,''),
		COALESCE(request_snippet,''), COALESCE(response_snippet,''), duration_ms, created_at
		FROM request_traces WHERE 1=1`
	args := make([]any, 0, 5)
	if f.TraceID != "" {
		query += ` AND trace_id = ?`
		args = append(args, f.TraceID)
	}
	if f.ChatID != "" {
		query += ` AND chat_id = ?`
		args = append(args, f.ChatID)
	}
	if f.SessionID != "" {
		query += ` AND session_id = ?`
		args = append(args, f.SessionID)
	}
	if f.APIKey != "" {
		query += ` AND api_key = ?`
		args = append(args, f.APIKey)
	}
	if len(f.APIKeys) > 0 {
		placeholders := make([]string, 0, len(f.APIKeys))
		for _, k := range f.APIKeys {
			k = NormalizeStoredAPIKeyReference(k)
			placeholders = append(placeholders, "?")
			args = append(args, k)
		}
		query += ` AND api_key IN (` + strings.Join(placeholders, ",") + `)`
	}
	query += ` ORDER BY id DESC LIMIT ?`
	args = append(args, f.Limit)

	rows, err := r.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []*RequestTraceRecord
	for rows.Next() {
		var t RequestTraceRecord
		if err := rows.Scan(&t.ID, &t.TraceID, &t.ChatID, &t.SessionID, &t.APIKey, &t.TenantID,
			&t.Model, &t.Channel, &t.StatusCode, &t.ErrorMessage, &t.RequestSnippet,
			&t.ResponseSnippet, &t.DurationMs, &t.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, &t)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

// PurgeRequestTraces deletes snapshots older than the retention window.
func (r *Repository) PurgeRequestTraces(cutoff time.Time) (int64, error) {
	res, err := r.db.Exec(`DELETE FROM request_traces WHERE created_at < ?`, cutoff.UTC().Format("2006-01-02 15:04:05"))
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// TruncateSnippet redacts credentials and clamps a payload to max bytes.
func TruncateSnippet(payload string, max int) string {
	if max <= 0 {
		max = 4096
	}
	cleaned := redactSensitiveValues(payload)
	if len(cleaned) <= max {
		return cleaned
	}
	// Cut on a rune boundary so the snippet stays valid UTF-8.
	cut := max
	for cut > 0 && cut < len(cleaned) && (cleaned[cut]&0xC0) == 0x80 {
		cut--
	}
	return cleaned[:cut] + "…(truncated)"
}

// redactSensitiveValues removes credential values from a request/response
// snippet. Four shapes are covered because the gateway archives all of them:
//
//	JSON      {"authorization":"Bearer sk-live-..."}
//	header    Authorization: Bearer sk-live-...
//	query     ?api_key=sk-live-...&model=...
//	bare      upstream rejected Bearer sk-live-...
//
// The previous single pattern stopped the value at the first whitespace, so
// `"authorization":"Bearer sk-..."` redacted only the word "Bearer" and stored
// the actual token in request_traces for the whole retention window.
func redactSensitiveValues(payload string) string {
	// 1. JSON / quoted values: replace the whole quoted string, escapes included.
	payload = sensitiveQuotedValuePattern.ReplaceAllString(payload, `$1"[REDACTED]"`)
	// 2. Authorization headers: the value may contain a scheme and a token, so it
	//    runs to the end of the line (or to the next JSON brace).
	payload = sensitiveAuthorizationPattern.ReplaceAllString(payload, `$1$2$3[REDACTED]`)
	// 3. Query-string and bare assignments: the value ends at a delimiter.
	payload = SensitiveKeyPattern.ReplaceAllString(payload, `$1[REDACTED]`)
	// 4. Schemes left over in free text (e.g. an upstream error message).
	payload = credentialSchemePattern.ReplaceAllString(payload, `$1 [REDACTED]`)
	return payload
}

// sensitiveFieldNames lists the credential-bearing field names used by the
// gateway's own JSON bodies, query strings and proxy headers.
const sensitiveFieldNames = `(?:proxy[_-]?authorization|authorization|x[_-]goog[_-]api[_-]key|x[_-]api[_-]key|api[_-]?key|api-key|access[_-]token|refresh[_-]token|id[_-]token|token|client[_-]secret|secret|password|passwd|pwd)`

// SensitiveKeyPattern matches a credential assignment whose value is unquoted
// (query strings, `key: value` pairs). Quoted values are handled by
// sensitiveQuotedValuePattern.
var SensitiveKeyPattern = regexp.MustCompile(`(?i)("?` + sensitiveFieldNames + `"?\s*[:=]\s*)([^\s"'&,;}]+)`)

// sensitiveQuotedValuePattern matches a credential field with a quoted value.
var sensitiveQuotedValuePattern = regexp.MustCompile(`(?i)("?` + sensitiveFieldNames + `"?\s*[:=]\s*)"(?:[^"\\]|\\.)*"`)

// sensitiveAuthorizationPattern matches an Authorization header, whose value is
// "<scheme> <credentials>" and therefore contains a space. The leading group
// keeps the regex from matching a quoted JSON field name (already handled above).
var sensitiveAuthorizationPattern = regexp.MustCompile(`(?i)(^|[^"\w])((?:proxy[_-]?)?authorization)(\s*[:=]\s*)([^\r\n"{}]+)`)

// credentialSchemePattern matches "Bearer <token>" / "Basic <blob>" wherever the
// field name is absent.
var credentialSchemePattern = regexp.MustCompile(`(?i)\b(bearer|basic)\s+([A-Za-z0-9\-._~+/=]{6,})`)
