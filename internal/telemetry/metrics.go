package telemetry

import (
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
