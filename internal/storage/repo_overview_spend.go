package storage

import "time"

// loadOverviewSpend uses UTC calendar windows on every replica. Integer sums are
// converted once for API display; key scopes are the same as the main overview.
func (r *Repository) loadOverviewSpend(stats *StatsOverview, keys []string, now time.Time) error {
	now = now.UTC()
	day := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	month := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
	trailing := now.Add(-24 * time.Hour)
	previous := now.Add(-48 * time.Hour)
	since := month
	if previous.Before(since) {
		since = previous
	}
	query := `SELECT
		COALESCE(SUM(CASE WHEN created_at >= ? THEN cost_units ELSE 0 END), 0),
		COALESCE(SUM(CASE WHEN created_at >= ? THEN cost_units ELSE 0 END), 0),
		COALESCE(SUM(CASE WHEN created_at >= ? THEN cost_units ELSE 0 END), 0),
		COALESCE(SUM(CASE WHEN created_at >= ? AND created_at < ? THEN cost_units ELSE 0 END), 0)
		FROM usage_logs WHERE created_at >= ? AND created_at <= ?`
	args := []any{r.dialectTimePrecise(day), r.dialectTimePrecise(month),
		r.dialectTimePrecise(trailing), r.dialectTimePrecise(previous), r.dialectTimePrecise(trailing),
		r.dialectTimePrecise(since), r.dialectTimePrecise(now)}
	if len(keys) > 0 {
		normalized := make([]string, len(keys))
		for i, key := range keys {
			normalized[i] = NormalizeStoredAPIKeyReference(key)
		}
		placeholders, keyArgs := inClause[string](nil, normalized)
		query += " AND api_key IN (" + placeholders + ")"
		args = append(args, keyArgs...)
	}
	var today, monthly, last24h, previous24h int64
	if err := r.db.QueryRow(query, args...).Scan(&today, &monthly, &last24h, &previous24h); err != nil {
		return err
	}
	stats.TodayCost, stats.MonthCost = FromUnits(today), FromUnits(monthly)
	stats.Cost24H, stats.PreviousCost24H = FromUnits(last24h), FromUnits(previous24h)
	return nil
}
