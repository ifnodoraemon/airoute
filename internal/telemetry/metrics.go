package telemetry

import (
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// Metrics holds the runtime counters and statistics.
type Metrics struct {
	TotalRequests      atomic.Uint64
	SuccessfulRequests atomic.Uint64
	FailedRequests     atomic.Uint64
	FallbackRequests   atomic.Uint64
	ActiveConnections  atomic.Int64

	TotalPromptTokens     atomic.Uint64
	TotalCompletionTokens atomic.Uint64

	// Latency & TTFT tracking
	mu             sync.RWMutex
	ttftSumMs      float64
	ttftCount      uint64
	totalDurMs     float64
	durCount       uint64
	latencyBuckets map[float64]uint64
	ttftBuckets    map[float64]uint64
	modelRequests  map[string]map[string]uint64 // model -> status -> count
}

var (
	defaultLatencyBuckets = []float64{25, 50, 100, 250, 500, 1000, 2500, 5000, 10000, 30000, 60000}
	defaultTTFTBuckets    = []float64{10, 25, 50, 100, 250, 500, 1000, 2500, 5000}
)

var GlobalMetrics = &Metrics{
	latencyBuckets: make(map[float64]uint64),
	ttftBuckets:    make(map[float64]uint64),
	modelRequests:  make(map[string]map[string]uint64),
}

// IncActiveConns increments active connection gauge.
func (m *Metrics) IncActiveConns() {
	m.ActiveConnections.Add(1)
}

// DecActiveConns decrements active connection gauge.
func (m *Metrics) DecActiveConns() {
	m.ActiveConnections.Add(-1)
}

// RecordRequestWithModel records a completed request with its model, duration and token count.
func (m *Metrics) RecordRequestWithModel(modelName string, success bool, dur time.Duration, promptTokens, completionTokens int) {
	m.TotalRequests.Add(1)
	if success {
		m.SuccessfulRequests.Add(1)
	} else {
		m.FailedRequests.Add(1)
	}
	m.TotalPromptTokens.Add(uint64(promptTokens))
	m.TotalCompletionTokens.Add(uint64(completionTokens))

	ms := float64(dur.Milliseconds())
	m.mu.Lock()
	m.totalDurMs += ms
	m.durCount++
	if m.latencyBuckets == nil {
		m.latencyBuckets = make(map[float64]uint64)
	}
	for _, b := range defaultLatencyBuckets {
		if ms <= b {
			m.latencyBuckets[b]++
		}
	}
	if modelName != "" {
		if m.modelRequests == nil {
			m.modelRequests = make(map[string]map[string]uint64)
		}
		statusKey := "failed"
		if success {
			statusKey = "success"
		}
		if m.modelRequests[modelName] == nil {
			m.modelRequests[modelName] = make(map[string]uint64)
		}
		m.modelRequests[modelName][statusKey]++
	}
	m.mu.Unlock()
}

// RecordRequest records a completed request with its duration and token count.
func (m *Metrics) RecordRequest(success bool, dur time.Duration, promptTokens, completionTokens int) {
	m.RecordRequestWithModel("", success, dur, promptTokens, completionTokens)
}

// GetLatencyPercentiles estimates P50, P90, and P99 latency percentiles in milliseconds.
func (m *Metrics) GetLatencyPercentiles() (p50, p90, p99 float64) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.durCount == 0 {
		return 0, 0, 0
	}
	p50 = estimateQuantile(0.50, m.durCount, defaultLatencyBuckets, m.latencyBuckets)
	p90 = estimateQuantile(0.90, m.durCount, defaultLatencyBuckets, m.latencyBuckets)
	p99 = estimateQuantile(0.99, m.durCount, defaultLatencyBuckets, m.latencyBuckets)
	return p50, p90, p99
}

func estimateQuantile(q float64, totalCount uint64, buckets []float64, counts map[float64]uint64) float64 {
	targetRank := q * float64(totalCount)
	var prevLe float64 = 0
	var prevCount uint64 = 0

	for _, b := range buckets {
		cnt := counts[b]
		if float64(cnt) >= targetRank {
			bucketWidth := b - prevLe
			countInBucket := float64(cnt - prevCount)
			if countInBucket <= 0 {
				return b
			}
			rankInBucket := targetRank - float64(prevCount)
			return prevLe + (rankInBucket/countInBucket)*bucketWidth
		}
		prevLe = b
		prevCount = cnt
	}
	if len(buckets) > 0 {
		return buckets[len(buckets)-1]
	}
	return 0
}

// RecordTTFT records the Time To First Token for a streaming request.
func (m *Metrics) RecordTTFT(d time.Duration) {
	ms := float64(d.Milliseconds())
	m.mu.Lock()
	m.ttftSumMs += ms
	m.ttftCount++
	if m.ttftBuckets == nil {
		m.ttftBuckets = make(map[float64]uint64)
	}
	for _, b := range defaultTTFTBuckets {
		if ms <= b {
			m.ttftBuckets[b]++
		}
	}
	m.mu.Unlock()
}

// RecordFallback records an occurrence of channel fallback.
func (m *Metrics) RecordFallback() {
	m.FallbackRequests.Add(1)
}

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
