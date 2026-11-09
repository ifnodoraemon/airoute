package storage

import (
	"fmt"
	"sort"
	"strings"
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

// NewEmptyTimeSeriesData constructs an initialized empty TimeSeriesData structure for the given timeframe.
func NewEmptyTimeSeriesData(timeframe string) *TimeSeriesData {
	timeframe = normalizeTimeframe(timeframe)
	buckets, _ := timeSeriesBuckets(timeframe, time.Now())
	return &TimeSeriesData{
		Timeframe: timeframe, Buckets: buckets,
		Percentiles: LatencyPercentiles{Sampled: 0, Total: 0},
		ModalityStats: map[string]int64{
			"chat": 0, "images": 0, "audio": 0, "video": 0, "embeddings": 0, "rerank": 0,
		},
	}
}

// GetStatsTimeSeries returns aggregated time-series buckets and true percentiles across all logs in the timeframe.
func (r *Repository) GetStatsTimeSeries(timeframe string, keys []string) (*TimeSeriesData, error) {
	timeframe = normalizeTimeframe(timeframe)
	sortedKeys := make([]string, len(keys))
	for i, key := range keys {
		sortedKeys[i] = NormalizeStoredAPIKeyReference(key)
	}
	sort.Strings(sortedKeys)
	cacheKey := fmt.Sprintf("%s:%s", timeframe, strings.Join(sortedKeys, ","))
	if cached := cachedTimeSeries(cacheKey); cached != nil {
		return cached, nil
	}
	now := time.Now()
	buckets, interval := timeSeriesBuckets(timeframe, now)
	window := r.newTimeSeriesWindow(now, len(buckets), interval, sortedKeys)
	res := &TimeSeriesData{Timeframe: timeframe, Buckets: buckets}
	totalRequests, err := r.readTimeSeriesBuckets(window, buckets)
	if err != nil {
		return nil, err
	}
	setTimeSeriesBucketRates(buckets, interval)
	if err := r.readTimeSeriesRates(window, res); err != nil {
		return nil, err
	}
	res.Percentiles, err = r.readTimeSeriesPercentiles(window, totalRequests)
	if err != nil {
		return nil, err
	}
	res.ModalityStats, err = r.readTimeSeriesModalities(window)
	if err != nil {
		return nil, err
	}
	cacheTimeSeries(cacheKey, res)
	return res, nil
}

func normalizeTimeframe(timeframe string) string {
	if timeframe != "1h" && timeframe != "7d" {
		return "24h"
	}
	return timeframe
}

func timeSeriesBuckets(timeframe string, now time.Time) ([]*TimeSeriesBucket, time.Duration) {
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

	return buckets, interval
}
