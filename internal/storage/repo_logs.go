package storage

import (
	"fmt"
	"strings"
	"time"
)

const usageLogErrorSQL = `(COALESCE(status_code, 0) < 200 OR status_code >= 400 OR COALESCE(error_message, '') <> '')`
const usageLogSuccessSQL = `(status_code >= 200 AND status_code < 400 AND COALESCE(error_message, '') = '')`

// UsageLogRecord represents an audit log entry.
type UsageLogRecord struct {
	ID               int64     `json:"id"`
	TraceID          string    `json:"trace_id"`
	ChatID           string    `json:"chat_id,omitempty"`
	SessionID        string    `json:"session_id,omitempty"`
	APIKey           string    `json:"api_key"`
	TenantID         string    `json:"tenant_id"`
	Model            string    `json:"model"`
	Channel          string    `json:"channel"`
	PromptTokens     int       `json:"prompt_tokens"`
	CompletionTokens int       `json:"completion_tokens"`
	CachedTokens     int       `json:"cached_tokens"`
	TotalTokens      int       `json:"total_tokens"`
	Cost             float64   `json:"cost"`
	IsOffPeak        bool      `json:"is_off_peak"`
	OffPeakDiscount  float64   `json:"off_peak_discount"`
	DurationMs       int64     `json:"duration_ms"`
	TTFTMs           int64     `json:"ttft_ms"`
	StatusCode       int       `json:"status_code"`
	UsageSource      string    `json:"usage_source"` // "upstream" (reported) or "estimated" (local fallback)
	ErrorMessage     string    `json:"error_message,omitempty"`
	CreatedAt        time.Time `json:"created_at"`
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

	var startClause, endClause string
	if r.db.Dialect() == "postgres" {
		startClause = ` AND created_at >= ?`
		endClause = ` AND created_at <= ?`
	} else {
		startClause = ` AND datetime(created_at) >= datetime(?)`
		endClause = ` AND datetime(created_at) <= datetime(?)`
	}

	if f.StartTime != "" {
		where += startClause
		args = append(args, f.StartTime)
	}
	if f.EndTime != "" {
		where += endClause
		args = append(args, f.EndTime)
	}
	if f.TraceID != "" {
		where += ` AND (trace_id = ? OR trace_id LIKE ?)`
		args = append(args, f.TraceID, "%"+f.TraceID+"%")
	}
	if f.ChatID != "" {
		where += ` AND (chat_id = ? OR chat_id LIKE ?)`
		args = append(args, f.ChatID, "%"+f.ChatID+"%")
	}
	if f.SessionID != "" {
		where += ` AND (session_id = ? OR session_id LIKE ?)`
		args = append(args, f.SessionID, "%"+f.SessionID+"%")
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
		like := "%" + f.Keyword + "%"
		where += ` AND (trace_id LIKE ? OR COALESCE(chat_id,'') LIKE ? OR COALESCE(session_id,'') LIKE ? OR COALESCE(model,'') LIKE ? OR COALESCE(channel,'') LIKE ? OR COALESCE(error_message,'') LIKE ?)`
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
	if f.Limit <= 0 || f.Limit > 200 {
		f.Limit = 50
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

	query := `SELECT id, trace_id, COALESCE(chat_id, ''), COALESCE(session_id, ''), COALESCE(api_key, ''), COALESCE(tenant_id, ''), COALESCE(model, ''), COALESCE(channel, ''), prompt_tokens, completion_tokens, COALESCE(cached_tokens, 0), total_tokens, COALESCE(cost, 0.0), COALESCE(is_off_peak, 0), COALESCE(off_peak_discount, 1.0), duration_ms, ttft_ms, status_code, COALESCE(usage_source, 'upstream'), COALESCE(error_message, ''), created_at FROM usage_logs WHERE 1=1` + where + ` ORDER BY id DESC LIMIT ? OFFSET ?`
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
		if err := rows.Scan(&l.ID, &l.TraceID, &l.ChatID, &l.SessionID, &l.APIKey, &l.TenantID, &l.Model, &l.Channel, &l.PromptTokens, &l.CompletionTokens, &l.CachedTokens, &l.TotalTokens, &l.Cost, &isOff, &l.OffPeakDiscount, &l.DurationMs, &l.TTFTMs, &l.StatusCode, &l.UsageSource, &l.ErrorMessage, &createdAt); err != nil {
			return nil, 0, err
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
	_, err := r.db.Exec(`DELETE FROM usage_logs`)
	return err
}
