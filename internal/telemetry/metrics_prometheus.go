package telemetry

import (
	"fmt"
	"strings"
)

var prometheusLabelReplacer = strings.NewReplacer(`\`, `\\`, `"`, `\"`, "\n", `\n`)

func escapePrometheusLabel(v string) string {
	return prometheusLabelReplacer.Replace(v)
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

	sb.WriteString("# HELP airoute_requests_client_canceled_total Requests abandoned by the downstream\n")
	sb.WriteString("# TYPE airoute_requests_client_canceled_total counter\n")
	sb.WriteString(fmt.Sprintf("airoute_requests_client_canceled_total %d\n", m.ClientCanceledRequests.Load()))

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

	// Usage-log ingestion pipeline: a drop means an audit/billing row was lost.
	sb.WriteString("# HELP airoute_usage_log_records_flushed_total Usage log rows persisted successfully\n")
	sb.WriteString("# TYPE airoute_usage_log_records_flushed_total counter\n")
	sb.WriteString(fmt.Sprintf("airoute_usage_log_records_flushed_total %d\n", m.UsageLogRecordsFlushed.Load()))

	sb.WriteString("# HELP airoute_usage_log_records_dropped_total Usage log rows dropped before persistence\n")
	sb.WriteString("# TYPE airoute_usage_log_records_dropped_total counter\n")
	sb.WriteString(fmt.Sprintf("airoute_usage_log_records_dropped_total %d\n", m.UsageLogRecordsDropped.Load()))

	sb.WriteString("# HELP airoute_usage_log_flush_failures_total Failed usage log batch writes\n")
	sb.WriteString("# TYPE airoute_usage_log_flush_failures_total counter\n")
	sb.WriteString(fmt.Sprintf("airoute_usage_log_flush_failures_total %d\n", m.UsageLogFlushFailures.Load()))

	sb.WriteString("# HELP airoute_usage_log_queue_depth Usage log rows waiting in the local buffer\n")
	sb.WriteString("# TYPE airoute_usage_log_queue_depth gauge\n")
	sb.WriteString(fmt.Sprintf("airoute_usage_log_queue_depth %d\n", m.UsageLogQueueDepth.Load()))

	sb.WriteString("# HELP airoute_billing_duplicates_skipped_total At-least-once deliveries rejected by the billing idempotency key\n")
	sb.WriteString("# TYPE airoute_billing_duplicates_skipped_total counter\n")
	sb.WriteString(fmt.Sprintf("airoute_billing_duplicates_skipped_total %d\n", m.BillingDuplicatesSkipped.Load()))

	sb.WriteString("# HELP airoute_billing_overdrafts_total Wallet charges that left a negative balance\n")
	sb.WriteString("# TYPE airoute_billing_overdrafts_total counter\n")
	sb.WriteString(fmt.Sprintf("airoute_billing_overdrafts_total %d\n", m.BillingOverdrafts.Load()))

	sb.WriteString("# HELP airoute_billing_settled_synchronously_total Charges applied before the audit row was written\n")
	sb.WriteString("# TYPE airoute_billing_settled_synchronously_total counter\n")
	sb.WriteString(fmt.Sprintf("airoute_billing_settled_synchronously_total %d\n", m.BillingSettledSynchronously.Load()))

	sb.WriteString("# HELP airoute_billing_settlements_deferred_total Charges deferred to the async pipeline\n")
	sb.WriteString("# TYPE airoute_billing_settlements_deferred_total counter\n")
	sb.WriteString(fmt.Sprintf("airoute_billing_settlements_deferred_total %d\n", m.BillingSettlementsDeferred.Load()))

	sb.WriteString("# HELP airoute_billing_charges_rejected_total Charges closed out without billing because the price is not in CNY\n")
	sb.WriteString("# TYPE airoute_billing_charges_rejected_total counter\n")
	sb.WriteString(fmt.Sprintf("airoute_billing_charges_rejected_total %d\n", m.BillingChargesRejected.Load()))

	sb.WriteString("# HELP airoute_billing_unattributed_total Charges settled with no wallet to bill (uncollected)\n")
	sb.WriteString("# TYPE airoute_billing_unattributed_total counter\n")
	sb.WriteString(fmt.Sprintf("airoute_billing_unattributed_total %d\n", m.BillingUnattributed.Load()))

	sb.WriteString("# HELP airoute_usage_log_records_spilled_total Billing records written to the local spill file\n")
	sb.WriteString("# TYPE airoute_usage_log_records_spilled_total counter\n")
	sb.WriteString(fmt.Sprintf("airoute_usage_log_records_spilled_total %d\n", m.UsageLogRecordsSpilled.Load()))

	sb.WriteString("# HELP airoute_usage_log_spill_replay_failures_total Unreadable spill records\n")
	sb.WriteString("# TYPE airoute_usage_log_spill_replay_failures_total counter\n")
	sb.WriteString(fmt.Sprintf("airoute_usage_log_spill_replay_failures_total %d\n", m.UsageLogSpillReplayFailures.Load()))

	sb.WriteString("# HELP airoute_billing_ledger_drift_units Absolute key-vs-audit cost drift in 1e-9 units\n")
	sb.WriteString("# TYPE airoute_billing_ledger_drift_units gauge\n")
	sb.WriteString(fmt.Sprintf("airoute_billing_ledger_drift_units %d\n", m.BillingLedgerDriftUnits.Load()))

	sb.WriteString("# HELP airoute_billing_wallet_drift_units Absolute balance-mirror drift in 1e-9 units\n")
	sb.WriteString("# TYPE airoute_billing_wallet_drift_units gauge\n")
	sb.WriteString(fmt.Sprintf("airoute_billing_wallet_drift_units %d\n", m.BillingWalletDriftUnits.Load()))

	sb.WriteString("# HELP airoute_aborted_drained_requests_total Aborted requests whose upstream was drained for exact usage\n")
	sb.WriteString("# TYPE airoute_aborted_drained_requests_total counter\n")
	sb.WriteString(fmt.Sprintf("airoute_aborted_drained_requests_total %d\n", m.AbortedDrainedRequests.Load()))

	sb.WriteString("# HELP airoute_aborted_drained_completion_tokens_total Completion tokens paid for and discarded by abort draining\n")
	sb.WriteString("# TYPE airoute_aborted_drained_completion_tokens_total counter\n")
	sb.WriteString(fmt.Sprintf("airoute_aborted_drained_completion_tokens_total %d\n", m.AbortedDrainedCompletionTokens.Load()))

	// The per-model series are bounded by modelLabelCap in metrics.go: models past
	// the cap fold into the "_unrecognized" bucket, so a client-influenced model
	// string cannot grow the exported series set without bound.
	sb.WriteString("# HELP airoute_billing_estimate_bias_ppm Local estimate vs upstream tokenizer, parts per million\n")
	sb.WriteString("# TYPE airoute_billing_estimate_bias_ppm gauge\n")
	sb.WriteString(fmt.Sprintf("airoute_billing_estimate_bias_ppm %d\n", m.EstimateBiasPPM.Load()))
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

	sb.WriteString("# HELP airoute_billing_audit_lag_units Absolute settlement-vs-audit difference in 1e-9 units\n")
	sb.WriteString("# TYPE airoute_billing_audit_lag_units gauge\n")
	sb.WriteString(fmt.Sprintf("airoute_billing_audit_lag_units %d\n", m.BillingAuditLagUnits.Load()))

	sb.WriteString("# HELP airoute_billing_audit_pruned_units Charge units whose audit row was deliberately removed (retention/admin)\n")
	sb.WriteString("# TYPE airoute_billing_audit_pruned_units gauge\n")
	sb.WriteString(fmt.Sprintf("airoute_billing_audit_pruned_units %d\n", m.BillingAuditPrunedUnits.Load()))

	sb.WriteString("# HELP airoute_billing_ledger_unlinked_units Settled units with no matching audit row (legacy keys included)\n")
	sb.WriteString("# TYPE airoute_billing_ledger_unlinked_units gauge\n")
	sb.WriteString(fmt.Sprintf("airoute_billing_ledger_unlinked_units %d\n", m.BillingLedgerUnlinkedUnits.Load()))

	sb.WriteString("# HELP airoute_settlement_batches_total Settlement flush transactions executed\n")
	sb.WriteString("# TYPE airoute_settlement_batches_total counter\n")
	sb.WriteString(fmt.Sprintf("airoute_settlement_batches_total %d\n", m.SettlementBatches.Load()))

	sb.WriteString("# HELP airoute_settlement_batch_records_total Charges applied inside settlement flushes\n")
	sb.WriteString("# TYPE airoute_settlement_batch_records_total counter\n")
	sb.WriteString(fmt.Sprintf("airoute_settlement_batch_records_total %d\n", m.SettlementBatchRecords.Load()))

	sb.WriteString("# HELP airoute_settlement_queue_depth Charges waiting for a settlement flush\n")
	sb.WriteString("# TYPE airoute_settlement_queue_depth gauge\n")
	sb.WriteString(fmt.Sprintf("airoute_settlement_queue_depth %d\n", m.SettlementQueueDepth.Load()))

	sb.WriteString("# HELP airoute_settlement_wait_timeouts_total Charges whose request-side settlement wait expired\n")
	sb.WriteString("# TYPE airoute_settlement_wait_timeouts_total counter\n")
	sb.WriteString(fmt.Sprintf("airoute_settlement_wait_timeouts_total %d\n", m.SettlementWaitTimeouts.Load()))

	sb.WriteString("# HELP airoute_settlement_lock_timeouts_total Settlement attempts that gave up on a row-lock deadline\n")
	sb.WriteString("# TYPE airoute_settlement_lock_timeouts_total counter\n")
	sb.WriteString(fmt.Sprintf("airoute_settlement_lock_timeouts_total %d\n", m.SettlementLockTimeouts.Load()))

	sb.WriteString("# HELP airoute_billing_reserved_units Currently pre-authorised wallet amount in 1e-9 units\n")
	sb.WriteString("# TYPE airoute_billing_reserved_units gauge\n")
	sb.WriteString(fmt.Sprintf("airoute_billing_reserved_units %d\n", m.BillingReservedUnits.Load()))

	sb.WriteString("# HELP airoute_billing_wallet_total_units Sum of all wallet balances in 1e-9 units\n")
	sb.WriteString("# TYPE airoute_billing_wallet_total_units gauge\n")
	sb.WriteString(fmt.Sprintf("airoute_billing_wallet_total_units %d\n", m.BillingWalletTotalUnits.Load()))

	sb.WriteString("# HELP airoute_billing_waived_units_total Money delivered but not collected at the overdraft floor, 1e-9 units\n")
	sb.WriteString("# TYPE airoute_billing_waived_units_total counter\n")
	sb.WriteString(fmt.Sprintf("airoute_billing_waived_units_total %d\n", m.BillingWaivedUnits.Load()))

	SettlementDurationSeconds.WritePrometheus(&sb, "airoute_settlement_duration_seconds", "Settlement transaction duration in seconds")
	SettlementBatchSize.WritePrometheus(&sb, "airoute_settlement_batch_size", "Number of charges merged into one settlement transaction")

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
			escMod := escapePrometheusLabel(mod)
			for st, cnt := range stats {
				escSt := escapePrometheusLabel(st)
				sb.WriteString(fmt.Sprintf("airoute_model_requests_total{model=\"%s\",status=\"%s\"} %d\n", escMod, escSt, cnt))
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
