package telemetry

import (
	"strings"
	"testing"
	"time"
)

func TestMetrics_PercentilesAndModelBreakdown(t *testing.T) {
	m := &Metrics{
		latencyBuckets: make(map[float64]uint64),
		ttftBuckets:    make(map[float64]uint64),
		modelRequests:  make(map[string]map[string]uint64),
	}

	// Record varying latencies
	m.RecordRequestWithModel("gpt-4o", true, 30*time.Millisecond, 10, 20)
	m.RecordRequestWithModel("gpt-4o", true, 80*time.Millisecond, 15, 25)
	m.RecordRequestWithModel("claude-3-5-sonnet", true, 200*time.Millisecond, 20, 30)
	m.RecordRequestWithModel("claude-3-5-sonnet", false, 1500*time.Millisecond, 0, 0)
	m.RecordRequestWithModel("deepseek-chat", true, 6000*time.Millisecond, 50, 100)

	p50, p90, p99 := m.GetLatencyPercentiles()
	if p50 <= 0 || p90 <= 0 || p99 <= 0 {
		t.Fatalf("expected positive percentiles, got p50=%.2f, p90=%.2f, p99=%.2f", p50, p90, p99)
	}
	if p50 > p90 || p90 > p99 {
		t.Fatalf("expected p50 <= p90 <= p99, got p50=%.2f, p90=%.2f, p99=%.2f", p50, p90, p99)
	}

	// Verify Prometheus format
	promText := m.ToPrometheusFormat()
	if !strings.Contains(promText, "airoute_latency_p50_ms") {
		t.Fatalf("expected Prometheus text to contain airoute_latency_p50_ms")
	}
	if !strings.Contains(promText, "airoute_latency_p99_ms") {
		t.Fatalf("expected Prometheus text to contain airoute_latency_p99_ms")
	}
	if !strings.Contains(promText, `airoute_model_requests_total{model="gpt-4o",status="success"} 2`) {
		t.Fatalf("expected Prometheus text to contain model metric for gpt-4o success, got:\n%s", promText)
	}
	if !strings.Contains(promText, `airoute_model_requests_total{model="claude-3-5-sonnet",status="failed"} 1`) {
		t.Fatalf("expected Prometheus text to contain model metric for claude failure, got:\n%s", promText)
	}
}

// The learned token ratio is only applied after a minimum number of samples, so
// the sample count must be observable: without it a bias value cannot be told
// apart from one that is deliberately not in effect yet.
func TestMetrics_EstimateCalibrationSamplesAreExposed(t *testing.T) {
	m := &Metrics{
		latencyBuckets: make(map[float64]uint64),
		ttftBuckets:    make(map[float64]uint64),
		modelRequests:  make(map[string]map[string]uint64),
	}
	m.SetEstimateBias("gpt-4o", 200000, 7)
	m.SetEstimateBias("claude-3-5-sonnet", -100000, 3)

	samples := m.EstimateSamplesByModel()
	if samples["gpt-4o"] != 7 || samples["claude-3-5-sonnet"] != 3 {
		t.Fatalf("per-model samples = %v, want 7 and 3", samples)
	}

	promText := m.ToPrometheusFormat()
	for _, want := range []string{
		`airoute_billing_estimate_samples{model="gpt-4o"} 7`,
		`airoute_billing_estimate_samples{model="claude-3-5-sonnet"} 3`,
		"airoute_billing_estimate_samples 10",
	} {
		if !strings.Contains(promText, want) {
			t.Fatalf("expected %q in Prometheus text, got:\n%s", want, promText)
		}
	}
}

// Usage-log rows are billing evidence. They used to be dropped with only a slog
// line, so a silent loss was invisible to monitoring.
func TestMetrics_UsageLogPipelineIsExposed(t *testing.T) {
	m := &Metrics{
		latencyBuckets: make(map[float64]uint64),
		ttftBuckets:    make(map[float64]uint64),
		modelRequests:  make(map[string]map[string]uint64),
	}
	m.UsageLogRecordsFlushed.Add(120)
	m.UsageLogRecordsDropped.Add(3)
	m.UsageLogFlushFailures.Add(1)
	m.UsageLogQueueDepth.Store(7)

	promText := m.ToPrometheusFormat()
	for _, want := range []string{
		"airoute_usage_log_records_flushed_total 120",
		"airoute_usage_log_records_dropped_total 3",
		"airoute_usage_log_flush_failures_total 1",
		"airoute_usage_log_queue_depth 7",
	} {
		if !strings.Contains(promText, want) {
			t.Fatalf("expected %q in Prometheus text, got:\n%s", want, promText)
		}
	}
}
