package storage

import (
	"fmt"
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
}

// Repository manages persistence for channels, keys, logs, pricing, users, and plugins.
type Repository struct {
	db *DB
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

// GetStatsOverview queries summary metrics.
func (r *Repository) GetStatsOverview() (*StatsOverview, error) {
	stats := &StatsOverview{}

	row := r.db.QueryRow(`SELECT COUNT(*), COALESCE(SUM(total_tokens), 0), COALESCE(SUM(prompt_tokens), 0), COALESCE(SUM(completion_tokens), 0), COALESCE(SUM(cached_tokens), 0), COALESCE(SUM(cost), 0.0), COALESCE(AVG(ttft_ms), 0), COALESCE(AVG(duration_ms), 0) FROM usage_logs`)
	err := row.Scan(&stats.TotalRequests, &stats.TotalTokens, &stats.PromptTokens, &stats.CompletionTokens, &stats.TotalCachedTokens, &stats.TotalCost, &stats.AvgTTFTMs, &stats.AvgDurationMs)
	if err != nil {
		return nil, fmt.Errorf("scan usage stats error: %w", err)
	}

	if stats.TotalCachedTokens > 0 {
		// Benchmark prompt cache savings (e.g. 1.8 CNY per 1M cached tokens discount)
		stats.SavedCost = float64(stats.TotalCachedTokens) * 1.8 / 1000000.0
	}

	_ = r.db.QueryRow(`SELECT COUNT(*) FROM channels WHERE status='active'`).Scan(&stats.ActiveChannels)
	_ = r.db.QueryRow(`SELECT COUNT(*) FROM api_keys WHERE status='active'`).Scan(&stats.ActiveKeys)

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
