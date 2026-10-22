package storage

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

const usageLogErrorSQL = `(COALESCE(status_code, 0) < 200 OR status_code >= 400 OR COALESCE(error_message, '') <> '')`
const usageLogSuccessSQL = `(status_code >= 200 AND status_code < 400 AND COALESCE(error_message, '') = '')`

// MaxUsageLogPageSize is the largest page the API will serve. The admin handler
// clamps to the same value so the page size it echoes back is the one the
// repository actually applied; letting the two drift made clients page by a
// size they never received.
const MaxUsageLogPageSize = 200

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
const sqliteTimestampLayout = "2006-01-02 15:04:05"

// normalizeSQLiteTimestamp converts an API time filter (RFC3339 from the console,
// or a plain date) into the stored representation.
func normalizeSQLiteTimestamp(raw string) (string, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", false
	}
	for _, layout := range []string{
		sqliteTimestampLayout,
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
	FixedPrice      float64 `json:"fixed_price"`
	GroupMultiplier float64 `json:"group_multiplier"`
	OffPeakDiscount float64 `json:"off_peak_discount"`
	BillableUnits   int     `json:"billable_units"`
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
	ID               int64                 `json:"id"`
	TraceID          string                `json:"trace_id"`
	ChatID           string                `json:"chat_id,omitempty"`
	SessionID        string                `json:"session_id,omitempty"`
	APIKey           string                `json:"api_key"`
	TenantID         string                `json:"tenant_id"`
	Model            string                `json:"model"`
	Channel          string                `json:"channel"`
	PromptTokens     int                   `json:"prompt_tokens"`
	CompletionTokens int                   `json:"completion_tokens"`
	CachedTokens     int                   `json:"cached_tokens"`
	TotalTokens      int                   `json:"total_tokens"`
	Cost             float64               `json:"cost"`
	PricingSnapshot  *UsagePricingSnapshot `json:"pricing_snapshot,omitempty"`
	IsOffPeak        bool                  `json:"is_off_peak"`
	OffPeakDiscount  float64               `json:"off_peak_discount"`
	DurationMs       int64                 `json:"duration_ms"`
	TTFTMs           int64                 `json:"ttft_ms"`
	StatusCode       int                   `json:"status_code"`
	UsageSource      string                `json:"usage_source"` // "upstream" (reported) or "estimated" (local fallback)
	ErrorMessage     string                `json:"error_message,omitempty"`
	CreatedAt        time.Time             `json:"created_at"`
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
}

// ListUsageLogs returns recent usage logs for audit and monitoring.
func (r *Repository) ListUsageLogs(limit int, offset int) ([]*UsageLogRecord, error) {
	return r.ListUsageLogsWithFilter(LogFilter{Limit: limit, Offset: offset})
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
		where += ` AND (trace_id LIKE ? ESCAPE '\' OR COALESCE(chat_id,'') LIKE ? ESCAPE '\' OR COALESCE(session_id,'') LIKE ? ESCAPE '\' OR COALESCE(model,'') LIKE ? ESCAPE '\' OR COALESCE(channel,'') LIKE ? ESCAPE '\' OR COALESCE(error_message,'') LIKE ? ESCAPE '\')`
		args = append(args, like, like, like, like, like, like)
	}
	return where, args
}

// ListUsageLogsWithFilter queries logs with flexible filters (time range, trace ID, session ID, model, tenant).
func (r *Repository) ListUsageLogsWithFilter(f LogFilter) ([]*UsageLogRecord, error) {
	logs, _, err := r.ListUsageLogsWithFilterAndCount(f)
	return logs, err
}

// ListUsageLogsWithFilterAndCount also returns the total number of rows matching
// the filter (ignoring LIMIT/OFFSET), so the UI can paginate the real result set.
func (r *Repository) ListUsageLogsWithFilterAndCount(f LogFilter) ([]*UsageLogRecord, int64, error) {
	if f.Limit <= 0 {
		f.Limit = 50
	}
	if f.Limit > MaxUsageLogPageSize {
		f.Limit = MaxUsageLogPageSize
	}
	if f.Offset < 0 {
		f.Offset = 0
	}
	if f.ScopeByAPIKeys && len(f.APIKeys) == 0 {
		return make([]*UsageLogRecord, 0), 0, nil
	}

	where, whereArgs := r.buildUsageLogWhere(f)

	var total int64
	if err := r.db.QueryRow(`SELECT COUNT(*) FROM usage_logs WHERE 1=1`+where, whereArgs...).Scan(&total); err != nil {
		return nil, 0, err
	}

	query := `SELECT id, trace_id, COALESCE(chat_id, ''), COALESCE(session_id, ''), COALESCE(api_key, ''), COALESCE(tenant_id, ''), COALESCE(model, ''), COALESCE(channel, ''), prompt_tokens, completion_tokens, COALESCE(cached_tokens, 0), total_tokens, COALESCE(cost, 0.0), COALESCE(is_off_peak, 0), COALESCE(off_peak_discount, 1.0), duration_ms, ttft_ms, status_code, COALESCE(usage_source, 'upstream'), COALESCE(error_message, ''), COALESCE(pricing_snapshot, ''), created_at FROM usage_logs WHERE 1=1` + where + ` ORDER BY id DESC LIMIT ? OFFSET ?`
	args := append(append([]interface{}{}, whereArgs...), f.Limit, f.Offset)

	rows, err := r.db.Query(query, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var logs []*UsageLogRecord
	for rows.Next() {
		var l UsageLogRecord
		var isOff int
		var createdAt time.Time
		var pricingJSON string
		if err := rows.Scan(&l.ID, &l.TraceID, &l.ChatID, &l.SessionID, &l.APIKey, &l.TenantID, &l.Model, &l.Channel, &l.PromptTokens, &l.CompletionTokens, &l.CachedTokens, &l.TotalTokens, &l.Cost, &isOff, &l.OffPeakDiscount, &l.DurationMs, &l.TTFTMs, &l.StatusCode, &l.UsageSource, &l.ErrorMessage, &pricingJSON, &createdAt); err != nil {
			return nil, 0, err
		}
		if pricingJSON != "" {
			_ = json.Unmarshal([]byte(pricingJSON), &l.PricingSnapshot)
		}
		l.IsOffPeak = isOff == 1
		l.CreatedAt = createdAt
		logs = append(logs, &l)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}
	return logs, total, nil
}

// DeleteUsageLog deletes a single audit log by ID.
func (r *Repository) DeleteUsageLog(id int64) error {
	_, err := r.db.Exec(`DELETE FROM usage_logs WHERE id = ?`, id)
	return err
}

// BatchDeleteUsageLogs deletes multiple usage logs by their IDs.
func (r *Repository) BatchDeleteUsageLogs(ids []int64) (int64, error) {
	if len(ids) == 0 {
		return 0, nil
	}
	placeholders := make([]string, len(ids))
	args := make([]interface{}, len(ids))
	for i, id := range ids {
		placeholders[i] = "?"
		args[i] = id
	}
	q := fmt.Sprintf(`DELETE FROM usage_logs WHERE id IN (%s)`, strings.Join(placeholders, ","))
	res, err := r.db.Exec(q, args...)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// ClearAllUsageLogs wipes all audit logs.
func (r *Repository) ClearAllUsageLogs() error {
	if _, err := r.db.Exec(`DELETE FROM usage_logs`); err != nil {
		return err
	}
	// The dropdowns are cached; an operator who just cleared the trail must not
	// keep seeing model/channel options for another TTL window.
	r.invalidateUsageLogFacetCache()
	return nil
}

// PurgeUsageLogsBefore deletes audit rows created before cutoff in bounded
// batches, so a large backlog cannot hold a long write lock on the data plane's
// database. Returns the number of deleted rows.
func (r *Repository) PurgeUsageLogsBefore(cutoff time.Time, batchSize int) (int64, error) {
	if batchSize <= 0 {
		batchSize = 5000
	}
	cutoffValue := cutoff.UTC().Format("2006-01-02 15:04:05")
	var total int64
	for {
		res, err := r.db.Exec(
			`DELETE FROM usage_logs WHERE id IN (SELECT id FROM usage_logs WHERE created_at < ? ORDER BY id LIMIT ?)`,
			cutoffValue, batchSize,
		)
		if err != nil {
			return total, err
		}
		deleted, err := res.RowsAffected()
		if err != nil {
			return total, err
		}
		total += deleted
		if deleted < int64(batchSize) {
			break
		}
	}
	if total > 0 {
		// Pruned rows may have been the last use of a model or channel; the filter
		// dropdowns must not keep advertising them for a full cache TTL.
		r.invalidateUsageLogFacetCache()
	}
	return total, nil
}
