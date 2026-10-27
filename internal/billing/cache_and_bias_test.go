package billing

import (
	"strings"
	"testing"
	"time"

	"github.com/ifnodoraemon/airoute/internal/storage"
	"github.com/ifnodoraemon/airoute/internal/telemetry"
)

// engineWithPrice builds an engine holding one price row, without a repository.
func engineWithPrice(rec *storage.ModelPriceRecord) *BillingEngine {
	engine := NewEngine(nil)
	engine.mu.Lock()
	engine.prices = map[string]*storage.ModelPriceRecord{strings.ToLower(rec.Model): rec}
	engine.mu.Unlock()
	return engine
}

// Cache writes cost a premium over the plain input rate (Anthropic: 1.25x). They
// used to be billed at the plain prompt rate, under-charging every cache miss.
func TestCacheWritesArePricedAtTheCacheWritePremium(t *testing.T) {
	rec := &storage.ModelPriceRecord{
		Model: "cache-model", GroupName: "default",
		PromptPrice: 2, CompletionPrice: 4, CacheReadPrice: 0.2, Currency: "CNY",
	}
	engine := engineWithPrice(rec)

	cost, _, cacheSaved, _, _, snapshot := engine.CalculateCostForUnitsWithCacheCreation(
		"cache-model", "default", 1900, 100, 500, 400, 1, time.Now())
	// 1000 uncached * 2.0 + 500 cached * 0.2 + 400 cache-write * (2.0*1.25) + 100 * 4.0
	const want = 0.0035
	if diff := cost - want; diff > 1e-9 || diff < -1e-9 {
		t.Fatalf("cost = %v, want %v", cost, want)
	}
	if snapshot.CacheCreationTokens != 400 {
		t.Fatalf("snapshot cache creation tokens = %d, want 400", snapshot.CacheCreationTokens)
	}
	if snapshot.CacheWritePrice != 2.5 {
		t.Fatalf("snapshot cache write price = %v, want 2.5 (1.25 x prompt price)", snapshot.CacheWritePrice)
	}
	if cacheSaved <= 0 {
		t.Fatalf("cache savings = %v, want the cache-read savings to stay reported", cacheSaved)
	}

	// Callers without cache writes are unaffected: prompt = 1900 with 500 cached.
	base, _, _, _, _, baseSnapshot := engine.CalculateCostForUnits("cache-model", "default", 1900, 100, 500, 1, time.Now())
	const wantBase = 0.0033 // 1400*2.0 + 500*0.2 + 100*4.0
	if diff := base - wantBase; diff > 1e-9 || diff < -1e-9 {
		t.Fatalf("cost without cache writes = %v, want %v", base, wantBase)
	}
	if baseSnapshot.CacheCreationTokens != 0 {
		t.Fatalf("snapshot cache creation tokens = %d, want 0", baseSnapshot.CacheCreationTokens)
	}

	// The three buckets can never exceed the prompt: an over-reported cache write is
	// clamped, not double charged.
	clamped, _, _, _, _, _ := engine.CalculateCostForUnitsWithCacheCreation(
		"cache-model", "default", 100, 0, 60, 1000, 1, time.Now())
	// 60 tokens stay cache reads, the remaining 40 prompt tokens are billed as cache
	// writes (creation clamped to prompt-cached), nothing is left at the plain rate.
	const wantClamped = (60*0.2 + 40*2.5) / 1_000_000.0
	if diff := clamped - wantClamped; diff > 1e-9 || diff < -1e-9 {
		t.Fatalf("clamped cost = %v, want %v", clamped, wantClamped)
	}
}

// The estimator-bias gauge used to hold the last observed model's ratio, so one
// busy model silently replaced the fleet-wide number. It must be the
// sample-weighted mean, and each model must stay visible on its own series.
func TestEstimateBiasMetricIsWeightedAcrossModels(t *testing.T) {
	previous := telemetry.GlobalMetrics
	telemetry.GlobalMetrics = &telemetry.Metrics{}
	t.Cleanup(func() { telemetry.GlobalMetrics = previous })

	calibrator := NewEstimateCalibrator()
	// model-a: upstream counts 2x our estimate, 10 observations => +1_000_000 ppm.
	for i := 0; i < 10; i++ {
		calibrator.Observe("model-a", 100, 200)
	}
	// model-b: upstream counts half, 1 observation => -500_000 ppm.
	calibrator.Observe("model-b", 100, 50)

	// Weighted mean = (1_000_000*10 + (-500_000)*1) / 11.
	const want = 863636
	if got := telemetry.GlobalMetrics.EstimateBiasPPM.Load(); got != want {
		t.Fatalf("global estimate bias = %d ppm, want %d (sample-weighted mean; the last observation alone was -500000)", got, want)
	}

	exposition := telemetry.GlobalMetrics.ToPrometheusFormat()
	for _, wantSeries := range []string{
		`airoute_billing_estimate_bias_ppm{model="model-a"} 1000000`,
		`airoute_billing_estimate_bias_ppm{model="model-b"} -500000`,
	} {
		if !strings.Contains(exposition, wantSeries) {
			t.Fatalf("per-model bias series missing (%s):\n%s", wantSeries, exposition)
		}
	}
}
