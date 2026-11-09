package storage

import (
	"database/sql"
	"fmt"
	"strings"
	"time"
)

type timeSeriesWindow struct {
	now        time.Time
	numBuckets int
	interval   time.Duration
	startValue any
	keyCond    string
	keyArgs    []any
	windowArgs []any
}

func (r *Repository) newTimeSeriesWindow(now time.Time, numBuckets int, interval time.Duration, sortedKeys []string) timeSeriesWindow {
	startTime := now.Add(-time.Duration(numBuckets) * interval)
	startValue := r.dialectTimePrecise(startTime)

	// Only the authorization scope filters here; all aggregation happens in SQL.
	keyCond := ""
	keyArgs := make([]any, 0, len(sortedKeys))
	if len(sortedKeys) > 0 {
		placeholders := make([]string, len(sortedKeys))
		for i, k := range sortedKeys {
			placeholders[i] = "?"
			keyArgs = append(keyArgs, k)
		}
		keyCond = " AND api_key IN (" + strings.Join(placeholders, ",") + ")"
	}
	windowArgs := append([]any{startValue}, keyArgs...)

	return timeSeriesWindow{now: now, numBuckets: numBuckets, interval: interval, startValue: startValue, keyCond: keyCond, keyArgs: keyArgs, windowArgs: windowArgs}
}

func (r *Repository) readTimeSeriesBuckets(window timeSeriesWindow, buckets []*TimeSeriesBucket) (int64, error) {
	epochExpr := `CAST(strftime('%s', created_at) AS INTEGER)`
	if r.db.Dialect() == "postgres" {
		epochExpr = `CAST(EXTRACT(EPOCH FROM created_at) AS BIGINT)`
	}

	// 1. Per-bucket aggregates computed by the database.
	//
	// The previous implementation pulled every row in the window into Go (48k rows
	// for 7d on the live cluster, growing linearly with traffic) only to fold them
	// into fixed-width buckets. GROUP BY does the same work inside the database and
	// returns at most window.numBuckets rows, so the cost no longer depends on volume.
	intervalSeconds := int64(window.interval / time.Second)
	if intervalSeconds <= 0 {
		intervalSeconds = 1
	}
	aggQuery := fmt.Sprintf(`SELECT (%d - 1) - ((? - %s) / ?) AS bucket_idx,
		COUNT(*) AS requests,
		COALESCE(SUM(total_tokens), 0) AS tokens,
		COALESCE(SUM(cost), 0.0) AS costs,
		COALESCE(SUM(CASE WHEN COALESCE(status_code, 200) >= 400 OR COALESCE(status_code, 200) < 200 OR COALESCE(error_message, '') <> '' THEN 1 ELSE 0 END), 0) AS errors,
		COALESCE(SUM(CASE WHEN ttft_ms > 0 THEN 1 ELSE 0 END), 0) AS ttft_samples,
		COALESCE(SUM(CASE WHEN ttft_ms > 0 THEN ttft_ms ELSE 0 END), 0) AS ttft_sum
		FROM usage_logs WHERE created_at >= ?%s
		GROUP BY 1`, window.numBuckets, epochExpr, window.keyCond)

	aggArgs := append([]any{window.now.Unix(), intervalSeconds}, window.windowArgs...)
	aggRows, err := r.db.Query(aggQuery, aggArgs...)
	if err != nil {
		return 0, fmt.Errorf("query timeseries buckets error: %w", err)
	}
	return scanTimeSeriesBuckets(aggRows, buckets)
}

func scanTimeSeriesBuckets(aggRows *sql.Rows, buckets []*TimeSeriesBucket) (int64, error) {
	var totalRequests int64
	for aggRows.Next() {
		var bucketIdx, requests, tokens, errors, ttftSamples, ttftSum int64
		var costs float64
		if err := aggRows.Scan(&bucketIdx, &requests, &tokens, &costs, &errors, &ttftSamples, &ttftSum); err != nil {
			aggRows.Close()
			return 0, fmt.Errorf("scan timeseries bucket error: %w", err)
		}
		if bucketIdx < 0 || bucketIdx >= int64(len(buckets)) {
			continue
		}
		bucket := buckets[bucketIdx]
		bucket.Requests = requests
		bucket.Tokens = tokens
		bucket.Costs = costs
		bucket.Errors = errors
		if ttftSamples > 0 {
			bucket.AvgTTFT = ttftSum / ttftSamples
		}
		totalRequests += requests
	}
	if err := aggRows.Err(); err != nil {
		aggRows.Close()
		return 0, fmt.Errorf("iterate timeseries buckets error: %w", err)
	}
	aggRows.Close()

	return totalRequests, nil
}
