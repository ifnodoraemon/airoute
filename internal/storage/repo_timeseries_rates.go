package storage

import (
	"fmt"
	"math"
	"time"
)

// peakRates returns the busiest one-minute window's request count and token total
// inside the window that starts at windowStart.
//
// It is shared by the time series (which passes its timeframe start) and the
// overview (which passes a fixed recent window), so "peak RPM/TPM" means the same
// thing on both endpoints.
func (r *Repository) peakRates(windowStart any, keyCond string, keyArgs []any) (int64, int64, error) {
	// The epoch expression is dialect-specific: SQLite has strftime, PostgreSQL
	// has EXTRACT. Both yield whole seconds, so the grouping is identical.
	epochExpr := `CAST(strftime('%s', created_at) AS INTEGER)`
	if r.db.Dialect() == "postgres" {
		epochExpr = `CAST(EXTRACT(EPOCH FROM created_at) AS BIGINT)`
	}
	query := fmt.Sprintf(`SELECT COALESCE(MAX(reqs), 0), COALESCE(MAX(toks), 0) FROM (
		SELECT COUNT(*) AS reqs, COALESCE(SUM(total_tokens), 0) AS toks
		FROM usage_logs WHERE created_at >= ?%s
		GROUP BY %s / 60) AS minute_agg`, keyCond, epochExpr)
	args := append([]any{windowStart}, keyArgs...)
	var peakRPM, peakTPM int64
	if err := r.db.QueryRow(query, args...).Scan(&peakRPM, &peakTPM); err != nil {
		return 0, 0, fmt.Errorf("query peak rates error: %w", err)
	}
	return peakRPM, peakTPM, nil
}

func setTimeSeriesBucketRates(buckets []*TimeSeriesBucket, interval time.Duration) {
	intervalMinutes := int64(interval / time.Minute)
	if intervalMinutes <= 0 {
		intervalMinutes = 1
	}
	for i := 0; i < len(buckets); i++ {
		buckets[i].RPM = int64(math.Round(float64(buckets[i].Requests) / float64(intervalMinutes)))
		buckets[i].TPM = int64(math.Round(float64(buckets[i].Tokens) / float64(intervalMinutes)))
	}

}

func (r *Repository) readTimeSeriesRates(window timeSeriesWindow, res *TimeSeriesData) error {
	// 2. Real-time rates over the trailing 60 seconds.
	recentValue := r.dialectTimePrecise(window.now.Add(-60 * time.Second))
	recentArgs := append([]any{recentValue}, window.keyArgs...)
	var currentRPM, currentTPM int64
	if err := r.db.QueryRow(`SELECT COUNT(*), COALESCE(SUM(total_tokens), 0) FROM usage_logs WHERE created_at >= ?`+window.keyCond, recentArgs...).
		Scan(&currentRPM, &currentTPM); err != nil {
		return fmt.Errorf("query timeseries realtime rates error: %w", err)
	}

	// 3. Peak per-minute volume inside the window. The same helper backs the
	// overview's fallback figure, so the two endpoints cannot disagree.
	peakRPM, peakTPM, err := r.peakRates(window.startValue, window.keyCond, window.keyArgs)
	if err != nil {
		return err
	}

	res.CurrentRPM, res.CurrentTPM = currentRPM, currentTPM
	res.PeakRPM, res.PeakTPM = peakRPM, peakTPM
	return nil
}
