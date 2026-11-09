package telemetry

import (
	"fmt"
	"strings"
)

var prometheusLabelReplacer = strings.NewReplacer(`\`, `\\`, `"`, `\"`, "\n", `\n`)

func escapePrometheusLabel(v string) string {
	return prometheusLabelReplacer.Replace(v)
}

type prometheusSnapshot struct {
	avgTTFT    float64
	avgLatency float64
	latCounts  map[float64]uint64
	ttftCounts map[float64]uint64
	durTotal   float64
	durCount   uint64
	ttftTotal  float64
	ttftCount  uint64
	modelMap   map[string]map[string]uint64
	p50        float64
	p90        float64
	p99        float64
}

// prometheusSnapshot copies shared aggregates under the existing read-lock boundary.
func (m *Metrics) prometheusSnapshot() prometheusSnapshot {
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

	return prometheusSnapshot{avgTTFT: avgTTFT, avgLatency: avgLatency, latCounts: latCounts, ttftCounts: ttftCounts, durTotal: durTotal, durCount: durCount, ttftTotal: ttftTotal, ttftCount: ttftCount, modelMap: modelMap, p50: p50, p90: p90, p99: p99}
}

// ToPrometheusFormat exports all metrics in standard Prometheus exposition text format.
func (m *Metrics) ToPrometheusFormat() string {
	stats := m.prometheusSnapshot()
	var sb strings.Builder
	m.writeRequestMetrics(&sb, stats)
	m.writePipelineMetrics(&sb, stats)
	m.writeEstimateMetrics(&sb, stats)
	m.writeSettlementMetrics(&sb, stats)
	m.writeLatencyMetrics(&sb, stats)
	m.writeModelMetrics(&sb, stats)
	m.writeDurationHistograms(&sb, stats)
	return sb.String()
}

func (m *Metrics) writeRequestMetrics(sb *strings.Builder, stats prometheusSnapshot) {
	writePrometheusScalar(sb, "airoute_requests_total", "Total number of HTTP requests", "counter", "%d", m.TotalRequests.Load())

	writePrometheusScalar(sb, "airoute_requests_success_total", "Total successful requests", "counter", "%d", m.SuccessfulRequests.Load())

	writePrometheusScalar(sb, "airoute_requests_failed_total", "Total failed requests", "counter", "%d", m.FailedRequests.Load())

	writePrometheusScalar(sb, "airoute_requests_client_canceled_total", "Requests abandoned by the downstream", "counter", "%d", m.ClientCanceledRequests.Load())

	writePrometheusScalar(sb, "airoute_fallback_total", "Total channel fallback events", "counter", "%d", m.FallbackRequests.Load())

	writePrometheusScalar(sb, "airoute_active_connections", "Current active connections", "gauge", "%d", m.ActiveConnections.Load())

	writePrometheusScalar(sb, "airoute_prompt_tokens_total", "Total prompt tokens processed", "counter", "%d", m.TotalPromptTokens.Load())

	writePrometheusScalar(sb, "airoute_completion_tokens_total", "Total completion tokens generated", "counter", "%d", m.TotalCompletionTokens.Load())
}

func (m *Metrics) writePipelineMetrics(sb *strings.Builder, stats prometheusSnapshot) {
	// Usage-log ingestion pipeline: a drop means an audit/billing row was lost.
	writePrometheusScalar(sb, "airoute_usage_log_records_flushed_total", "Usage log rows persisted successfully", "counter", "%d", m.UsageLogRecordsFlushed.Load())

	writePrometheusScalar(sb, "airoute_usage_log_records_dropped_total", "Usage log rows dropped before persistence", "counter", "%d", m.UsageLogRecordsDropped.Load())

	writePrometheusScalar(sb, "airoute_usage_log_flush_failures_total", "Failed usage log batch writes", "counter", "%d", m.UsageLogFlushFailures.Load())

	writePrometheusScalar(sb, "airoute_usage_log_queue_depth", "Usage log rows waiting in the local buffer", "gauge", "%d", m.UsageLogQueueDepth.Load())

	writePrometheusScalar(sb, "airoute_billing_duplicates_skipped_total", "At-least-once deliveries rejected by the billing idempotency key", "counter", "%d", m.BillingDuplicatesSkipped.Load())

	writePrometheusScalar(sb, "airoute_billing_overdrafts_total", "Wallet charges that left a negative balance", "counter", "%d", m.BillingOverdrafts.Load())

	writePrometheusScalar(sb, "airoute_billing_settled_synchronously_total", "Charges applied before the audit row was written", "counter", "%d", m.BillingSettledSynchronously.Load())

	writePrometheusScalar(sb, "airoute_billing_settlements_deferred_total", "Charges deferred to the async pipeline", "counter", "%d", m.BillingSettlementsDeferred.Load())

	writePrometheusScalar(sb, "airoute_billing_charges_rejected_total", "Charges closed out without billing because the price is not in CNY", "counter", "%d", m.BillingChargesRejected.Load())

	writePrometheusScalar(sb, "airoute_billing_unattributed_total", "Charges settled with no wallet to bill (uncollected)", "counter", "%d", m.BillingUnattributed.Load())

	writePrometheusScalar(sb, "airoute_usage_log_records_spilled_total", "Billing records written to the local spill file", "counter", "%d", m.UsageLogRecordsSpilled.Load())

	writePrometheusScalar(sb, "airoute_usage_log_spill_replay_failures_total", "Unreadable spill records", "counter", "%d", m.UsageLogSpillReplayFailures.Load())

	writePrometheusScalar(sb, "airoute_usage_log_spill_replays_total", "Spill records replayed into the database", "counter", "%d", m.UsageLogSpillReplays.Load())

	writePrometheusScalar(sb, "airoute_usage_log_stream_read_failures_total", "Failed audit-stream consumer reads", "counter", "%d", m.UsageLogStreamReadFailures.Load())

	writePrometheusScalar(sb, "airoute_usage_log_group_recreated_total", "Consumer groups re-created after going missing", "counter", "%d", m.UsageLogGroupRecreated.Load())

	writePrometheusScalar(sb, "airoute_usage_log_stream_poisoned_total", "Undecodable audit-stream entries dead-lettered", "counter", "%d", m.UsageLogStreamPoisoned.Load())

	writePrometheusScalar(sb, "airoute_billing_ledger_prune_contended_total", "Ledger retention batches retried after a replica collision", "counter", "%d", m.BillingLedgerPruneContended.Load())

	writePrometheusScalar(sb, "airoute_billing_ledger_drift_units", "Absolute key-vs-audit cost drift in 1e-9 units", "gauge", "%d", m.BillingLedgerDriftUnits.Load())

	writePrometheusScalar(sb, "airoute_billing_wallet_drift_units", "Absolute balance-mirror drift in 1e-9 units", "gauge", "%d", m.BillingWalletDriftUnits.Load())

	writePrometheusScalar(sb, "airoute_aborted_drained_requests_total", "Aborted requests whose upstream was drained for exact usage", "counter", "%d", m.AbortedDrainedRequests.Load())

	writePrometheusScalar(sb, "airoute_aborted_drained_completion_tokens_total", "Completion tokens paid for and discarded by abort draining", "counter", "%d", m.AbortedDrainedCompletionTokens.Load())
}

func (m *Metrics) writeEstimateMetrics(sb *strings.Builder, stats prometheusSnapshot) {
	// The per-model series are bounded by modelLabelCap in metrics.go: models past
	// the cap fold into the "_unrecognized" bucket, so a client-influenced model
	// string cannot grow the exported series set without bound.
	writePrometheusScalar(sb, "airoute_billing_estimate_bias_ppm", "Local estimate vs upstream tokenizer, parts per million", "gauge", "%d", m.EstimateBiasPPM.Load())
	for model, ppm := range m.EstimateBiasByModel() {
		sb.WriteString(fmt.Sprintf("airoute_billing_estimate_bias_ppm{model=%q} %d\n", escapePrometheusLabel(model), ppm))
	}

	// A learned ratio is only applied after a minimum number of samples for that
	// model, so the counts are exported next to the bias: without them a bias value
	// cannot be told apart from one that is deliberately not in effect yet.
	sb.WriteString("# HELP airoute_billing_estimate_samples Upstream usage reports behind each model's learned token ratio\n")
	sb.WriteString("# TYPE airoute_billing_estimate_samples gauge\n")
	var estimateSamplesTotal uint64
	for model, samples := range m.EstimateSamplesByModel() {
		estimateSamplesTotal += samples
		sb.WriteString(fmt.Sprintf("airoute_billing_estimate_samples{model=%q} %d\n", escapePrometheusLabel(model), samples))
	}
	sb.WriteString(fmt.Sprintf("airoute_billing_estimate_samples %d\n", estimateSamplesTotal))
}

func (m *Metrics) writeSettlementMetrics(sb *strings.Builder, stats prometheusSnapshot) {
	writePrometheusScalar(sb, "airoute_billing_audit_lag_units", "Absolute settlement-vs-audit difference in 1e-9 units", "gauge", "%d", m.BillingAuditLagUnits.Load())

	writePrometheusScalar(sb, "airoute_billing_audit_pruned_units", "Charge units whose audit row was deliberately removed (retention/admin)", "gauge", "%d", m.BillingAuditPrunedUnits.Load())

	writePrometheusScalar(sb, "airoute_billing_ledger_unlinked_units", "Settled units with no matching audit row (legacy keys included)", "gauge", "%d", m.BillingLedgerUnlinkedUnits.Load())

	writePrometheusScalar(sb, "airoute_settlement_batches_total", "Settlement flush transactions executed", "counter", "%d", m.SettlementBatches.Load())

	writePrometheusScalar(sb, "airoute_settlement_batch_records_total", "Charges applied inside settlement flushes", "counter", "%d", m.SettlementBatchRecords.Load())

	writePrometheusScalar(sb, "airoute_settlement_queue_depth", "Charges waiting for a settlement flush", "gauge", "%d", m.SettlementQueueDepth.Load())

	writePrometheusScalar(sb, "airoute_settlement_wait_timeouts_total", "Charges whose request-side settlement wait expired", "counter", "%d", m.SettlementWaitTimeouts.Load())

	writePrometheusScalar(sb, "airoute_settlement_lock_timeouts_total", "Settlement attempts that gave up on a row-lock deadline", "counter", "%d", m.SettlementLockTimeouts.Load())

	writePrometheusScalar(sb, "airoute_billing_reserved_units", "Currently pre-authorised wallet amount in 1e-9 units", "gauge", "%d", m.BillingReservedUnits.Load())

	writePrometheusScalar(sb, "airoute_billing_wallet_total_units", "Sum of all wallet balances in 1e-9 units", "gauge", "%d", m.BillingWalletTotalUnits.Load())

	writePrometheusScalar(sb, "airoute_billing_waived_units_total", "Money delivered but not collected at the overdraft floor, 1e-9 units", "counter", "%d", m.BillingWaivedUnits.Load())

	SettlementDurationSeconds.WritePrometheus(sb, "airoute_settlement_duration_seconds", "Settlement transaction duration in seconds")
	SettlementBatchSize.WritePrometheus(sb, "airoute_settlement_batch_size", "Number of charges merged into one settlement transaction")
}

func (m *Metrics) writeLatencyMetrics(sb *strings.Builder, stats prometheusSnapshot) {
	writePrometheusScalar(sb, "airoute_ttft_avg_ms", "Average Time To First Token in milliseconds", "gauge", "%.2f", stats.avgTTFT)

	writePrometheusScalar(sb, "airoute_latency_avg_ms", "Average request latency in milliseconds", "gauge", "%.2f", stats.avgLatency)

	// Quantile latency percentiles
	writePrometheusScalar(sb, "airoute_latency_p50_ms", "Estimated P50 latency in milliseconds", "gauge", "%.2f", stats.p50)

	writePrometheusScalar(sb, "airoute_latency_p90_ms", "Estimated P90 latency in milliseconds", "gauge", "%.2f", stats.p90)

	writePrometheusScalar(sb, "airoute_latency_p99_ms", "Estimated P99 latency in milliseconds", "gauge", "%.2f", stats.p99)
}

func (m *Metrics) writeModelMetrics(sb *strings.Builder, stats prometheusSnapshot) {
	// Multi-dimensional model request metrics
	if len(stats.modelMap) > 0 {
		sb.WriteString("# HELP airoute_model_requests_total Total number of requests partitioned by model and status\n")
		sb.WriteString("# TYPE airoute_model_requests_total counter\n")
		for mod, stats := range stats.modelMap {
			escMod := escapePrometheusLabel(mod)
			for st, cnt := range stats {
				escSt := escapePrometheusLabel(st)
				sb.WriteString(fmt.Sprintf("airoute_model_requests_total{model=\"%s\",status=\"%s\"} %d\n", escMod, escSt, cnt))
			}
		}
	}
}

func (m *Metrics) writeDurationHistograms(sb *strings.Builder, stats prometheusSnapshot) {
	// Prometheus Histogram for Request Duration
	sb.WriteString("# HELP airoute_request_duration_ms Request latency duration histogram in milliseconds\n")
	sb.WriteString("# TYPE airoute_request_duration_ms histogram\n")
	for _, b := range defaultLatencyBuckets {
		sb.WriteString(fmt.Sprintf("airoute_request_duration_ms_bucket{le=\"%.0f\"} %d\n", b, stats.latCounts[b]))
	}
	sb.WriteString(fmt.Sprintf("airoute_request_duration_ms_bucket{le=\"+Inf\"} %d\n", stats.durCount))
	sb.WriteString(fmt.Sprintf("airoute_request_duration_ms_sum %.2f\n", stats.durTotal))
	sb.WriteString(fmt.Sprintf("airoute_request_duration_ms_count %d\n", stats.durCount))

	// Prometheus Histogram for TTFT
	sb.WriteString("# HELP airoute_ttft_duration_ms Time To First Token histogram in milliseconds\n")
	sb.WriteString("# TYPE airoute_ttft_duration_ms histogram\n")
	for _, b := range defaultTTFTBuckets {
		sb.WriteString(fmt.Sprintf("airoute_ttft_duration_ms_bucket{le=\"%.0f\"} %d\n", b, stats.ttftCounts[b]))
	}
	sb.WriteString(fmt.Sprintf("airoute_ttft_duration_ms_bucket{le=\"+Inf\"} %d\n", stats.ttftCount))
	sb.WriteString(fmt.Sprintf("airoute_ttft_duration_ms_sum %.2f\n", stats.ttftTotal))
	sb.WriteString(fmt.Sprintf("airoute_ttft_duration_ms_count %d\n", stats.ttftCount))
}

func writePrometheusScalar(sb *strings.Builder, name, help, kind, format string, value any) {
	fmt.Fprintf(sb, "# HELP %s %s\n# TYPE %s %s\n%s "+format+"\n", name, help, name, kind, name, value)
}
