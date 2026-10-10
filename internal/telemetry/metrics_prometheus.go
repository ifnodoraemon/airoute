package telemetry

import (
	"fmt"
	"strings"
)

// ToPrometheusFormat exports all metrics in standard Prometheus exposition text format.
func (m *Metrics) ToPrometheusFormat() string {
	m.mu.RLock()
	var avgTTFT float64
	if m.ttftCount > 0 {
		avgTTFT = m.ttftSumMs / float64(m.ttftCount)
	}
	var avgLatency float64
	if m.durCount > 0 {
		avgLatency = m.totalDurMs / float64(m.durCount)
	}

	// Copy bucket counts under read lock
	latCounts := make(map[float64]uint64, len(defaultLatencyBuckets))
	for _, b := range defaultLatencyBuckets {
		latCounts[b] = m.latencyBuckets[b]
	}
	ttftCounts := make(map[float64]uint64, len(defaultTTFTBuckets))
	for _, b := range defaultTTFTBuckets {
		ttftCounts[b] = m.ttftBuckets[b]
	}
	durTotal := m.totalDurMs
	durCount := m.durCount
	ttftTotal := m.ttftSumMs
	ttftCount := m.ttftCount

	// Copy model request metrics
	modelMap := make(map[string]map[string]uint64, len(m.modelRequests))
	for k, v := range m.modelRequests {
		sub := make(map[string]uint64, len(v))
		for sk, sv := range v {
			sub[sk] = sv
		}
		modelMap[k] = sub
	}
	m.mu.RUnlock()

	p50, p90, p99 := m.GetLatencyPercentiles()

	var sb strings.Builder
	sb.WriteString("# HELP airoute_requests_total Total number of HTTP requests\n")
	sb.WriteString("# TYPE airoute_requests_total counter\n")
	sb.WriteString(fmt.Sprintf("airoute_requests_total %d\n", m.TotalRequests.Load()))

	sb.WriteString("# HELP airoute_requests_success_total Total successful requests\n")
	sb.WriteString("# TYPE airoute_requests_success_total counter\n")
	sb.WriteString(fmt.Sprintf("airoute_requests_success_total %d\n", m.SuccessfulRequests.Load()))

	sb.WriteString("# HELP airoute_requests_failed_total Total failed requests\n")
	sb.WriteString("# TYPE airoute_requests_failed_total counter\n")
	sb.WriteString(fmt.Sprintf("airoute_requests_failed_total %d\n", m.FailedRequests.Load()))

	sb.WriteString("# HELP airoute_fallback_total Total channel fallback events\n")
	sb.WriteString("# TYPE airoute_fallback_total counter\n")
	sb.WriteString(fmt.Sprintf("airoute_fallback_total %d\n", m.FallbackRequests.Load()))

	sb.WriteString("# HELP airoute_active_connections Current active connections\n")
	sb.WriteString("# TYPE airoute_active_connections gauge\n")
	sb.WriteString(fmt.Sprintf("airoute_active_connections %d\n", m.ActiveConnections.Load()))

	sb.WriteString("# HELP airoute_prompt_tokens_total Total prompt tokens processed\n")
	sb.WriteString("# TYPE airoute_prompt_tokens_total counter\n")
	sb.WriteString(fmt.Sprintf("airoute_prompt_tokens_total %d\n", m.TotalPromptTokens.Load()))

	sb.WriteString("# HELP airoute_completion_tokens_total Total completion tokens generated\n")
	sb.WriteString("# TYPE airoute_completion_tokens_total counter\n")
	sb.WriteString(fmt.Sprintf("airoute_completion_tokens_total %d\n", m.TotalCompletionTokens.Load()))

	sb.WriteString("# HELP airoute_ttft_avg_ms Average Time To First Token in milliseconds\n")
	sb.WriteString("# TYPE airoute_ttft_avg_ms gauge\n")
	sb.WriteString(fmt.Sprintf("airoute_ttft_avg_ms %.2f\n", avgTTFT))

	sb.WriteString("# HELP airoute_latency_avg_ms Average request latency in milliseconds\n")
	sb.WriteString("# TYPE airoute_latency_avg_ms gauge\n")
	sb.WriteString(fmt.Sprintf("airoute_latency_avg_ms %.2f\n", avgLatency))

	// Quantile latency percentiles
	sb.WriteString("# HELP airoute_latency_p50_ms Estimated P50 latency in milliseconds\n")
	sb.WriteString("# TYPE airoute_latency_p50_ms gauge\n")
	sb.WriteString(fmt.Sprintf("airoute_latency_p50_ms %.2f\n", p50))

	sb.WriteString("# HELP airoute_latency_p90_ms Estimated P90 latency in milliseconds\n")
	sb.WriteString("# TYPE airoute_latency_p90_ms gauge\n")
	sb.WriteString(fmt.Sprintf("airoute_latency_p90_ms %.2f\n", p90))

	sb.WriteString("# HELP airoute_latency_p99_ms Estimated P99 latency in milliseconds\n")
	sb.WriteString("# TYPE airoute_latency_p99_ms gauge\n")
	sb.WriteString(fmt.Sprintf("airoute_latency_p99_ms %.2f\n", p99))

	// Multi-dimensional model request metrics
	if len(modelMap) > 0 {
		sb.WriteString("# HELP airoute_model_requests_total Total number of requests partitioned by model and status\n")
		sb.WriteString("# TYPE airoute_model_requests_total counter\n")
		for mod, stats := range modelMap {
			for st, cnt := range stats {
				sb.WriteString(fmt.Sprintf("airoute_model_requests_total{model=\"%s\",status=\"%s\"} %d\n", mod, st, cnt))
			}
		}
	}

	// Prometheus Histogram for Request Duration
	sb.WriteString("# HELP airoute_request_duration_ms Request latency duration histogram in milliseconds\n")
	sb.WriteString("# TYPE airoute_request_duration_ms histogram\n")
	for _, b := range defaultLatencyBuckets {
		sb.WriteString(fmt.Sprintf("airoute_request_duration_ms_bucket{le=\"%.0f\"} %d\n", b, latCounts[b]))
	}
	sb.WriteString(fmt.Sprintf("airoute_request_duration_ms_bucket{le=\"+Inf\"} %d\n", durCount))
	sb.WriteString(fmt.Sprintf("airoute_request_duration_ms_sum %.2f\n", durTotal))
	sb.WriteString(fmt.Sprintf("airoute_request_duration_ms_count %d\n", durCount))

	// Prometheus Histogram for TTFT
	sb.WriteString("# HELP airoute_ttft_duration_ms Time To First Token histogram in milliseconds\n")
	sb.WriteString("# TYPE airoute_ttft_duration_ms histogram\n")
	for _, b := range defaultTTFTBuckets {
		sb.WriteString(fmt.Sprintf("airoute_ttft_duration_ms_bucket{le=\"%.0f\"} %d\n", b, ttftCounts[b]))
	}
	sb.WriteString(fmt.Sprintf("airoute_ttft_duration_ms_bucket{le=\"+Inf\"} %d\n", ttftCount))
	sb.WriteString(fmt.Sprintf("airoute_ttft_duration_ms_sum %.2f\n", ttftTotal))
	sb.WriteString(fmt.Sprintf("airoute_ttft_duration_ms_count %d\n", ttftCount))

	return sb.String()
}
