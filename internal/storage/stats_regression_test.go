package storage

import (
	"math"
	"testing"
	"time"
)

// Regression (ST-14): the time series used to materialise every usage_logs row in
// the window (48k rows for 7d on the live cluster) to fold them into buckets in
// Go. The aggregation now happens in SQL, which keeps the output contract:
// buckets, per-bucket sums, errors, average TTFT, modality distribution and
// percentiles with an explicit sample size.
func TestTimeSeriesAggregatesInSQLWithBoundedPercentileSample(t *testing.T) {
	repo := newRetentionRepo(t)
	// tsCache is package-level with a 3s TTL, so a previous test's "24h" result
	// would otherwise be served here and hide the aggregation under test.
	tsCacheMu.Lock()
	tsCache = make(map[string]struct {
		data *TimeSeriesData
		at   time.Time
	})
	tsCacheMu.Unlock()
	t.Setenv("GATEWAY_TIMESERIES_TTFT_SAMPLE", "3")
	now := time.Now()

	// Ten successful chat requests with known TTFT and tokens, plus one error.
	for i := 1; i <= 10; i++ {
		if err := repo.RecordUsageLog(&UsageLogRecord{
			APIKey: "k1", Model: "gpt-4o", StatusCode: 200,
			TotalTokens: 100, TTFTMs: int64(i * 10), Cost: 0.01,
			CreatedAt: now.Add(-time.Duration(i) * time.Minute),
		}); err != nil {
			t.Fatalf("record log %d: %v", i, err)
		}
	}
	if err := repo.RecordUsageLog(&UsageLogRecord{
		APIKey: "k1", Model: "dall-e-3", StatusCode: 500, TotalTokens: 100, Cost: 0.01,
		CreatedAt: now.Add(-30 * time.Minute),
	}); err != nil {
		t.Fatalf("record error log: %v", err)
	}

	data, err := repo.GetStatsTimeSeries("24h", nil)
	if err != nil {
		t.Fatalf("GetStatsTimeSeries: %v", err)
	}
	if len(data.Buckets) != 24 {
		t.Fatalf("bucket contract changed: got %d buckets, want 24", len(data.Buckets))
	}

	var requests, tokens, errors int64
	for _, bucket := range data.Buckets {
		requests += bucket.Requests
		tokens += bucket.Tokens
		errors += bucket.Errors
	}
	if requests != 11 {
		t.Fatalf("bucket requests = %d, want 11", requests)
	}
	if tokens != 1100 {
		t.Fatalf("bucket tokens = %d, want 1100", tokens)
	}
	if errors != 1 {
		t.Fatalf("bucket errors = %d, want 1 (the 500)", errors)
	}
	if data.ModalityStats["chat"] != 10 || data.ModalityStats["images"] != 1 {
		t.Fatalf("modality distribution wrong: %+v", data.ModalityStats)
	}

	// Percentiles must be drawn from a bounded sample but still report the full
	// population in Total.
	if data.Percentiles.Total != 11 {
		t.Fatalf("percentile total = %d, want 11", data.Percentiles.Total)
	}
	if data.Percentiles.Sampled > 3 {
		t.Fatalf("percentile sample = %d, want <= 3 (bounded)", data.Percentiles.Sampled)
	}
	if data.Percentiles.P50 == nil || data.Percentiles.P90 == nil || data.Percentiles.P99 == nil {
		t.Fatalf("percentiles missing: %+v", data.Percentiles)
	}
	if data.PeakRPM <= 0 {
		t.Fatalf("per-minute peak was not aggregated: peak_rpm=%d", data.PeakRPM)
	}
}

// Regression (ST-13): "saved cost" was cached_tokens * 1.8 CNY/1M regardless of
// the configured cache price. It must now be derived from model_prices.
func TestSavedCostIsDerivedFromCacheReadPrice(t *testing.T) {
	repo := newRetentionRepo(t)

	// prompt 4 CNY/M, cache-read 1 CNY/M => 3 CNY saved per 1M cached tokens.
	if err := repo.SaveModelPrice(&ModelPriceRecord{
		Model: "cache-model", PromptPrice: 4, CompletionPrice: 8, CacheReadPrice: 1,
	}); err != nil {
		t.Fatalf("save price: %v", err)
	}
	if err := repo.RecordUsageLog(&UsageLogRecord{
		APIKey: "k1", Model: "cache-model", StatusCode: 200,
		CachedTokens: 1000000, TotalTokens: 1000000, Cost: 3, CreatedAt: time.Now(),
	}); err != nil {
		t.Fatalf("record log: %v", err)
	}

	stats, err := repo.GetStatsOverview()
	if err != nil {
		t.Fatalf("GetStatsOverview: %v", err)
	}
	if math.Abs(stats.SavedCost-3.0) > 1e-6 {
		t.Fatalf("saved cost = %.6f, want 3.0 (1M cached tokens x (4-1)/1M)", stats.SavedCost)
	}
}

// A model without a price row contributes no saving (documented fallback) instead
// of being valued at a made-up constant.
func TestSavedCostSkipsUnpricedModels(t *testing.T) {
	repo := newRetentionRepo(t)

	if err := repo.RecordUsageLog(&UsageLogRecord{
		APIKey: "k1", Model: "unpriced-model", StatusCode: 200,
		CachedTokens: 1000000, TotalTokens: 1000000, Cost: 0, CreatedAt: time.Now(),
	}); err != nil {
		t.Fatalf("record log: %v", err)
	}
	stats, err := repo.GetStatsOverview()
	if err != nil {
		t.Fatalf("GetStatsOverview: %v", err)
	}
	if stats.SavedCost != 0 {
		t.Fatalf("saved cost = %.6f for an unpriced model, want 0", stats.SavedCost)
	}
}

// A cache price at or above the prompt price means "no saving"; the join must not
// report a negative discount.
func TestSavedCostNeverNegative(t *testing.T) {
	repo := newRetentionRepo(t)

	if err := repo.SaveModelPrice(&ModelPriceRecord{
		Model: "no-cache-discount", PromptPrice: 2, CompletionPrice: 4, CacheReadPrice: 2,
	}); err != nil {
		t.Fatalf("save price: %v", err)
	}
	if err := repo.RecordUsageLog(&UsageLogRecord{
		APIKey: "k1", Model: "no-cache-discount", StatusCode: 200,
		CachedTokens: 1000000, TotalTokens: 1000000, Cost: 2, CreatedAt: time.Now(),
	}); err != nil {
		t.Fatalf("record log: %v", err)
	}
	stats, err := repo.GetStatsOverview()
	if err != nil {
		t.Fatalf("GetStatsOverview: %v", err)
	}
	if stats.SavedCost < 0 {
		t.Fatalf("saved cost is negative: %.6f", stats.SavedCost)
	}
}
