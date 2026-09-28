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
