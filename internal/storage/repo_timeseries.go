package storage

import (
	"fmt"
	"math"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// TimeSeriesBucket represents a single aggregated time window.
type TimeSeriesBucket struct {
	Label     string  `json:"label"`
	Timestamp int64   `json:"timestamp"`
	Requests  int64   `json:"requests"`
	Tokens    int64   `json:"tokens"`
	Errors    int64   `json:"errors"`
	AvgTTFT   int64   `json:"avgTtft"`
	Costs     float64 `json:"costs"`
	RPM       int64   `json:"rpm"`
	TPM       int64   `json:"tpm"`
}

// LatencyPercentiles represents latency distribution statistics.
type LatencyPercentiles struct {
	P50     *int64 `json:"p50"`
	P90     *int64 `json:"p90"`
	P99     *int64 `json:"p99"`
	Sampled int64  `json:"sampled"`
	Total   int64  `json:"total"`
}

// TimeSeriesData contains the time-series buckets, percentiles, real-time rates, and modality distribution.
type TimeSeriesData struct {
	Timeframe     string              `json:"timeframe"`
	CurrentRPM    int64               `json:"current_rpm"`
	CurrentTPM    int64               `json:"current_tpm"`
	PeakRPM       int64               `json:"peak_rpm"`
	PeakTPM       int64               `json:"peak_tpm"`
	Buckets       []*TimeSeriesBucket `json:"buckets"`
	Percentiles   LatencyPercentiles  `json:"percentiles"`
	ModalityStats map[string]int64    `json:"modality_stats"`
}

const maxTsCacheEntries = 128

// defaultTTFTSampleLimit bounds how many TTFT observations are pulled for the
// percentile estimate. LatencyPercentiles.Sampled reports the actual sample size,
// so a bounded newest-first sample keeps the dashboard query cost constant instead
// of growing with the audit trail.
const defaultTTFTSampleLimit = 5000

// ttftSampleLimit resolves the percentile sample bound, overridable through
// GATEWAY_TIMESERIES_TTFT_SAMPLE for deployments that want a larger or smaller
// sample. Invalid values fall back to the default.
func ttftSampleLimit() int {
	raw := strings.TrimSpace(os.Getenv("GATEWAY_TIMESERIES_TTFT_SAMPLE"))
	if raw == "" {
		return defaultTTFTSampleLimit
	}
	sample, err := strconv.Atoi(raw)
	if err != nil || sample <= 0 {
		return defaultTTFTSampleLimit
	}
	return sample
}

var (
	tsCacheMu sync.RWMutex
	tsCache   = make(map[string]struct {
		data *TimeSeriesData
		at   time.Time
	})
)

// NewEmptyTimeSeriesData constructs an initialized empty TimeSeriesData structure for the given timeframe.
func NewEmptyTimeSeriesData(timeframe string) *TimeSeriesData {
	if timeframe != "1h" && timeframe != "7d" {
		timeframe = "24h"
	}
	var numBuckets int
	var interval time.Duration
	switch timeframe {
	case "1h":
		numBuckets = 12
		interval = 5 * time.Minute
	case "7d":
		numBuckets = 14
		interval = 12 * time.Hour
	default:
		timeframe = "24h"
		numBuckets = 24
		interval = 1 * time.Hour
	}

	now := time.Now()
	buckets := make([]*TimeSeriesBucket, numBuckets)
	for i := 0; i < numBuckets; i++ {
		bTime := now.Add(-time.Duration(numBuckets-1-i) * interval)
		var label string
		switch timeframe {
		case "1h":
			label = bTime.Format("15:04")
		case "7d":
			label = fmt.Sprintf("%d/%d %02d:00", bTime.Month(), bTime.Day(), bTime.Hour())
		default:
			label = fmt.Sprintf("%02d:00", bTime.Hour())
		}
		buckets[i] = &TimeSeriesBucket{
			Label:     label,
			Timestamp: bTime.UnixMilli(),
		}
	}

	return &TimeSeriesData{
		Timeframe:   timeframe,
		Buckets:     buckets,
		Percentiles: LatencyPercentiles{Sampled: 0, Total: 0},
		ModalityStats: map[string]int64{
			"chat": 0, "images": 0, "audio": 0, "video": 0, "embeddings": 0, "rerank": 0,
		},
	}
}

// GetStatsTimeSeries returns aggregated time-series buckets and true percentiles across all logs in the timeframe.
func (r *Repository) GetStatsTimeSeries(timeframe string, keys []string) (*TimeSeriesData, error) {
	if timeframe != "1h" && timeframe != "7d" {
		timeframe = "24h"
	}

	sortedKeys := make([]string, len(keys))
	copy(sortedKeys, keys)
	sort.Strings(sortedKeys)
	cacheKey := fmt.Sprintf("%s:%s", timeframe, strings.Join(sortedKeys, ","))

	tsCacheMu.RLock()
	if entry, ok := tsCache[cacheKey]; ok && time.Since(entry.at) < 3*time.Second {
		tsCacheMu.RUnlock()
		return entry.data, nil
	}
	tsCacheMu.RUnlock()

	var numBuckets int
	var interval time.Duration

	switch timeframe {
	case "1h":
		numBuckets = 12
		interval = 5 * time.Minute
	case "7d":
		numBuckets = 14
		interval = 12 * time.Hour
	default:
		timeframe = "24h"
		numBuckets = 24
		interval = 1 * time.Hour
	}

	now := time.Now()
	buckets := make([]*TimeSeriesBucket, numBuckets)
	for i := 0; i < numBuckets; i++ {
		bTime := now.Add(-time.Duration(numBuckets-1-i) * interval)
		var label string
		switch timeframe {
		case "1h":
			label = bTime.Format("15:04")
		case "7d":
			label = fmt.Sprintf("%d/%d %02d:00", bTime.Month(), bTime.Day(), bTime.Hour())
		default:
			label = fmt.Sprintf("%02d:00", bTime.Hour())
		}
		buckets[i] = &TimeSeriesBucket{
			Label:     label,
			Timestamp: bTime.UnixMilli(),
		}
	}

	startTime := now.Add(-time.Duration(numBuckets) * interval)
	startValue := startTime.UTC().Format(sqliteTimestampLayout)

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

	epochExpr := `CAST(strftime('%s', created_at) AS INTEGER)`
	if r.db.Dialect() == "postgres" {
		epochExpr = `CAST(EXTRACT(EPOCH FROM created_at) AS BIGINT)`
	}

	// 1. Per-bucket aggregates computed by the database.
	//
	// The previous implementation pulled every row in the window into Go (48k rows
	// for 7d on the live cluster, growing linearly with traffic) only to fold them
	// into fixed-width buckets. GROUP BY does the same work inside the database and
	// returns at most numBuckets rows, so the cost no longer depends on volume.
	intervalSeconds := int64(interval / time.Second)
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
		GROUP BY 1`, numBuckets, epochExpr, keyCond)

	aggArgs := append([]any{now.Unix(), intervalSeconds}, windowArgs...)
	aggRows, err := r.db.Query(aggQuery, aggArgs...)
	if err != nil {
		return nil, fmt.Errorf("query timeseries buckets error: %w", err)
	}
	var totalRequests int64
	for aggRows.Next() {
		var bucketIdx, requests, tokens, errors, ttftSamples, ttftSum int64
		var costs float64
		if err := aggRows.Scan(&bucketIdx, &requests, &tokens, &costs, &errors, &ttftSamples, &ttftSum); err != nil {
			aggRows.Close()
			return nil, fmt.Errorf("scan timeseries bucket error: %w", err)
		}
		if bucketIdx < 0 || bucketIdx >= int64(numBuckets) {
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
		return nil, fmt.Errorf("iterate timeseries buckets error: %w", err)
	}
	aggRows.Close()

	intervalMinutes := int64(interval / time.Minute)
	if intervalMinutes <= 0 {
		intervalMinutes = 1
	}
	for i := 0; i < numBuckets; i++ {
		buckets[i].RPM = int64(math.Round(float64(buckets[i].Requests) / float64(intervalMinutes)))
		buckets[i].TPM = int64(math.Round(float64(buckets[i].Tokens) / float64(intervalMinutes)))
	}

	// 2. Real-time rates over the trailing 60 seconds.
	recentValue := now.Add(-60 * time.Second).UTC().Format(sqliteTimestampLayout)
	recentArgs := append([]any{recentValue}, keyArgs...)
	var currentRPM, currentTPM int64
	if err := r.db.QueryRow(`SELECT COUNT(*), COALESCE(SUM(total_tokens), 0) FROM usage_logs WHERE created_at >= ?`+keyCond, recentArgs...).
		Scan(&currentRPM, &currentTPM); err != nil {
		return nil, fmt.Errorf("query timeseries realtime rates error: %w", err)
	}

	// 3. Peak per-minute volume inside the window.
	var peakRPM, peakTPM int64
	peakQuery := fmt.Sprintf(`SELECT COALESCE(MAX(reqs), 0), COALESCE(MAX(toks), 0) FROM (
		SELECT COUNT(*) AS reqs, COALESCE(SUM(total_tokens), 0) AS toks
		FROM usage_logs WHERE created_at >= ?%s
		GROUP BY %s / 60) AS minute_agg`, keyCond, epochExpr)
	if err := r.db.QueryRow(peakQuery, windowArgs...).Scan(&peakRPM, &peakTPM); err != nil {
		return nil, fmt.Errorf("query timeseries peak rates error: %w", err)
	}

	// 4. Latency percentiles from a bounded sample.
	//
	// The dashboard only needs the shape of the tail and LatencyPercentiles reports
	// its sample size, so the newest ttftSampleLimit() observations are enough; the
	// previous code materialised every ttft value in the window.
	sampleArgs := append(append([]any{startValue}, keyArgs...), ttftSampleLimit())
	sampleRows, err := r.db.Query(`SELECT ttft_ms FROM usage_logs WHERE created_at >= ?`+keyCond+` AND ttft_ms > 0 ORDER BY id DESC LIMIT ?`, sampleArgs...)
	if err != nil {
		return nil, fmt.Errorf("query timeseries ttft sample error: %w", err)
	}
	ttftSample := make([]int64, 0, 256)
	for sampleRows.Next() {
		var ttft int64
		if err := sampleRows.Scan(&ttft); err != nil {
			sampleRows.Close()
			return nil, fmt.Errorf("scan timeseries ttft sample error: %w", err)
		}
		ttftSample = append(ttftSample, ttft)
	}
	if err := sampleRows.Err(); err != nil {
		sampleRows.Close()
		return nil, fmt.Errorf("iterate timeseries ttft sample error: %w", err)
	}
	sampleRows.Close()

	percentiles := LatencyPercentiles{
		Sampled: int64(len(ttftSample)),
		Total:   totalRequests,
	}
	if len(ttftSample) > 0 {
		sort.Slice(ttftSample, func(i, j int) bool { return ttftSample[i] < ttftSample[j] })
		at := func(q float64) int64 {
			idx := int(math.Floor(float64(len(ttftSample)) * q))
			if idx >= len(ttftSample) {
				idx = len(ttftSample) - 1
			}
			return ttftSample[idx]
		}
		p50 := at(0.50)
		p90 := at(0.90)
		p99 := at(0.99)
		percentiles.P50 = &p50
		percentiles.P90 = &p90
		percentiles.P99 = &p99
	}

	// 5. Modality distribution. Grouping by model keeps this bounded by the number
	// of distinct models rather than the number of requests; the modality itself is
	// a name heuristic that lives in Go.
	modalityCounts := map[string]int64{
		"chat": 0, "images": 0, "audio": 0, "video": 0, "embeddings": 0, "rerank": 0,
	}
	modalityRows, err := r.db.Query(`SELECT COALESCE(model, ''), COUNT(*) FROM usage_logs WHERE created_at >= ?`+keyCond+` GROUP BY COALESCE(model, '')`, windowArgs...)
	if err != nil {
		return nil, fmt.Errorf("query timeseries modality error: %w", err)
	}
	for modalityRows.Next() {
		var modelName string
		var count int64
		if err := modalityRows.Scan(&modelName, &count); err != nil {
			modalityRows.Close()
			return nil, fmt.Errorf("scan timeseries modality error: %w", err)
		}
		modalityCounts[inferModalityCategory(modelName)] += count
	}
	if err := modalityRows.Err(); err != nil {
		modalityRows.Close()
		return nil, fmt.Errorf("iterate timeseries modality error: %w", err)
	}
	modalityRows.Close()

	res := &TimeSeriesData{
		Timeframe:     timeframe,
		CurrentRPM:    currentRPM,
		CurrentTPM:    currentTPM,
		PeakRPM:       peakRPM,
		PeakTPM:       peakTPM,
		Buckets:       buckets,
		Percentiles:   percentiles,
		ModalityStats: modalityCounts,
	}

	tsCacheMu.Lock()
	if len(tsCache) >= maxTsCacheEntries {
		// Prune expired entries
		nowTime := time.Now()
		for k, v := range tsCache {
			if nowTime.Sub(v.at) > 3*time.Second {
				delete(tsCache, k)
			}
		}
		// If still full, purge all
		if len(tsCache) >= maxTsCacheEntries {
			tsCache = make(map[string]struct {
				data *TimeSeriesData
				at   time.Time
			})
		}
	}
	tsCache[cacheKey] = struct {
		data *TimeSeriesData
		at   time.Time
	}{data: res, at: time.Now()}
	tsCacheMu.Unlock()

	return res, nil
}

func inferModalityCategory(modelName string) string {
	lower := strings.ToLower(modelName)
	if strings.Contains(lower, "rerank") || strings.Contains(lower, "bge-reranker") {
		return "rerank"
	}
	if strings.Contains(lower, "embed") || strings.Contains(lower, "bge-") || strings.Contains(lower, "text-embedding") {
		return "embeddings"
	}
	if strings.Contains(lower, "image") || strings.Contains(lower, "dall-e") || strings.Contains(lower, "flux") || strings.Contains(lower, "midjourney") || strings.Contains(lower, "stable-diffusion") || strings.Contains(lower, "seedream") || strings.Contains(lower, "sdxl") {
		return "images"
	}
	if strings.Contains(lower, "tts") || strings.Contains(lower, "speech") || strings.Contains(lower, "whisper") || strings.Contains(lower, "audio") || strings.Contains(lower, "voice") || strings.Contains(lower, "cosyvoice") || strings.Contains(lower, "sensevoice") || strings.Contains(lower, "transcription") || strings.Contains(lower, "asr") {
		return "audio"
	}
	if strings.Contains(lower, "video") || strings.Contains(lower, "sora") || strings.Contains(lower, "kling") || strings.Contains(lower, "wan") || strings.Contains(lower, "cogvideox") || strings.Contains(lower, "seedance") || strings.Contains(lower, "luma") || strings.Contains(lower, "runway") || strings.Contains(lower, "pika") || strings.Contains(lower, "vidu") {
		return "video"
	}
	return "chat"
}
