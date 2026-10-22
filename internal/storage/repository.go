package storage

import (
	"fmt"
	"strings"
	"sync"
	"time"
)

// StatsOverview aggregates system stats for the dashboard.
type StatsOverview struct {
	TotalRequests     int64   `json:"total_requests"`
	TotalTokens       int64   `json:"total_tokens"`
	PromptTokens      int64   `json:"prompt_tokens"`
	CompletionTokens  int64   `json:"completion_tokens"`
	TotalCachedTokens int64   `json:"total_cached_tokens"`
	TotalCost         float64 `json:"total_cost"`
	SavedCost         float64 `json:"saved_cost"`
	ActiveChannels    int     `json:"active_channels"`
	ActiveKeys        int     `json:"active_keys"`
	AvgTTFTMs         float64 `json:"avg_ttft_ms"`
	AvgDurationMs     float64 `json:"avg_duration_ms"`
	CurrentRPM        int64   `json:"current_rpm"` // Real-time requests in the last 60 seconds
	CurrentTPM        int64   `json:"current_tpm"` // Real-time tokens in the last 60 seconds
	PeakRPM           int64   `json:"peak_rpm"`    // Peak RPM in recent timeframe
	PeakTPM           int64   `json:"peak_tpm"`    // Peak TPM in recent timeframe
	// Status breakdown across ALL usage logs (not just the page the console
	// happens to have loaded). The dashboard used to compute its SLA and error
	// figures from the 50-row log page, which made them meaningless.
	ClientErrorRequests int64 `json:"client_error_requests"` // 4xx
	ServerErrorRequests int64 `json:"server_error_requests"` // 5xx + other failures
	SuccessRequests     int64 `json:"success_requests"`
	// TTFT sample count, so the console can tell "no samples" apart from "0 ms".
	TTFTSamples int64 `json:"ttft_samples"`
}

// Repository manages persistence for channels, keys, logs, pricing, users, and plugins.
type Repository struct {
	db           *DB
	statsCache   *StatsOverview
	statsCacheAt time.Time
	statsMu      sync.RWMutex

	// facetMu guards facetCache, a short-lived per-scope cache of the audit
	// filter dropdowns. They used to cost two full-history DISTINCT scans on
	// every console request, including the 10-second polls.
	facetMu    sync.Mutex
	facetCache map[string]usageLogFacetCacheEntry
}

var globalRepo *Repository

// SetGlobalRepository registers the global storage repository instance.
func SetGlobalRepository(repo *Repository) {
	globalRepo = repo
}

// GetGlobalRepository returns the active global repository instance.
func GetGlobalRepository() *Repository {
	return globalRepo
}

// NewRepository creates a new Repository.
func NewRepository(db *DB) *Repository {
	r := &Repository{db: db}
	globalRepo = r
	return r
}

// GetDB returns the database instance.
func (r *Repository) GetDB() *DB {
	if r == nil {
		return nil
	}
	return r.db
}

// Dialect returns the database engine dialect ("sqlite" or "postgres").
func (r *Repository) Dialect() string {
	if r == nil || r.db == nil {
		return "sqlite"
	}
	return r.db.Dialect()
}

// GetStatsOverview queries summary metrics with a 5-second in-memory cache to prevent full-table scan bottlenecks.
func (r *Repository) GetStatsOverview() (*StatsOverview, error) {
	r.statsMu.RLock()
	if r.statsCache != nil && time.Since(r.statsCacheAt) < 5*time.Second {
		cached := *r.statsCache
		r.statsMu.RUnlock()
		return &cached, nil
	}
	r.statsMu.RUnlock()

	stats := &StatsOverview{}

	row := r.db.QueryRow(`SELECT COUNT(*), COALESCE(SUM(total_tokens), 0), COALESCE(SUM(prompt_tokens), 0), COALESCE(SUM(completion_tokens), 0), COALESCE(SUM(cached_tokens), 0), COALESCE(SUM(cost), 0.0), COALESCE(AVG(ttft_ms), 0), COALESCE(AVG(duration_ms), 0), ` +
		`COALESCE(SUM(CASE WHEN status_code >= 400 AND status_code < 500 THEN 1 ELSE 0 END), 0), ` +
		`COALESCE(SUM(CASE WHEN ` + usageLogErrorSQL + ` AND (COALESCE(status_code, 0) < 400 OR status_code >= 500) THEN 1 ELSE 0 END), 0), ` +
		`COALESCE(SUM(CASE WHEN ` + usageLogSuccessSQL + ` THEN 1 ELSE 0 END), 0), ` +
		`COALESCE(SUM(CASE WHEN ttft_ms > 0 THEN 1 ELSE 0 END), 0) ` +
		`FROM usage_logs`)
	err := row.Scan(&stats.TotalRequests, &stats.TotalTokens, &stats.PromptTokens, &stats.CompletionTokens, &stats.TotalCachedTokens, &stats.TotalCost, &stats.AvgTTFTMs, &stats.AvgDurationMs,
		&stats.ClientErrorRequests, &stats.ServerErrorRequests, &stats.SuccessRequests, &stats.TTFTSamples)
	if err != nil {
		return nil, fmt.Errorf("scan usage stats error: %w", err)
	}

	if stats.TotalCachedTokens > 0 {
		// Benchmark prompt cache savings (e.g. 1.8 CNY per 1M cached tokens discount)
		stats.SavedCost = float64(stats.TotalCachedTokens) * 1.8 / 1000000.0
	}

	_ = r.db.QueryRow(`SELECT COUNT(*) FROM channels WHERE status='active'`).Scan(&stats.ActiveChannels)
	_ = r.db.QueryRow(`SELECT COUNT(*) FROM api_keys WHERE status='active'`).Scan(&stats.ActiveKeys)

	// Real-time RPM & TPM over the trailing 60 seconds
	oneMinAgo := time.Now().Add(-60 * time.Second).UTC().Format("2006-01-02 15:04:05")
	_ = r.db.QueryRow(`SELECT COUNT(*), COALESCE(SUM(total_tokens), 0) FROM usage_logs WHERE created_at >= ?`, oneMinAgo).Scan(&stats.CurrentRPM, &stats.CurrentTPM)

	r.statsMu.Lock()
	r.statsCache = stats
	r.statsCacheAt = time.Now()
	r.statsMu.Unlock()

	return stats, nil
}

// GetSetting retrieves a system configuration value.
func (r *Repository) GetSetting(key, defaultVal string) string {
	var val string
	err := r.db.QueryRow(`SELECT value FROM system_settings WHERE key = ?`, key).Scan(&val)
	if err != nil {
		return defaultVal
	}
	return val
}

// SetSetting saves a system configuration value.
func (r *Repository) SetSetting(key, val string) error {
	_, err := r.db.Exec(`
		INSERT INTO system_settings (key, value, updated_at) VALUES (?, ?, CURRENT_TIMESTAMP)
		ON CONFLICT(key) DO UPDATE SET value = excluded.value, updated_at = CURRENT_TIMESTAMP
	`, key, val)
	return err
}

// GetStatsOverviewForKeys returns the same aggregate metrics as GetStatsOverview
// but scoped to the supplied API keys. It is used to keep tenant dashboards from
// exposing cluster-wide business volume to regular users.
func (r *Repository) GetStatsOverviewForKeys(keys []string) (*StatsOverview, error) {
	if len(keys) == 0 {
		return &StatsOverview{}, nil
	}
	stats := &StatsOverview{}

	placeholders := make([]string, 0, len(keys))
	args := make([]any, 0, len(keys))
	for _, k := range keys {
		placeholders = append(placeholders, "?")
		args = append(args, k)
	}
	clause := strings.Join(placeholders, ",")

	query := fmt.Sprintf(`SELECT COUNT(*), COALESCE(SUM(total_tokens), 0), COALESCE(SUM(prompt_tokens), 0), COALESCE(SUM(completion_tokens), 0), COALESCE(SUM(cached_tokens), 0), COALESCE(SUM(cost), 0.0), COALESCE(AVG(ttft_ms), 0), COALESCE(AVG(duration_ms), 0), `+
		`COALESCE(SUM(CASE WHEN status_code >= 400 AND status_code < 500 THEN 1 ELSE 0 END), 0), `+
		`COALESCE(SUM(CASE WHEN `+usageLogErrorSQL+` AND (COALESCE(status_code, 0) < 400 OR status_code >= 500) THEN 1 ELSE 0 END), 0), `+
		`COALESCE(SUM(CASE WHEN `+usageLogSuccessSQL+` THEN 1 ELSE 0 END), 0), `+
		`COALESCE(SUM(CASE WHEN ttft_ms > 0 THEN 1 ELSE 0 END), 0) `+
		`FROM usage_logs WHERE api_key IN (%s)`, clause)
	if err := r.db.QueryRow(query, args...).Scan(&stats.TotalRequests, &stats.TotalTokens, &stats.PromptTokens, &stats.CompletionTokens, &stats.TotalCachedTokens, &stats.TotalCost, &stats.AvgTTFTMs, &stats.AvgDurationMs,
		&stats.ClientErrorRequests, &stats.ServerErrorRequests, &stats.SuccessRequests, &stats.TTFTSamples); err != nil {
		return nil, err
	}
	if stats.TotalCachedTokens > 0 {
		stats.SavedCost = float64(stats.TotalCachedTokens) * 1.8 / 1000000.0
	}
	activeQuery := fmt.Sprintf(`SELECT COUNT(*) FROM api_keys WHERE status='active' AND key IN (%s)`, clause)
	_ = r.db.QueryRow(activeQuery, args...).Scan(&stats.ActiveKeys)

	// Real-time RPM & TPM over trailing 60s for scoped keys
	oneMinAgo := time.Now().Add(-60 * time.Second).UTC().Format("2006-01-02 15:04:05")
	rtQuery := fmt.Sprintf(`SELECT COUNT(*), COALESCE(SUM(total_tokens), 0) FROM usage_logs WHERE created_at >= ? AND api_key IN (%s)`, clause)
	rtArgs := append([]any{oneMinAgo}, args...)
	_ = r.db.QueryRow(rtQuery, rtArgs...).Scan(&stats.CurrentRPM, &stats.CurrentTPM)

	return stats, nil
}
