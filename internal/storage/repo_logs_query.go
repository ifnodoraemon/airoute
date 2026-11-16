package storage

import (
	"encoding/json"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/ifnodoraemon/airoute/internal/model"
)

// escapeLikePattern makes user input match literally inside a LIKE pattern.
// Without it a keyword such as "100%" or "a_b" silently acts as a wildcard.
func escapeLikePattern(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, "%", `\%`)
	s = strings.ReplaceAll(s, "_", `\_`)
	return s
}

// sqliteTimestampLayout is how usage_logs.created_at is written (UTC, fixed
// width). Comparing against that exact format is what lets SQLite use
// idx_usage_created.
//
// Microseconds are mandatory. The previous whole-second layout collapsed every
// row of a flush onto the same instant: a burst of 502s all read 02:09:44 with
// no way to order them, measure retry spacing, or correlate them with the
// nanosecond-precision access log. It also made the audit page mix two
// timestamp shapes ("...:44" and "...:44.905597") in one page.
const sqliteTimestampLayout = "2006-01-02 15:04:05.000000"

// legacySecondTimestampLayout is how rows written before the precision fix were
// stored (SQLite only; PostgreSQL kept whatever precision it was given).
// Time filters still accept it so old rows remain queryable.
const legacySecondTimestampLayout = "2006-01-02 15:04:05"

// ftsPhrase renders user text as an FTS5 phrase, doubling embedded quotes so the
// match grammar cannot interpret it as operators.
func ftsPhrase(keyword string) string {
	return `"` + strings.ReplaceAll(keyword, `"`, `""`) + `"`
}

// sqliteUsageLogFTSAvailable reports whether migration v20 created the FTS5 index.
func (r *Repository) sqliteUsageLogFTSAvailable() bool {
	r.ftsOnce.Do(func() {
		if r.db.Dialect() != "sqlite" {
			return
		}
		var count int
		if err := r.db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = 'usage_logs_fts'`).Scan(&count); err == nil {
			r.ftsAvailable = count > 0
		}
	})
	return r.ftsAvailable
}

// normalizeSQLiteTimestamp converts an API time filter (RFC3339 from the console,
// or a plain date) into the stored representation.
func normalizeSQLiteTimestamp(raw string) (string, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", false
	}
	for _, layout := range []string{
		sqliteTimestampLayout,
		legacySecondTimestampLayout,
		"2006-01-02T15:04:05",
		time.RFC3339Nano,
		"2006-01-02",
	} {
		if parsed, err := time.Parse(layout, raw); err == nil {
			return parsed.UTC().Format(sqliteTimestampLayout), true
		}
	}
	return "", false
}

// timeFilterClause builds one side of the created_at range. SQLite stores the
// column as a UTC string, so normalizing the parameter lets the bare column be
// compared; wrapping it in datetime() forced a full scan on every page load.
// Values that cannot be parsed keep the legacy wrapped form rather than silently
// matching nothing.
func (r *Repository) timeFilterClause(op, raw string) (string, interface{}) {
	if raw == "" {
		return "", nil
	}
	if r.db.Dialect() == "postgres" {
		return ` AND created_at ` + op + ` ?`, raw
	}
	if normalized, ok := normalizeSQLiteTimestamp(raw); ok {
		return ` AND created_at ` + op + ` ?`, normalized
	}
	return ` AND datetime(created_at) ` + op + ` datetime(?)`, raw
}

// UsagePricingSnapshot is the rate actually applied when a request was billed.
// Historical charges must never be explained using today's editable prices.
type UsagePricingSnapshot struct {
	Model           string  `json:"model"`
	GroupName       string  `json:"group_name"`
	PriceGroup      string  `json:"price_group"`
	Currency        string  `json:"currency"`
	PromptPrice     float64 `json:"prompt_price"`
	CompletionPrice float64 `json:"completion_price"`
	CacheReadPrice  float64 `json:"cache_read_price"`
	// CacheWritePrice is the rate applied to prompt-cache *writes*; providers
	// charge a premium over the plain input rate for them.
	CacheWritePrice float64 `json:"cache_write_price,omitempty"`
	// CacheCreationTokens is how many of PromptTokens were written to the cache.
	CacheCreationTokens int     `json:"cache_creation_tokens,omitempty"`
	FixedPrice          float64 `json:"fixed_price"`
	GroupMultiplier     float64 `json:"group_multiplier"`
	OffPeakDiscount     float64 `json:"off_peak_discount"`
	BillableUnits       int     `json:"billable_units"`
}

func encodePricingSnapshot(snapshot *UsagePricingSnapshot) string {
	if snapshot == nil {
		return ""
	}
	encoded, _ := json.Marshal(snapshot)
	return string(encoded)
}

// UsageLogRecord represents an audit log entry.
type UsageLogRecord struct {
	Thinking         *model.ThinkingObservation `json:"thinking,omitempty"`
	ID               int64                      `json:"id"`
	TraceID          string                     `json:"trace_id"`
	ChatID           string                     `json:"chat_id,omitempty"`
	SessionID        string                     `json:"session_id,omitempty"`
	APIKey           string                     `json:"api_key"`
	TenantID         string                     `json:"tenant_id"`
	Model            string                     `json:"model"`
	Channel          string                     `json:"channel"`
	ChannelTrail     string                     `json:"channel_trail,omitempty"`
	PromptTokens     int                        `json:"prompt_tokens"`
	CompletionTokens int                        `json:"completion_tokens"`
	CachedTokens     int                        `json:"cached_tokens"`
	TotalTokens      int                        `json:"total_tokens"`
	Cost             float64                    `json:"cost"`
	PricingSnapshot  *UsagePricingSnapshot      `json:"pricing_snapshot,omitempty"`
	IsOffPeak        bool                       `json:"is_off_peak"`
	OffPeakDiscount  float64                    `json:"off_peak_discount"`
	DurationMs       int64                      `json:"duration_ms"`
	TTFTMs           int64                      `json:"ttft_ms"`
	StatusCode       int                        `json:"status_code"`
	UsageSource      string                     `json:"usage_source"` // "upstream" (reported) or "estimated" (local fallback)
	ErrorMessage     string                     `json:"error_message,omitempty"`
	CreatedAt        time.Time                  `json:"created_at"`
	// BillingID is the server-generated idempotency key for this billable row.
	// It is never taken from client input: trace_id is client-controllable
	// (X-Request-ID / traceparent) and one reused value already spans dozens of
	// genuine requests, so a unique index on it would break billing. A retried
	// Redis stream delivery carries the same BillingID and is applied once.
	BillingID     string `json:"billing_id,omitempty"`
	ReservationID string `json:"reservation_id,omitempty"`
	// Settled marks a record whose wallet movement has already been applied
	// synchronously. The async writer skips the settlement for those rows; the
	// settlement ledger's unique key is the actual guarantee, this is only an
	// optimisation that avoids a redundant conflict probe.
	Settled bool `json:"settled,omitempty"`
}

// LogFilter defines search/filtering criteria for usage audit logs.
type LogFilter struct {
	Limit          int
	Offset         int
	StartTime      string // e.g. "2026-09-26 00:00:00" or ISO8601
	EndTime        string
	TraceID        string
	ChatID         string
	SessionID      string
	Model          string
	TenantID       string
	APIKeys        []string
	ScopeByAPIKeys bool
	APIKeyID       int64
	// Channel narrows to one upstream channel name.
	Channel string
	// Status is "success" | "error" | "" (all).
	Status string
	// Cache is "cached" | "uncached" | "" (all).
	Cache string
	// Latency is "fast" (<=1000ms) | "slow" (>1000ms) | "" (all), measured on duration_ms.
	Latency string
	// Keyword is a free-text match across trace/chat/session/model/channel/error.
	Keyword string
	// SkipCount omits the total-row COUNT. Dashboard polls and CSV export only
	// need the rows, and COUNT over the filtered set is the expensive part of a
	// page request.
	SkipCount bool
}

// ListUsageLogsWithFilter queries logs with flexible filters (time range, trace ID, session ID, model, tenant).
// buildUsageLogWhere assembles the WHERE clause and args shared by the page
// query and the total-count query, so pagination always reflects the complete
// filtered result set.
func (r *Repository) buildUsageLogWhere(f LogFilter) (string, []interface{}) {
	where := ""
	var args []interface{}

	if clause, value := r.timeFilterClause(">=", f.StartTime); clause != "" {
		where += clause
		args = append(args, value)
	}
	if clause, value := r.timeFilterClause("<=", f.EndTime); clause != "" {
		where += clause
		args = append(args, value)
	}
	// These three filters are deliberately left unindexed.
	//
	// Each one is an equality OR a leading-wildcard LIKE on the same column. A
	// B-tree cannot serve `LIKE '%q%'`, and the OR means both sides must be
	// evaluated, so an index on the column is never chosen — it only costs a write
	// on every audit insert (the hottest write path). Migration 42 dropped the
	// single-column indexes v1/v7/v10 created for them. The tenant/key/model
	// filters, which are equality plus a created_at range, keep their composites.
	if f.TraceID != "" {
		where += ` AND (trace_id = ? OR trace_id LIKE ? ESCAPE '\')`
		args = append(args, f.TraceID, "%"+escapeLikePattern(f.TraceID)+"%")
	}
	if f.ChatID != "" {
		where += ` AND (chat_id = ? OR chat_id LIKE ? ESCAPE '\')`
		args = append(args, f.ChatID, "%"+escapeLikePattern(f.ChatID)+"%")
	}
	if f.SessionID != "" {
		where += ` AND (session_id = ? OR session_id LIKE ? ESCAPE '\')`
		args = append(args, f.SessionID, "%"+escapeLikePattern(f.SessionID)+"%")
	}
	if f.Model != "" {
		where += ` AND model = ?`
		args = append(args, f.Model)
	}
	if f.TenantID != "" {
		where += ` AND tenant_id = ?`
		args = append(args, f.TenantID)
	}
	if len(f.APIKeys) > 0 {
		placeholders := make([]string, 0, len(f.APIKeys))
		for _, k := range f.APIKeys {
			// Callers may pass the stored digest (the console resolves key ids to
			// api_keys.key) or a bare identifier; both normalize to the digest form
			// that the api_key column holds.
			k = NormalizeStoredAPIKeyReference(k)
			placeholders = append(placeholders, "?")
			args = append(args, k)
		}
		where += ` AND api_key IN (` + strings.Join(placeholders, ",") + `)`
	}
	if f.APIKeyID > 0 {
		where += ` AND api_key IN (SELECT key FROM api_keys WHERE id = ?)`
		args = append(args, f.APIKeyID)
	}
	if f.Channel != "" {
		where += ` AND channel = ?`
		args = append(args, f.Channel)
	}
	if f.Status == "success" {
		where += ` AND ` + usageLogSuccessSQL
	} else if f.Status == "error" {
		where += ` AND ` + usageLogErrorSQL
	}
	if f.Cache == "cached" {
		where += ` AND COALESCE(cached_tokens, 0) > 0`
	} else if f.Cache == "uncached" {
		where += ` AND COALESCE(cached_tokens, 0) = 0`
	}
	if f.Latency == "fast" {
		where += ` AND COALESCE(duration_ms, 0) <= 1000`
	} else if f.Latency == "slow" {
		where += ` AND COALESCE(duration_ms, 0) > 1000`
	}
	if f.Keyword != "" {
		// Free-text search must run in SQL: filtering only the loaded page
		// silently misses every match living on another page.
		like := "%" + escapeLikePattern(f.Keyword) + "%"
		likeClause := ` AND (trace_id LIKE ? ESCAPE '\' OR COALESCE(chat_id,'') LIKE ? ESCAPE '\' OR COALESCE(session_id,'') LIKE ? ESCAPE '\' OR COALESCE(model,'') LIKE ? ESCAPE '\' OR COALESCE(channel,'') LIKE ? ESCAPE '\' OR COALESCE(error_message,'') LIKE ? ESCAPE '\' OR channel_trail LIKE ? ESCAPE '\')`
		// The trigram tokenizer cannot answer queries shorter than three
		// characters, so those keep the LIKE scan; everything else goes through
		// the FTS5 index instead of scanning every searchable field.
		if r.db.Dialect() == "sqlite" && utf8.RuneCountInString(f.Keyword) >= 3 && r.sqliteUsageLogFTSAvailable() {
			where += ` AND id IN (SELECT rowid FROM usage_logs_fts WHERE usage_logs_fts MATCH ?)`
			args = append(args, ftsPhrase(f.Keyword))
		} else {
			where += likeClause
			args = append(args, like, like, like, like, like, like, like)
		}
	}
	return where, args
}
