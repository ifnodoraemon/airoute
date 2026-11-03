package storage

import (
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/ifnodoraemon/airoute/internal/telemetry"
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

	// ftsOnce/ftsAvailable remember whether migration v20 created the SQLite FTS5
	// index, so the keyword query can pick MATCH or the LIKE fallback without
	// probing sqlite_master on every request.
	ftsOnce      sync.Once
	ftsAvailable bool

	// Retention maintenance is opportunistic: the gateway has no dedicated
	// migration/retention process and these tables are written rarely, so the
	// cheapest place to prune is the write path itself. Each slot records the
	// last prune attempt (UnixNano) so a burst of writes only prunes once per
	// interval.
	verificationPruneAt atomic.Int64
	videoTaskPruneAt    atomic.Int64
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

// NewRepository creates a new Repository and registers it as the process-wide
// instance.
//
// The registration goes through SetGlobalRepository so there is a single write
// path to globalRepo. Writing the variable directly here made the setter
// unreachable from main (deadcode flagged it) and left two ways to install a
// repository — the one line nobody used and the one nobody documented.
func NewRepository(db *DB) *Repository {
	r := &Repository{db: db}
	SetGlobalRepository(r)
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

// overviewPeakWindow bounds the "peak" figures on the overview dashboard. An hour
// is long enough that the number means something and short enough that the
// GROUP BY stays cheap behind the 5-second cache.
const overviewPeakWindow = time.Hour

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
		stats.SavedCost = r.cachedTokenSavings("", nil)
	}

	_ = r.db.QueryRow(`SELECT COUNT(*) FROM channels WHERE status='active'`).Scan(&stats.ActiveChannels)
	_ = r.db.QueryRow(`SELECT COUNT(*) FROM api_keys WHERE status='active'`).Scan(&stats.ActiveKeys)

	// Real-time RPM & TPM over the trailing 60 seconds
	oneMinAgo := r.dialectTimePrecise(time.Now().Add(-60 * time.Second))
	_ = r.db.QueryRow(`SELECT COUNT(*), COALESCE(SUM(total_tokens), 0) FROM usage_logs WHERE created_at >= ?`, oneMinAgo).Scan(&stats.CurrentRPM, &stats.CurrentTPM)

	// Peak rates over the trailing hour. These two fields used to be declared,
	// serialized and read by the console's fallback path but never assigned, so a
	// dashboard without time-series data displayed a fabricated 0.
	peakSince := time.Now().Add(-overviewPeakWindow)
	if peakRPM, peakTPM, err := r.peakRates(r.dialectTimePrecise(peakSince), "", nil); err == nil {
		stats.PeakRPM, stats.PeakTPM = peakRPM, peakTPM
	}

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

// GetOrCreateSetting lets concurrent replicas agree on a single generated value.
func (r *Repository) GetOrCreateSetting(key, initial string) (string, error) {
	if _, err := r.db.Exec(`INSERT INTO system_settings (key, value, updated_at)
		VALUES (?, ?, CURRENT_TIMESTAMP) ON CONFLICT(key) DO NOTHING`, key, initial); err != nil {
		return "", err
	}
	var value string
	err := r.db.QueryRow(`SELECT value FROM system_settings WHERE key = ?`, key).Scan(&value)
	return value, err
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
		args = append(args, NormalizeStoredAPIKeyReference(k))
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
		stats.SavedCost = r.cachedTokenSavings(clause, args)
	}
	activeQuery := fmt.Sprintf(`SELECT COUNT(*) FROM api_keys WHERE status='active' AND key IN (%s)`, clause)
	_ = r.db.QueryRow(activeQuery, args...).Scan(&stats.ActiveKeys)

	// Real-time RPM & TPM over trailing 60s for scoped keys
	oneMinAgo := r.dialectTimePrecise(time.Now().Add(-60 * time.Second))
	rtQuery := fmt.Sprintf(`SELECT COUNT(*), COALESCE(SUM(total_tokens), 0) FROM usage_logs WHERE created_at >= ? AND api_key IN (%s)`, clause)
	rtArgs := append([]any{oneMinAgo}, args...)
	_ = r.db.QueryRow(rtQuery, rtArgs...).Scan(&stats.CurrentRPM, &stats.CurrentTPM)

	// Peak rates over the trailing hour for the same key scope, matching the
	// cluster-wide overview and the time-series peak.
	peakSince := r.dialectTimePrecise(time.Now().Add(-overviewPeakWindow))
	peakCond := fmt.Sprintf(` AND api_key IN (%s)`, clause)
	if peakRPM, peakTPM, err := r.peakRates(peakSince, peakCond, args); err == nil {
		stats.PeakRPM, stats.PeakTPM = peakRPM, peakTPM
	}

	return stats, nil
}

// cachedTokenSavings derives the prompt-cache saving from the configured price
// list instead of a built-in constant.
//
// The console used to report cached_tokens * 1.8 CNY/1M — a hardcoded benchmark
// rate that had no relationship to model_prices.cache_read_price, so the "saved
// cost" figure was fabricated whenever the real cache price differed (which is
// the normal case: prices are configured per model).
//
// Derivation: only the cached prompt tokens are discounted, by the difference
// between the model's prompt price and its cache-read price. Models without a
// price row (or with cache_read_price >= prompt_price, i.e. no discount) are
// skipped: the documented fallback is that such traffic contributes no saving,
// because its cache rate is not knowable from configuration. The default pricing
// group is used, matching how a request without a special group is billed.
//
// scopeClause/scopeArgs optionally restrict the calculation to a set of API keys
// (the tenant-scoped dashboard); clause "" means all traffic.
func (r *Repository) cachedTokenSavings(scopeClause string, scopeArgs []any) float64 {
	if r == nil || r.db == nil {
		return 0
	}
	query := `SELECT COALESCE(SUM(u.cached_tokens * (p.prompt_price - p.cache_read_price) / 1000000.0), 0)
		FROM usage_logs u
		JOIN model_prices p ON p.model = u.model AND COALESCE(p.group_name, 'default') = 'default'
		WHERE COALESCE(u.cached_tokens, 0) > 0
		  AND p.cache_read_price >= 0
		  AND p.prompt_price > p.cache_read_price`
	args := []any{}
	if scopeClause != "" {
		query += ` AND u.api_key IN (` + scopeClause + `)`
		args = append(args, scopeArgs...)
	}
	var savings float64
	if err := r.db.QueryRow(query, args...).Scan(&savings); err != nil {
		// A missing/unreadable price join must not fail the whole dashboard.
		telemetry.ScopedLogger("billing").Warn("failed to derive cached token savings from model_prices", "error", err.Error())
		return 0
	}
	if savings < 0 {
		return 0
	}
	return savings
}
