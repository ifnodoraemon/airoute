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
	// BillingChargesRejected counts charges that were deliberately closed out
	// without moving money because their pricing snapshot is not settled in CNY
	// (a legacy foreign-currency rate row). The audit row is still written; a
	// non-zero value means a price row needs re-pricing.
	BillingChargesRejected atomic.Uint64
	// BillingUnattributed counts charges whose settlement ledger row was recorded
	// but which no wallet could absorb: the api key has no owner row and no tenant
	// matched a user. The money is uncollected, so this must never be silent.
	BillingUnattributed atomic.Uint64

	// UsageLogRecordsSpilled counts billing records written to the local
	// append-only spill file because no queue accepted them. Any non-zero value
	// means the pipeline is degraded; the records are replayed by the periodic
	// replay loop (and again at the next start if they cannot be persisted).
	UsageLogRecordsSpilled atomic.Uint64
	// UsageLogSpillReplayFailures counts unreadable spill lines.
	UsageLogSpillReplayFailures atomic.Uint64
	// UsageLogSpillReplays counts records successfully replayed from a spill file.
	UsageLogSpillReplays atomic.Uint64
	// UsageLogStreamReadFailures counts failed XREADGROUP calls on the distributed
	// audit stream. A sustained rate means the consumer cannot read (a lost
	// consumer group, a Redis outage) — the condition that used to be invisible
	// while writers kept appending and nothing was ever persisted.
	UsageLogStreamReadFailures atomic.Uint64
	// UsageLogGroupRecreated counts successful consumer-group rebuilds after the
	// group went missing. Non-zero means a Redis state loss was recovered without
	// a process restart.
	UsageLogGroupRecreated atomic.Uint64
	// UsageLogStreamPoisoned counts stream entries that could not be decoded and
	// were dead-lettered instead of being silently acknowledged and dropped.
	UsageLogStreamPoisoned atomic.Uint64
	// BillingLedgerPruneContended counts ledger retention batches that collided
	// with another replica and had to be retried.
	BillingLedgerPruneContended atomic.Uint64
	// BillingLedgerDriftUnits is the absolute per-key difference between
	// api_keys.used_cost_units and SUM(usage_logs.cost_units), refreshed by the
	// reconciliation worker.
	BillingLedgerDriftUnits atomic.Int64
	// BillingWalletDriftUnits is the absolute difference between users.balance_units
	// and its float mirror, refreshed by the reconciliation worker.
	BillingWalletDriftUnits atomic.Int64
	// AbortedDrainedRequests and AbortedDrainedCompletionTokens quantify the cost
	// of keeping an upstream alive after the client left, so the extra spend that
	// exact billing buys is visible (drained completion tokens are paid for and
	// then discarded).
	AbortedDrainedRequests         atomic.Uint64
	AbortedDrainedCompletionTokens atomic.Uint64
	// EstimateBiasPPM is how far the local token estimate sits from the upstream
	// tokenizer, in parts per million (positive = upstream counts more tokens than
	// we estimated). It is the sample-weighted mean across every model, not the
	// last observation: a single traffic burst on one model used to overwrite the
	// fleet-wide number and hide the others.
	EstimateBiasPPM atomic.Int64
	// estimateBiasByModel holds the per-model bias, keyed by model name, so the
	// metric can report which model is mis-estimated instead of one blended value.
	// Guarded by mu (the latency maps' lock) to keep the Metrics struct simple.
	//
	// The map is bounded exactly like modelRequests (modelLabelCap entries plus the
	// modelLabelOverflow bucket): the model name comes from the client's request, so
	// an unbounded map would be an unbounded memory leak and an unbounded Prometheus
	// label set.
	estimateBiasByModel map[string]estimateBiasSample
	// estimateBiasWeightedPPM and estimateBiasWeights keep the unlabelled
	// sample-weighted mean incrementally, so publishing one model's bias is O(1)
	// instead of re-summing the whole map on every upstream usage report (it runs on
	// the request path).
	estimateBiasWeightedPPM int64
	estimateBiasWeights     int64
	// BillingAuditLagUnits is the absolute difference between the settlement
	// ledger (money actually moved) and the audit rows. It is expected to be
	// non-zero briefly while an audit write is retried; a sustained value means
	// audit rows are being lost while charges still settle.
	BillingAuditLagUnits atomic.Int64
	// BillingAuditPrunedUnits is the running total of charge units whose audit row
	// was deliberately removed (retention pruning, or an admin clearing the trail).
	// auditLag adds it back so the gauge measures unexplained loss instead of
	// growing forever after the first cleanup.
	BillingAuditPrunedUnits atomic.Int64
	// BillingLedgerUnlinkedUnits is the amount recorded in the settlement ledger
	// that no audit row can be matched to (using the same legacy-key expression the
	// reconciler uses). A non-zero value means delivered charges lost their audit
	// row.
	BillingLedgerUnlinkedUnits atomic.Int64

	// Settlement batching (group commit). Charges are merged into one database
	// transaction; these counters make the batching observable and let an
	// operator tell "the writer is saturated" from "requests are waiting".
	SettlementBatches      atomic.Uint64 // flush transactions executed
	SettlementBatchRecords atomic.Uint64 // charges applied inside those flushes
	SettlementQueueDepth   atomic.Int64  // charges waiting for a flush slot
	// SettlementWaitTimeouts counts charges whose request-side wait expired
	// before the flush committed. They are not lost: the record still travels
	// through the audit pipeline and is settled there, but a sustained rate
	// means the writer cannot keep up with the offered load.
	SettlementWaitTimeouts atomic.Uint64
	// SettlementLockTimeouts counts settlement attempts that gave up on a row
	// lock deadline. Non-zero means a hot account is serialising charges.
	SettlementLockTimeouts atomic.Uint64
	// BillingReservedUnits is the amount currently pre-authorised against
	// wallets (1e-9 units). It is the exposure the admission gate refuses to
	// exceed; a value that tracks close to the sum of balances means the
	// reservation is doing the work the balance check alone used to skip.
	BillingReservedUnits atomic.Int64
	// BillingWalletTotalUnits is the sum of every wallet's authoritative balance
	// (1e-9 units), refreshed by the reconciliation worker. It gives the
	// reserved-units gauge a denominator: pre-authorisation is only meaningful
	// relative to the money actually held.
	BillingWalletTotalUnits atomic.Int64
	// BillingWaivedUnits accumulates money delivered but not collected because a
	// charge hit the overdraft floor (1e-9 units). The wallet is clamped, so this
	// is the only place the shortfall is visible at all.
	BillingWaivedUnits atomic.Int64

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

const (
	// modelLabelCap bounds the per-model label maps. Model names come from the
	// client's request, so without a cap a caller could grow the process and the
	// Prometheus series set without bound (the exact failure mode modelRequests was
	// already protected against).
	modelLabelCap = 500
	// modelLabelOverflow is the bucket every model beyond the cap folds into.
	modelLabelOverflow = "_unrecognized"
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
		if _, exists := m.modelRequests[cleanModel]; !exists && len(m.modelRequests) >= modelLabelCap {
			cleanModel = modelLabelOverflow
		}
		statusKey := outcome
		if m.modelRequests[cleanModel] == nil {
			m.modelRequests[cleanModel] = make(map[string]uint64)
		}
		m.modelRequests[cleanModel][statusKey]++
	}
	m.mu.Unlock()
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

// estimateBiasSample is one model's estimator-vs-upstream ratio, expressed in ppm
// and weighted by how many upstream reports produced it.
type estimateBiasSample struct {
	ppm     int64
	samples uint64
}

// weight is this sample's weight in the fleet-wide mean: the number of upstream
// reports behind it, or 1 when the count is unknown (a zero-weight sample would be
// invisible in the mean).
func (s estimateBiasSample) weight() int64 {
	if s.samples == 0 {
		return 1
	}
	return int64(s.samples)
}

// SetEstimateBias publishes the estimator bias for one model.
//
// The per-model value is what an operator needs (a single model with a bad
// tokenizer guess), and the unlabelled total is its sample-weighted mean, so
// neither number is decided by whichever request happened to be observed last.
//
// Two properties the previous implementation lacked:
//
//   - the model key is bounded like modelRequests: the name comes from the
//     client's request, so without modelLabelCap the map (and the Prometheus label
//     set) grew without bound for the process lifetime;
//   - the weighted mean is maintained incrementally (subtract the previous value,
//     add the new one), so this call is O(1) instead of re-summing every model on
//     each upstream usage report.
func (m *Metrics) SetEstimateBias(model string, ppm int64, samples uint64) {
	if model == "" {
		// An empty label carries no information; the previous implementation also
		// stored nothing for it, which left the mean untouched.
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.estimateBiasByModel == nil {
		m.estimateBiasByModel = make(map[string]estimateBiasSample)
	}

	key := model
	if _, known := m.estimateBiasByModel[key]; !known && len(m.estimateBiasByModel) >= modelLabelCap {
		key = modelLabelOverflow
	}
	next := estimateBiasSample{ppm: ppm, samples: samples}
	if key == modelLabelOverflow {
		// Several models share this bucket. Keep a merged, sample-weighted view of
		// them (rather than letting whichever model was observed last overwrite the
		// bucket), so the bounded label still says "traffic outside the top models is
		// mis-estimated by about this much".
		if previous, ok := m.estimateBiasByModel[key]; ok {
			prevWeight, nextWeight := previous.weight(), next.weight()
			mergedSamples := previous.samples + samples
			mergedPPM := (previous.ppm*prevWeight + ppm*nextWeight) / (prevWeight + nextWeight)
			next = estimateBiasSample{ppm: mergedPPM, samples: mergedSamples}
		}
	}

	if previous, ok := m.estimateBiasByModel[key]; ok {
		m.estimateBiasWeightedPPM -= previous.ppm * previous.weight()
		m.estimateBiasWeights -= previous.weight()
	}
	m.estimateBiasByModel[key] = next
	m.estimateBiasWeightedPPM += next.ppm * next.weight()
	m.estimateBiasWeights += next.weight()
	if m.estimateBiasWeights > 0 {
		m.EstimateBiasPPM.Store(m.estimateBiasWeightedPPM / m.estimateBiasWeights)
	}
}

// EstimateBiasByModel returns a copy of the per-model bias samples.
func (m *Metrics) EstimateBiasByModel() map[string]int64 {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make(map[string]int64, len(m.estimateBiasByModel))
	for model, sample := range m.estimateBiasByModel {
		out[model] = sample.ppm
	}
	return out
}

// EstimateSamplesByModel returns how many upstream usage reports back each
// model's learned ratio. The learned ratio is only applied after a minimum number
// of samples, so the count is what separates "calibration is in effect" from "not
// enough data yet" - the bias gauge alone cannot express that difference.
func (m *Metrics) EstimateSamplesByModel() map[string]uint64 {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make(map[string]uint64, len(m.estimateBiasByModel))
	for model, sample := range m.estimateBiasByModel {
		out[model] = sample.samples
	}
	return out
}
