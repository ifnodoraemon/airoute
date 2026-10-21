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

var (
	tsCacheMu sync.RWMutex
	tsCache   = make(map[string]struct {
		data *TimeSeriesData
		at   time.Time
	})
)

// GetStatsTimeSeries returns aggregated time-series buckets and true percentiles across all logs in the timeframe.
func (r *Repository) GetStatsTimeSeries(timeframe string, keys []string) (*TimeSeriesData, error) {
	if timeframe != "1h" && timeframe != "7d" {
		timeframe = "24h"
	}

	cacheKey := fmt.Sprintf("%s:%s", timeframe, strings.Join(keys, ","))
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
			label = fmt.Sprintf("%d/%d", bTime.Month(), bTime.Day())
		default:
			label = fmt.Sprintf("%d:00", bTime.Hour())
		}
		buckets[i] = &TimeSeriesBucket{
			Label:     label,
			Timestamp: bTime.UnixMilli(),
		}
		bucketTTFTs[i] = make([]int64, 0)
	}

	startTime := now.Add(-time.Duration(numBuckets) * interval)
	var where string
	var args []interface{}

	if r.db.Dialect() == "postgres" {
		where = "WHERE created_at >= ?"
	} else {
		where = "WHERE datetime(created_at) >= datetime(?)"
	}
	args = append(args, startTime.UTC().Format("2006-01-02 15:04:05"))

	if len(keys) > 0 {
		placeholders := make([]string, len(keys))
		for i, k := range keys {
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
			buckets[idx].Requests++
			buckets[idx].Tokens += totalTokens
			buckets[idx].Costs += cost
			if statusCode >= 400 || statusCode < 200 || errorMsg != "" {
				buckets[idx].Errors++
			}
			if ttftMs > 0 {
				bucketTTFTs[idx] = append(bucketTTFTs[idx], ttftMs)
			}
		}

		if ttftMs > 0 {
			allTTFTs = append(allTTFTs, ttftMs)
		}

		mod := inferModalityCategory(model)
		modalityCounts[mod]++
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
		Total:   int64(len(allTTFTs)),
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
	tsCache[cacheKey] = struct {
		data *TimeSeriesData
		at   time.Time
	}{data: res, at: time.Now()}
	tsCacheMu.Unlock()

	return res, nil
}

func inferModalityCategory(modelName string) string {
	lower := strings.ToLower(modelName)
	if strings.Contains(lower, "rerank") {
		return "rerank"
	}
	if strings.Contains(lower, "embed") || strings.Contains(lower, "bge-m3") || strings.Contains(lower, "text-embedding") {
		return "embeddings"
	}
	if strings.Contains(lower, "image") || strings.Contains(lower, "dall-e") || strings.Contains(lower, "flux") || strings.Contains(lower, "stable-diffusion") {
		return "images"
	}
	if strings.Contains(lower, "tts") || strings.Contains(lower, "speech") || strings.Contains(lower, "whisper") || strings.Contains(lower, "audio") || strings.Contains(lower, "voice") || strings.Contains(lower, "cosyvoice") {
		return "audio"
	}
	if strings.Contains(lower, "video") || strings.Contains(lower, "sora") || strings.Contains(lower, "kling") || strings.Contains(lower, "wan") || strings.Contains(lower, "cogvideox") {
		return "video"
	}
	return "chat"
}
