package storage

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/ifnodoraemon/airoute/internal/telemetry"
)

// clampRunes returns s truncated to at most max runes ("" passes through).
func clampRunes(s string, max int) string {
	if s == "" {
		return s
	}
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	return string(r[:max])
}

// normalizeUsageLog clamps string fields to their storage column limits
// (model/channel/tenant_id VARCHAR(128), api_key VARCHAR(255) on Postgres).
func normalizeUsageLog(log *UsageLogRecord) {
	if log == nil {
		return
	}
	log.Model = clampRunes(log.Model, 128)
	log.Channel = clampRunes(log.Channel, 128)
	log.TenantID = clampRunes(log.TenantID, 128)
	log.APIKey = NormalizeStoredAPIKeyReference(log.APIKey)
	log.APIKey = clampRunes(log.APIKey, 255)
}

// NormalizeStoredAPIKeyReference turns any api_key column/JSON field value into the
// stored digest form.
//
// The request path already supplies the digest. The remaining sources are:
//
//   - a Redis Stream message or spill file written by a pre-migration binary, which
//     carries the cleartext credential. Hashing it yields exactly the digest the
//     migration wrote into api_keys.key for that credential, so settlement and
//     reconciliation still attribute the charge correctly - and no cleartext is
//     persisted;
//   - an auth-failure audit row (a fingerprint) or an operator-supplied label,
//     which hashes into a digest that matches no key and therefore behaves as an
//     unattributed charge, as before.
//
// Either way the column never holds a readable credential.
func NormalizeStoredAPIKeyReference(value string) string {
	if value == "" {
		return ""
	}
	if IsAPIKeyHash(value) {
		return value
	}
	return HashAPIKey(value)
}

// truncateError keeps persisted error strings bounded (2 KiB).
func truncateError(msg string) string {
	if len(msg) <= 2048 {
		return msg
	}
	cut := 2048
	for cut > 0 && (msg[cut]&0xC0) == 0x80 {
		cut--
	}
	return msg[:cut]
}

// normalizeUsageSource clamps the usage provenance marker to a known value.
func normalizeUsageSource(src string) string {
	if src == "estimated" {
		return "estimated"
	}
	return "upstream"
}

// RecordUsageLog records an audit row and settles its charge.
func (r *Repository) RecordUsageLog(log *UsageLogRecord) error {
	return r.RecordUsageLogContext(context.Background(), log)
}

// RecordUsageLogContext is RecordUsageLog bound to a context/deadline.
//
// The charge is settled first, in its own transaction, and the audit row is
// written afterwards. Keeping them apart is what lets a charge survive an audit
// write failure: the money no longer depends on the log row.
func (r *Repository) RecordUsageLogContext(ctx context.Context, log *UsageLogRecord) error {
	normalizeUsageLog(log)
	if err := r.settleCharges(ctx, []*UsageLogRecord{log}); err != nil {
		return err
	}
	return r.insertUsageLogsContext(ctx, []*UsageLogRecord{log})
}

// BatchRecordUsageLogs writes multiple usage logs, settling their charges first.
func (r *Repository) BatchRecordUsageLogs(logs []*UsageLogRecord) error {
	return r.BatchRecordUsageLogsContext(context.Background(), logs)
}

// BatchRecordUsageLogsContext is BatchRecordUsageLogs bound to a context.
// Background writers must use a deadline: without one a stalled database pins
// the flush goroutine forever, the in-memory queue fills up, and request
// completion starts blocking on backpressure.
func (r *Repository) BatchRecordUsageLogsContext(ctx context.Context, logs []*UsageLogRecord) error {
	if len(logs) == 0 {
		return nil
	}
	for _, log := range logs {
		normalizeUsageLog(log)
	}
	// 1. Money first: idempotent per billing_id and independent of the audit row.
	if err := r.settleCharges(ctx, logs); err != nil {
		return err
	}
	// 2. Audit rows second: also idempotent per billing_id; a failure is retried
	//    by the caller (Redis PEL, spill replay, in-memory retry).
	return r.insertUsageLogsContext(ctx, logs)
}

// insertUsageLogsContext persists audit rows only. It never moves money; that is
// settleCharges' job.
func (r *Repository) insertUsageLogsContext(ctx context.Context, logs []*UsageLogRecord) error {
	filtered := make([]*UsageLogRecord, 0, len(logs))
	for _, log := range logs {
		if log == nil {
			continue
		}
		if log.BillingID == "" {
			log.BillingID = NewBillingID()
		}
		filtered = append(filtered, log)
	}
	if len(filtered) == 0 {
		return nil
	}

	tx, err := r.db.BeginTx(ctx)
	if err != nil {
		return err
	}
	defer func() {
		_ = tx.Rollback()
	}()

	const (
		insertColumns  = `trace_id, chat_id, session_id, api_key, tenant_id, model, channel, prompt_tokens, completion_tokens, cached_tokens, total_tokens, cost, cost_units, is_off_peak, off_peak_discount, duration_ms, ttft_ms, status_code, usage_source, error_message, pricing_snapshot, created_at, billing_id`
		insertColumnsN = 23
		insertChunk    = 25
	)

	placeholdersFor := func(rows int) string {
		rowPlaceholder := "(" + strings.Repeat("?, ", insertColumnsN-1) + "?)"
		placeholders := make([]string, rows)
		for i := range placeholders {
			placeholders[i] = rowPlaceholder
		}
		return strings.Join(placeholders, ",")
	}

	appendArgs := func(args []interface{}, log *UsageLogRecord) []interface{} {
		isOff := 0
		if log.IsOffPeak {
			isOff = 1
		}
		discount := log.OffPeakDiscount
		if discount <= 0 {
			discount = 1.0
		}
		if log.TraceID == "" {
			if log.SessionID != "" {
				log.TraceID = log.SessionID
			} else {
				log.TraceID = fmt.Sprintf("tr-%x", time.Now().UnixMicro())
			}
		}
		createdAt := log.CreatedAt
		if createdAt.IsZero() {
			createdAt = time.Now()
		}
		// Every column is supplied so rows in one VALUES list share a shape, and
		// created_at always carries microseconds.
		return append(args,
			log.TraceID, log.ChatID, log.SessionID, log.APIKey, log.TenantID, log.Model, log.Channel,
			log.PromptTokens, log.CompletionTokens, log.CachedTokens, log.TotalTokens, log.Cost, ToUnits(log.Cost),
			isOff, discount, log.DurationMs, log.TTFTMs, log.StatusCode, normalizeUsageSource(log.UsageSource),
			truncateError(log.ErrorMessage), encodePricingSnapshot(log.PricingSnapshot),
			createdAt.UTC().Format(sqliteTimestampLayout), log.BillingID,
		)
	}

	if len(filtered) == 1 {
		log := filtered[0]
		res, err := tx.Exec(`INSERT INTO usage_logs (`+insertColumns+`) VALUES `+placeholdersFor(1)+` ON CONFLICT (billing_id) DO NOTHING`,
			appendArgs(make([]interface{}, 0, insertColumnsN), log)...)
		if err != nil {
			return err
		}
		if affected, raErr := res.RowsAffected(); raErr == nil && affected == 0 {
			telemetry.GlobalMetrics.BillingDuplicatesSkipped.Add(1)
		}
		return tx.Commit()
	}

	rowArgs := make([]interface{}, 0, len(filtered)*insertColumnsN)
	for _, log := range filtered {
		rowArgs = appendArgs(rowArgs, log)
	}

	rowCount := len(rowArgs) / insertColumnsN
	inserted := 0
	for start := 0; start < rowCount; start += insertChunk {
		end := start + insertChunk
		if end > rowCount {
			end = rowCount
		}
		query := `INSERT INTO usage_logs (` + insertColumns + `) VALUES ` + placeholdersFor(end-start) + ` ON CONFLICT (billing_id) DO NOTHING RETURNING billing_id`
		rows, err := tx.Query(query, rowArgs[start*insertColumnsN:end*insertColumnsN]...)
		if err != nil {
			return err
		}
		for rows.Next() {
			var billingID string
			if err := rows.Scan(&billingID); err != nil {
				rows.Close()
				return err
			}
			inserted++
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			return err
		}
		rows.Close()
	}
	if skipped := rowCount - inserted; skipped > 0 {
		telemetry.GlobalMetrics.BillingDuplicatesSkipped.Add(uint64(skipped))
	}
	return tx.Commit()
}
