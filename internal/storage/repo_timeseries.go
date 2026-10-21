package storage

import (
	"fmt"
	"math"
	"sort"
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
}

// LatencyPercentiles represents latency distribution statistics.
type LatencyPercentiles struct {
	P50     *int64 `json:"p50"`
	P90     *int64 `json:"p90"`
	P99     *int64 `json:"p99"`
	Sampled int64  `json:"sampled"`
	Total   int64  `json:"total"`
}

// TimeSeriesData contains the time-series buckets, percentiles, and modality distribution.
type TimeSeriesData struct {
	Timeframe     string              `json:"timeframe"`
	Buckets       []*TimeSeriesBucket `json:"buckets"`
	Percentiles   LatencyPercentiles  `json:"percentiles"`
	ModalityStats map[string]int64    `json:"modality_stats"`
}

const maxTsCacheEntries = 128

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
	bucketTTFTs := make([][]int64, numBuckets)

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
		bucketTTFTs[i] = make([]int64, 0)
	}

	startTime := now.Add(-time.Duration(numBuckets) * interval)
	var args []interface{}
	where := "WHERE created_at >= ?"
	args = append(args, startTime.UTC().Format("2006-01-02 15:04:05"))

	if len(sortedKeys) > 0 {
		placeholders := make([]string, len(sortedKeys))
		for i, k := range sortedKeys {
			placeholders[i] = "?"
			args = append(args, k)
		}
		where += " AND api_key IN (" + strings.Join(placeholders, ",") + ")"
	}

	query := `SELECT created_at, COALESCE(model, ''), COALESCE(total_tokens, 0), COALESCE(status_code, 200), COALESCE(error_message, ''), COALESCE(ttft_ms, 0), COALESCE(cost, 0.0) FROM usage_logs ` + where

	rows, err := r.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("query timeseries logs error: %w", err)
	}
	defer rows.Close()

	var allTTFTs []int64
	var totalRequests int64
	modalityCounts := map[string]int64{
		"chat": 0, "images": 0, "audio": 0, "video": 0, "embeddings": 0, "rerank": 0,
	}

	for rows.Next() {
		var rawTime interface{}
		var model string
		var totalTokens int64
		var statusCode int
		var errorMsg string
		var ttftMs int64
		var cost float64

		if err := rows.Scan(&rawTime, &model, &totalTokens, &statusCode, &errorMsg, &ttftMs, &cost); err != nil {
			return nil, fmt.Errorf("scan timeseries row error: %w", err)
		}

		t := parseSQLTime(rawTime)
		if t == nil {
			continue
		}

		diff := now.Sub(*t)
		idx := numBuckets - 1 - int(diff/interval)
		if idx >= 0 && idx < numBuckets {
			totalRequests++
			buckets[idx].Requests++
			buckets[idx].Tokens += totalTokens
			buckets[idx].Costs += cost
			if statusCode >= 400 || statusCode < 200 || errorMsg != "" {
				buckets[idx].Errors++
			}
			if ttftMs > 0 {
				bucketTTFTs[idx] = append(bucketTTFTs[idx], ttftMs)
				allTTFTs = append(allTTFTs, ttftMs)
			}
			mod := inferModalityCategory(model)
			modalityCounts[mod]++
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate timeseries rows error: %w", err)
	}

	for i := 0; i < numBuckets; i++ {
		if len(bucketTTFTs[i]) > 0 {
			var sum int64
			for _, v := range bucketTTFTs[i] {
				sum += v
			}
			buckets[i].AvgTTFT = sum / int64(len(bucketTTFTs[i]))
		}
	}

	percentiles := LatencyPercentiles{
		Sampled: int64(len(allTTFTs)),
		Total:   totalRequests,
	}
	if len(allTTFTs) > 0 {
		sort.Slice(allTTFTs, func(i, j int) bool { return allTTFTs[i] < allTTFTs[j] })
		at := func(q float64) int64 {
			idx := int(math.Floor(float64(len(allTTFTs)) * q))
			if idx >= len(allTTFTs) {
				idx = len(allTTFTs) - 1
			}
			return allTTFTs[idx]
		}
		p50 := at(0.50)
		p90 := at(0.90)
		p99 := at(0.99)
		percentiles.P50 = &p50
		percentiles.P90 = &p90
		percentiles.P99 = &p99
	}

	res := &TimeSeriesData{
		Timeframe:     timeframe,
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
