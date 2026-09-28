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
	mu         sync.RWMutex
	ttftSumMs  float64
	ttftCount  uint64
	totalDurMs float64
	durCount   uint64
}

var GlobalMetrics = &Metrics{}

// IncActiveConns increments active connection gauge.
func (m *Metrics) IncActiveConns() {
	m.ActiveConnections.Add(1)
}

// DecActiveConns decrements active connection gauge.
func (m *Metrics) DecActiveConns() {
	m.ActiveConnections.Add(-1)
}

// RecordRequest records a completed request with its duration and token count.
func (m *Metrics) RecordRequest(success bool, dur time.Duration, promptTokens, completionTokens int) {
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
	m.mu.Unlock()
}

// RecordTTFT records the Time To First Token for a streaming request.
func (m *Metrics) RecordTTFT(d time.Duration) {
	ms := float64(d.Milliseconds())
	m.mu.Lock()
	m.ttftSumMs += ms
	m.ttftCount++
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
	m.mu.RUnlock()

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

	return sb.String()
}
