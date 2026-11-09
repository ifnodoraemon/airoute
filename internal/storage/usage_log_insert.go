package storage

import (
	"context"
	"strings"
	"time"

	"github.com/ifnodoraemon/airoute/internal/telemetry"
)

const usageInsertColumns = `trace_id, chat_id, session_id, api_key, tenant_id, model, channel, prompt_tokens, completion_tokens, cached_tokens, total_tokens, cost, cost_units, is_off_peak, off_peak_discount, duration_ms, ttft_ms, status_code, usage_source, error_message, pricing_snapshot, created_at, billing_id, thinking, channel_trail`

func (r *Repository) insertUsageLogsContext(ctx context.Context, logs []*UsageLogRecord) error {
	filtered := make([]*UsageLogRecord, 0, len(logs))
	for _, rec := range logs {
		if rec == nil {
			continue
		}
		if rec.BillingID == "" {
			rec.BillingID = NewBillingID()
		}
		filtered = append(filtered, rec)
	}
	if len(filtered) == 0 {
		return nil
	}
	tx, err := r.db.BeginTx(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	partitioned := r.usageLogsPartitioned()
	if partitioned {
		filtered, err = reserveUsageAuditIDs(tx, filtered)
		if err != nil {
			return err
		}
	}
	for start := 0; start < len(filtered); start += 25 {
		chunk := filtered[start:min(start+25, len(filtered))]
		inserted, err := r.insertAuditChunk(ctx, tx, chunk, partitioned)
		if err != nil {
			return err
		}
		if skipped := len(chunk) - inserted; skipped > 0 {
			telemetry.GlobalMetrics.BillingDuplicatesSkipped.Add(uint64(skipped))
		}
	}
	return commitUsageAudit(tx, logs)
}

func (r *Repository) insertAuditChunk(ctx context.Context, tx *Tx, logs []*UsageLogRecord, partitioned bool) (int, error) {
	args := make([]any, 0, len(logs)*25)
	placeholders := make([]string, len(logs))
	for i, rec := range logs {
		placeholders[i] = "(" + strings.TrimSuffix(strings.Repeat("?,", 25), ",") + ")"
		args = append(args, r.usageLogInsertArgs(rec)...)
	}
	query := `INSERT INTO usage_logs (` + usageInsertColumns + `) VALUES ` + strings.Join(placeholders, ",")
	if !partitioned {
		query += ` ON CONFLICT (billing_id) DO NOTHING`
	}
	rows, err := tx.QueryContext(ctx, query+` RETURNING billing_id`, args...)
	if err != nil {
		return 0, err
	}
	defer rows.Close()
	inserted := 0
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return 0, err
		}
		inserted++
	}
	return inserted, rows.Err()
}

func (r *Repository) usageLogInsertArgs(rec *UsageLogRecord) []any {
	ensureTraceID(rec)
	isOff := 0
	if rec.IsOffPeak {
		isOff = 1
	}
	discount := rec.OffPeakDiscount
	if discount <= 0 {
		discount = 1
	}
	created := rec.CreatedAt
	if created.IsZero() {
		created = time.Now()
	}
	return []any{
		rec.TraceID, rec.ChatID, rec.SessionID, rec.APIKey, rec.TenantID, rec.Model, rec.Channel,
		rec.PromptTokens, rec.CompletionTokens, rec.CachedTokens, rec.TotalTokens, rec.Cost, ToUnits(rec.Cost),
		isOff, discount, rec.DurationMs, rec.TTFTMs, rec.StatusCode, normalizeUsageSource(rec.UsageSource),
		truncateError(rec.ErrorMessage), encodePricingSnapshot(rec.PricingSnapshot), r.dialectTimePrecise(created),
		rec.BillingID, encodeThinkingObservation(rec.Thinking), rec.ChannelTrail,
	}
}
