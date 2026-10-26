package telemetry

import (
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
	// ClientCanceledRequests counts requests the downstream abandoned. They are
	// neither success nor failure: a client hanging up must not inflate the
	// gateway error rate (it used to be recorded as a 502 upstream failure).
	ClientCanceledRequests atomic.Uint64
	ActiveConnections      atomic.Int64

	TotalPromptTokens     atomic.Uint64
	TotalCompletionTokens atomic.Uint64

	// Usage-log ingestion pipeline. Audit rows are billing evidence, so a drop
	// has to be visible on /metrics rather than only in a slog line nobody
	// watches.
	UsageLogRecordsFlushed atomic.Uint64 // rows persisted successfully
	UsageLogRecordsDropped atomic.Uint64 // rows lost to a full buffer or repeated write failures
	UsageLogFlushFailures  atomic.Uint64 // failed batch writes (each is retried)
	UsageLogQueueDepth     atomic.Int64  // rows currently waiting in the local buffer
	// BillingDuplicatesSkipped counts at-least-once deliveries that were
	// rejected by the usage_logs.billing_id unique index. A non-zero value is
	// expected occasionally (Redis redelivery); a sustained rate means the
	// consumer is failing to XACK and should be investigated.
	BillingDuplicatesSkipped atomic.Uint64
	// BillingOverdrafts counts wallet charges that left a negative balance (the
	// account was served on credit). Sustained non-zero means the in-flight
	// reserve is too small for the prompts being served.
	BillingOverdrafts atomic.Uint64
	// BillingSettledSynchronously counts charges applied at request completion,
	// before the audit row was written.
	BillingSettledSynchronously atomic.Uint64
	// BillingSettlementsDeferred counts charges that fell back to the async
	// pipeline because the synchronous settlement could not run.
	BillingSettlementsDeferred atomic.Uint64

	// UsageLogRecordsSpilled counts billing records written to the local
	// append-only spill file because no queue accepted them. Any non-zero value
	// means the pipeline is degraded; the records are replayed on restart.
	UsageLogRecordsSpilled atomic.Uint64
	// UsageLogSpillReplayFailures counts unreadable spill lines.
	UsageLogSpillReplayFailures atomic.Uint64
	// BillingLedgerDriftUnits is the absolute per-key difference between
	// api_keys.used_cost_units and SUM(usage_logs.cost_units), refreshed by the
	// reconciliation worker.
	BillingLedgerDriftUnits atomic.Int64
	// BillingWalletDriftUnits is the absolute difference between users.balance_units
	// and its float mirror, refreshed by the reconciliation worker.
	BillingWalletDriftUnits atomic.Int64
	// EstimateBiasPPM is how far the local token estimate sits from the upstream
	// tokenizer for the most recently observed model, in parts per million
	// (positive = upstream counts more tokens than we estimated).
	EstimateBiasPPM atomic.Int64
	// BillingAuditLagUnits is the absolute difference between the settlement
	// ledger (money actually moved) and the audit rows. It is expected to be
	// non-zero briefly while an audit write is retried; a sustained value means
	// audit rows are being lost while charges still settle.
	BillingAuditLagUnits atomic.Int64

	// Latency & TTFT tracking
	mu             sync.RWMutex
	ttftSumMs      float64
	ttftCount      uint64
	totalDurMs     float64
	durCount       uint64
	maxDurMs       float64
	maxTTFTMs      float64
	latencyBuckets map[float64]uint64
	ttftBuckets    map[float64]uint64
	modelRequests  map[string]map[string]uint64 // model -> status -> count
}

var (
	// Long-context streaming regularly exceeds two minutes. The old top bucket
	// was 60s, so every slower request fell outside the histogram and p99 was
	// reported as exactly 60000 while the real maximum was 113128ms.
	defaultLatencyBuckets = []float64{25, 50, 100, 250, 500, 1000, 2500, 5000, 10000, 30000, 60000, 120000, 300000, 600000}
	defaultTTFTBuckets    = []float64{10, 25, 50, 100, 250, 500, 1000, 2500, 5000, 10000, 30000, 60000, 120000}
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
	outcome := "failed"
	if success {
		outcome = "success"
	}
	m.recordRequestStats(modelName, outcome, dur, promptTokens, completionTokens)
}

// RecordClientCanceled records a request the downstream abandoned (client closed
// before the gateway finished). It counts toward total requests and latency but
// not toward failures.
func (m *Metrics) RecordClientCanceled(modelName string, dur time.Duration, promptTokens, completionTokens int) {
	m.recordRequestStats(modelName, "canceled", dur, promptTokens, completionTokens)
}

func (m *Metrics) recordRequestStats(modelName, outcome string, dur time.Duration, promptTokens, completionTokens int) {
	m.TotalRequests.Add(1)
	switch outcome {
	case "success":
		m.SuccessfulRequests.Add(1)
	case "canceled":
		m.ClientCanceledRequests.Add(1)
	default:
		m.FailedRequests.Add(1)
	}
	m.TotalPromptTokens.Add(uint64(promptTokens))
	m.TotalCompletionTokens.Add(uint64(completionTokens))

	ms := float64(dur.Milliseconds())
	m.mu.Lock()
	m.totalDurMs += ms
	m.durCount++
	if ms > m.maxDurMs {
		m.maxDurMs = ms
	}
	if m.latencyBuckets == nil {
		m.latencyBuckets = make(map[float64]uint64)
	}
	for _, b := range defaultLatencyBuckets {
		if ms <= b {
			m.latencyBuckets[b]++
		}
	}
	if modelName != "" {
		cleanModel := strings.TrimSpace(modelName)
		if len(cleanModel) > 64 {
			cleanModel = cleanModel[:64]
		}
		if m.modelRequests == nil {
			m.modelRequests = make(map[string]map[string]uint64)
		}
		if _, exists := m.modelRequests[cleanModel]; !exists && len(m.modelRequests) >= 500 {
			cleanModel = "_unrecognized"
		}
		statusKey := outcome
		if m.modelRequests[cleanModel] == nil {
			m.modelRequests[cleanModel] = make(map[string]uint64)
		}
		m.modelRequests[cleanModel][statusKey]++
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
	p50 = estimateQuantile(0.50, m.durCount, defaultLatencyBuckets, m.latencyBuckets, m.maxDurMs)
	p90 = estimateQuantile(0.90, m.durCount, defaultLatencyBuckets, m.latencyBuckets, m.maxDurMs)
	p99 = estimateQuantile(0.99, m.durCount, defaultLatencyBuckets, m.latencyBuckets, m.maxDurMs)
	return p50, p90, p99
}

// estimateQuantile interpolates within cumulative "less or equal" buckets.
// overflowMax is the largest observation actually recorded; it is reported when
// the target rank lies beyond the widest bucket, so a tail slower than the last
// bucket is never silently clamped to that bucket's upper bound.
func estimateQuantile(q float64, totalCount uint64, buckets []float64, counts map[float64]uint64, overflowMax float64) float64 {
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
		if overflowMax > buckets[len(buckets)-1] {
			return overflowMax
		}
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
	if ms > m.maxTTFTMs {
		m.maxTTFTMs = ms
	}
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
