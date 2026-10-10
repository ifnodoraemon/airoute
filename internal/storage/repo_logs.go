package storage

import (
	"fmt"
	"strings"
	"time"
)

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
}

// ListUsageLogs returns recent usage logs for audit and monitoring.
func (r *Repository) ListUsageLogs(limit int, offset int) ([]*UsageLogRecord, error) {
	return r.ListUsageLogsWithFilter(LogFilter{Limit: limit, Offset: offset})
}

// ListUsageLogsWithFilter queries logs with flexible filters (time range, trace ID, session ID, model, tenant).
func (r *Repository) ListUsageLogsWithFilter(f LogFilter) ([]*UsageLogRecord, error) {
	if f.Limit <= 0 || f.Limit > 200 {
		f.Limit = 50
	}
	if f.Offset < 0 {
		f.Offset = 0
	}
	if f.ScopeByAPIKeys && len(f.APIKeys) == 0 {
		return make([]*UsageLogRecord, 0), nil
	}

	query := `SELECT id, trace_id, COALESCE(chat_id, ''), COALESCE(session_id, ''), COALESCE(api_key, ''), COALESCE(tenant_id, ''), COALESCE(model, ''), COALESCE(channel, ''), prompt_tokens, completion_tokens, COALESCE(cached_tokens, 0), total_tokens, COALESCE(cost, 0.0), COALESCE(is_off_peak, 0), COALESCE(off_peak_discount, 1.0), duration_ms, ttft_ms, status_code, created_at FROM usage_logs WHERE 1=1`
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
		query += startClause
		args = append(args, f.StartTime)
	}
	if f.EndTime != "" {
		query += endClause
		args = append(args, f.EndTime)
	}
	if f.TraceID != "" {
		query += ` AND (trace_id = ? OR trace_id LIKE ?)`
		args = append(args, f.TraceID, "%"+f.TraceID+"%")
	}
	if f.ChatID != "" {
		query += ` AND (chat_id = ? OR chat_id LIKE ?)`
		args = append(args, f.ChatID, "%"+f.ChatID+"%")
	}
	if f.SessionID != "" {
		query += ` AND (session_id = ? OR session_id LIKE ?)`
		args = append(args, f.SessionID, "%"+f.SessionID+"%")
	}
	if f.Model != "" {
		query += ` AND model = ?`
		args = append(args, f.Model)
	}
	if f.TenantID != "" {
		query += ` AND tenant_id = ?`
		args = append(args, f.TenantID)
	}
	if len(f.APIKeys) > 0 {
		placeholders := make([]string, 0, len(f.APIKeys))
		for _, k := range f.APIKeys {
			placeholders = append(placeholders, "?")
			args = append(args, k)
		}
		query += ` AND api_key IN (` + strings.Join(placeholders, ",") + `)`
	}

	query += ` ORDER BY id DESC LIMIT ? OFFSET ?`
	args = append(args, f.Limit, f.Offset)

	rows, err := r.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var logs []*UsageLogRecord
	for rows.Next() {
		var l UsageLogRecord
		var isOff int
		var createdAt time.Time
		if err := rows.Scan(&l.ID, &l.TraceID, &l.ChatID, &l.SessionID, &l.APIKey, &l.TenantID, &l.Model, &l.Channel, &l.PromptTokens, &l.CompletionTokens, &l.CachedTokens, &l.TotalTokens, &l.Cost, &isOff, &l.OffPeakDiscount, &l.DurationMs, &l.TTFTMs, &l.StatusCode, &createdAt); err != nil {
			return nil, err
		}
		l.IsOffPeak = isOff == 1
		l.CreatedAt = createdAt
		logs = append(logs, &l)
	}
	return logs, nil
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
