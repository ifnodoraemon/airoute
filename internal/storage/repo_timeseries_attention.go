package storage

import "fmt"

// AttentionSummary covers every log in the authorized time window, independent
// of the audit table's pagination. Status codes include 2xx rows with errors.
type AttentionSummary struct {
	TotalRequests       int64         `json:"total_requests"`
	FailedRequests      int64         `json:"failed_requests"`
	RateLimitedRequests int64         `json:"rate_limited_requests"`
	ErrorCountByCode    map[int]int64 `json:"error_count_by_code"`
}

func (r *Repository) readTimeSeriesAttention(window timeSeriesWindow) (*AttentionSummary, error) {
	rows, err := r.db.Query(`SELECT COALESCE(status_code, 200), COUNT(*),
		SUM(CASE WHEN COALESCE(status_code, 200) >= 400 OR COALESCE(status_code, 200) < 200
		OR COALESCE(error_message, '') <> '' THEN 1 ELSE 0 END)
		FROM usage_logs WHERE created_at >= ?`+window.keyCond+` GROUP BY COALESCE(status_code, 200)`, window.windowArgs...)
	if err != nil {
		return nil, fmt.Errorf("query timeseries attention: %w", err)
	}
	defer rows.Close()
	summary := &AttentionSummary{ErrorCountByCode: make(map[int]int64)}
	for rows.Next() {
		var code int
		var count, failed int64
		if err := rows.Scan(&code, &count, &failed); err != nil {
			return nil, fmt.Errorf("scan timeseries attention: %w", err)
		}
		summary.TotalRequests += count
		summary.FailedRequests += failed
		if failed > 0 {
			summary.ErrorCountByCode[code] = failed
		}
		if code == 429 {
			summary.RateLimitedRequests += count
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate timeseries attention: %w", err)
	}
	return summary, nil
}
