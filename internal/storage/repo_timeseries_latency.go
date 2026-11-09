package storage

import (
	"fmt"
	"math"
	"os"
	"sort"
	"strconv"
	"strings"
)

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

func (r *Repository) readTimeSeriesPercentiles(window timeSeriesWindow, totalRequests int64) (LatencyPercentiles, error) {
	sampleArgs := append(append([]any{window.startValue}, window.keyArgs...), ttftSampleLimit())
	sampleRows, err := r.db.Query(`SELECT ttft_ms FROM usage_logs WHERE created_at >= ?`+window.keyCond+` AND ttft_ms > 0 ORDER BY id DESC LIMIT ?`, sampleArgs...)
	if err != nil {
		return LatencyPercentiles{}, fmt.Errorf("query timeseries ttft sample error: %w", err)
	}
	ttftSample := make([]int64, 0, 256)
	for sampleRows.Next() {
		var ttft int64
		if err := sampleRows.Scan(&ttft); err != nil {
			sampleRows.Close()
			return LatencyPercentiles{}, fmt.Errorf("scan timeseries ttft sample error: %w", err)
		}
		ttftSample = append(ttftSample, ttft)
	}
	if err := sampleRows.Err(); err != nil {
		sampleRows.Close()
		return LatencyPercentiles{}, fmt.Errorf("iterate timeseries ttft sample error: %w", err)
	}
	sampleRows.Close()

	return timeSeriesPercentiles(ttftSample, totalRequests), nil
}

func timeSeriesPercentiles(ttftSample []int64, totalRequests int64) LatencyPercentiles {
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

	return percentiles
}
